package schemas

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

var NoDeadline time.Time

var reservedKeys = []any{
	RakshaContextKeyVirtualKey,
	RakshaContextKeyAPIKeyName,
	RakshaContextKeyAPIKeyID,
	RakshaContextKeyDirectKey,
	RakshaContextKeyRequestID,
	RakshaContextKeyFallbackRequestID,
	RakshaContextKeySelectedKeyID,
	RakshaContextKeySelectedKeyName,
	RakshaContextKeyNumberOfRetries,
	RakshaContextKeyFallbackIndex,
	RakshaContextKeySkipKeySelection,
	RakshaContextKeySkipBudgetAndRateLimits,
	RakshaContextKeyURLPath,
	RakshaContextKeyDeferTraceCompletion,
	RakshaContextKeyAttemptTrail,
	RakshaContextKeyStreamGated,
	RakshaContextKeyMCPHealthCheckRequest,
}

// pluginLogStore holds plugin log entries accumulated during request processing.
// It is shared between the root RakshaContext and all scoped contexts derived from it.
// Uses a flat slice (not map) to minimize heap allocations.
type pluginLogStore struct {
	mu   sync.Mutex
	logs []PluginLogEntry
}

// pluginLogStorePool pools pluginLogStore instances to reduce per-request allocations.
var pluginLogStorePool = sync.Pool{
	New: func() any {
		return &pluginLogStore{logs: make([]PluginLogEntry, 0, 8)}
	},
}

// RakshaContext is a custom context.Context implementation that tracks user-set values.
// It supports deadlines, can be derived from other contexts, and provides layered
// value inheritance when derived from another RakshaContext.
type RakshaContext struct {
	parent                context.Context
	deadline              time.Time
	hasDeadline           bool
	done                  chan struct{}
	doneOnce              sync.Once
	err                   error
	errMu                 sync.RWMutex
	userValues            map[any]any
	valuesMu              sync.RWMutex
	blockRestrictedWrites atomic.Bool

	// Plugin scoping fields
	pluginScope   *string                        // Non-nil when this is a scoped plugin context
	pluginLogs    atomic.Pointer[pluginLogStore] // Shared log store; lazily initialized on root, shared by scoped contexts
	valueDelegate *RakshaContext                // For scoped contexts: delegate Value/SetValue to this root context
}

// NewRakshaContext creates a new RakshaContext with the given parent context and deadline.
// If the deadline is zero, no deadline is set on this context (though the parent may have one).
// The context will be cancelled when the deadline expires or when the parent context is cancelled.
func NewRakshaContext(parent context.Context, deadline time.Time) *RakshaContext {
	if parent == nil {
		parent = context.Background()
	}
	// Unwrap pooled scoped RakshaContexts to their delegate root. A scoped
	// context (from WithPluginScope) is reset and returned to a sync.Pool when
	// ReleasePluginScope is called, which can happen before a derived context's
	// watchCancellation goroutine has finished observing parent.Deadline()/Done().
	// Pointing the derived parent at the long-lived root avoids that race.
	for {
		bc, ok := parent.(*RakshaContext)
		if !ok || bc.valueDelegate == nil {
			break
		}
		parent = bc.valueDelegate
	}
	ctx := &RakshaContext{
		parent:                parent,
		deadline:              deadline,
		hasDeadline:           !deadline.IsZero(),
		done:                  make(chan struct{}),
		userValues:            make(map[any]any),
		blockRestrictedWrites: atomic.Bool{},
	}
	ctx.blockRestrictedWrites.Store(false)
	// Only start goroutine if there's something to watch:
	// - If we have a deadline, we need the timer
	// - If parent can be cancelled (Done() != nil) AND is not a non-cancelling context
	// - If parent has a deadline, we need a timer (parent may not properly cancel via Done())
	_, parentHasDeadline := parent.Deadline()
	parentCanCancel := parent.Done() != nil && !isNonCancellingContext(parent)
	if ctx.hasDeadline || parentCanCancel || parentHasDeadline {
		go ctx.watchCancellation()
	}
	return ctx
}

// NewRakshaContextWithValue creates a new RakshaContext with the given value set.
func NewRakshaContextWithValue(parent context.Context, deadline time.Time, key any, value any) *RakshaContext {
	ctx := NewRakshaContext(parent, deadline)
	ctx.SetValue(key, value)
	return ctx
}

// NewRakshaContextWithTimeout creates a new RakshaContext with a timeout duration.
// This is a convenience wrapper around NewRakshaContext.
// Returns the context and a cancel function that should be called to release resources.
func NewRakshaContextWithTimeout(parent context.Context, timeout time.Duration) (*RakshaContext, context.CancelFunc) {
	ctx := NewRakshaContext(parent, time.Now().Add(timeout))
	return ctx, func() { ctx.Cancel() }
}

// NewRakshaContextWithCancel creates a new RakshaContext with a cancel function.
// This is a convenience wrapper around NewRakshaContext.
// Returns the context and a cancel function that should be called to release resources.
func NewRakshaContextWithCancel(parent context.Context) (*RakshaContext, context.CancelFunc) {
	ctx := NewRakshaContext(parent, NoDeadline)
	return ctx, func() { ctx.Cancel() }
}

// WithValue returns a new context with the given value set.
func (bc *RakshaContext) WithValue(key any, value any) *RakshaContext {
	bc.SetValue(key, value)
	return bc
}

// Root returns the underlying root RakshaContext. For root contexts this is
// the receiver itself; for plugin-scoped contexts it is the underlying root
// that scoped Value/SetValue calls delegate to.
//
// PLUGIN AUTHORS: capture Root() synchronously inside Pre/PostLLMHook (or
// any other hook) when you need to write to the context from a goroutine
// that outlives the hook. The plugin-scoped *RakshaContext passed into your
// hook is reclaimed by an internal sync.Pool the moment the hook returns —
// any later SetValue/Value call on it lands in detached storage that nobody
// downstream can read (and can leak into a future pool reuse). The root,
// in contrast, lives for the entire request, so a pointer captured here is
// safe to use for the lifetime of the request even after your hook returns.
//
// Example:
//
//	func (p *Plugin) PreLLMHook(ctx *schemas.RakshaContext, req ...) (...) {
//	    rootCtx := ctx.Root() // capture before the scope is released
//	    go func() {
//	        // ... long-running work that produces stream chunks ...
//	        rootCtx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
//	    }()
//	    return req, &schemas.LLMPluginShortCircuit{Stream: ch}, nil
//	}
func (bc *RakshaContext) Root() *RakshaContext {
	// Unwrap the full delegation chain. A scoped context can in principle be
	// derived from another scoped context (e.g. nested plugin scopes), and
	// stopping at the first valueDelegate would return an intermediate pooled
	// scope — which loses the async-safety guarantee as soon as that
	// intermediate scope is released.
	for bc != nil && bc.valueDelegate != nil {
		bc = bc.valueDelegate
	}
	return bc
}

// BlockRestrictedWrites returns true if restricted writes are blocked.
func (bc *RakshaContext) BlockRestrictedWrites() {
	bc.blockRestrictedWrites.Store(true)
}

// UnblockRestrictedWrites unblocks restricted writes.
func (bc *RakshaContext) UnblockRestrictedWrites() {
	bc.blockRestrictedWrites.Store(false)
}

// Cancel cancels the context, closing the Done channel and setting the error to context.Canceled.
func (bc *RakshaContext) Cancel() {
	bc.cancel(context.Canceled)
}

// watchCancellation monitors for deadline expiration and parent cancellation.
func (bc *RakshaContext) watchCancellation() {
	var timer <-chan time.Time

	// Use effective deadline (considers both own and parent deadlines)
	// This handles cases where parent has a deadline but doesn't properly
	// cancel via Done() (e.g., fasthttp.RequestCtx)
	if effectiveDeadline, hasDeadline := bc.Deadline(); hasDeadline {
		duration := time.Until(effectiveDeadline)
		if duration <= 0 {
			// Deadline already passed
			bc.cancel(context.DeadlineExceeded)
			return
		}
		t := time.NewTimer(duration)
		defer t.Stop()
		timer = t.C
	}

	// Don't watch parent.Done() for contexts known to never close it
	// (e.g., fasthttp.RequestCtx pools contexts and never cancels them)
	if isNonCancellingContext(bc.parent) {
		select {
		case <-timer:
			bc.cancel(context.DeadlineExceeded)
		case <-bc.done:
			// Already cancelled
		}
		return
	}

	select {
	case <-bc.parent.Done():
		bc.cancel(bc.parent.Err())
	case <-timer:
		bc.cancel(context.DeadlineExceeded)
	case <-bc.done:
		// Already cancelled
	}
}

// cancel closes the done channel and sets the error.
func (bc *RakshaContext) cancel(err error) {
	bc.doneOnce.Do(func() {
		bc.errMu.Lock()
		bc.err = err
		bc.errMu.Unlock()
		close(bc.done)
	})
}

// Deadline returns the deadline for this context.
// For scoped contexts, delegates to the root context.
// If both this context and the parent have deadlines, the earlier one is returned.
func (bc *RakshaContext) Deadline() (time.Time, bool) {
	if bc.valueDelegate != nil {
		return bc.valueDelegate.Deadline()
	}
	parentDeadline, parentHasDeadline := bc.parent.Deadline()

	if !bc.hasDeadline && !parentHasDeadline {
		return time.Time{}, false
	}

	if !bc.hasDeadline {
		return parentDeadline, true
	}

	if !parentHasDeadline {
		return bc.deadline, true
	}

	// Both have deadlines, return the earlier one
	if bc.deadline.Before(parentDeadline) {
		return bc.deadline, true
	}
	return parentDeadline, true
}

// Done returns a channel that is closed when the context is cancelled.
func (bc *RakshaContext) Done() <-chan struct{} {
	return bc.done
}

// Err returns the error explaining why the context was cancelled.
// For scoped contexts, delegates to the root context.
// Returns nil if the context has not been cancelled.
func (bc *RakshaContext) Err() error {
	if bc.valueDelegate != nil {
		return bc.valueDelegate.Err()
	}
	bc.errMu.RLock()
	defer bc.errMu.RUnlock()
	return bc.err
}

// Value returns the value associated with the key.
// For scoped contexts, delegates to the root context via valueDelegate.
// Otherwise checks the internal userValues map, then delegates to the parent context.
func (bc *RakshaContext) Value(key any) any {
	if bc.valueDelegate != nil {
		return bc.valueDelegate.Value(key)
	}
	bc.valuesMu.RLock()
	if val, ok := bc.userValues[key]; ok {
		bc.valuesMu.RUnlock()
		return val
	}
	bc.valuesMu.RUnlock()

	if bc.parent == nil {
		return nil
	}

	// Never read through to a pooled *fasthttp.RequestCtx parent — it's recycled once the handler returns.
	if isNonCancellingContext(bc.parent) {
		return nil
	}

	return bc.parent.Value(key)
}

// AuthMode derives the per-user OAuth lookup mode from current context state.
// Priority: UserID > VirtualKey > session. Call this at token-lookup time, not
// in middleware — the governance plugin can inject UserID (via VK→owner
// resolution) after middleware runs, and the mode must reflect that.
//
// Returns MCPAuthModeNone when no identity column is populated (no user, no
// VK, no session header), so callers that branch on the returned mode alone
// cannot mistake an unauthenticated request for a session-mode caller.
//
// VK check uses RakshaContextKeyGovernanceVirtualKeyID (the resolved VK row
// ID) rather than RakshaContextKeyVirtualKey (the raw header value) because
// vk-mode token rows are keyed by the resolved VK ID.
func (bc *RakshaContext) MCPAuthMode() MCPAuthMode {
	if userID, ok := bc.Value(RakshaContextKeyUserID).(string); ok && userID != "" {
		return MCPAuthModeUser
	}
	if vkID, ok := bc.Value(RakshaContextKeyGovernanceVirtualKeyID).(string); ok && vkID != "" {
		return MCPAuthModeVK
	}
	if sid, ok := bc.Value(RakshaContextKeyMCPSessionID).(string); ok && sid != "" {
		return MCPAuthModeSession
	}
	return MCPAuthModeNone
}

// SetValue sets a value in the internal userValues map.
// For scoped contexts, delegates to the root context via valueDelegate.
// This is thread-safe and can be called concurrently.
func (bc *RakshaContext) SetValue(key, value any) {
	if bc.valueDelegate != nil {
		bc.valueDelegate.SetValue(key, value)
		return
	}
	// Check if the key is a reserved key
	if bc.blockRestrictedWrites.Load() && slices.Contains(reservedKeys, key) {
		// we silently drop writes for these reserved keys
		return
	}
	bc.valuesMu.Lock()
	defer bc.valuesMu.Unlock()
	if bc.userValues == nil {
		bc.userValues = make(map[any]any)
	}
	bc.userValues[key] = value
}

// setReservedValue writes a Raksha-owned reserved key, bypassing the
// blockRestrictedWrites check that gates the public SetValue path. It mirrors
// SetValue's valueDelegate recursion so writes from scoped plugin contexts
// reach the root. Internal use only — exposing this would defeat reservedKeys.
func (bc *RakshaContext) setReservedValue(key, value any) {
	if bc.valueDelegate != nil {
		bc.valueDelegate.setReservedValue(key, value)
		return
	}
	bc.valuesMu.Lock()
	defer bc.valuesMu.Unlock()
	if bc.userValues == nil {
		bc.userValues = make(map[any]any)
	}
	bc.userValues[key] = value
}

// ClearValue clears a value from the internal userValues map.
// For scoped contexts, delegates to the root context via valueDelegate.
func (bc *RakshaContext) ClearValue(key any) {
	if bc.valueDelegate != nil {
		bc.valueDelegate.ClearValue(key)
		return
	}
	// Check if the key is a reserved key
	if bc.blockRestrictedWrites.Load() && slices.Contains(reservedKeys, key) {
		// we silently drop writes for these reserved keys
		return
	}
	bc.valuesMu.Lock()
	defer bc.valuesMu.Unlock()
	if bc.userValues != nil {
		bc.userValues[key] = nil
	}
}

// GetAndSetValue gets a value from the internal userValues map and sets it.
// For scoped contexts, delegates to the root context via valueDelegate.
func (bc *RakshaContext) GetAndSetValue(key any, value any) any {
	if bc.valueDelegate != nil {
		return bc.valueDelegate.GetAndSetValue(key, value)
	}
	bc.valuesMu.Lock()
	defer bc.valuesMu.Unlock()
	// Check if the key is a reserved key
	if bc.blockRestrictedWrites.Load() && slices.Contains(reservedKeys, key) {
		// we silently drop writes for these reserved keys
		return bc.userValues[key]
	}
	if bc.userValues == nil {
		bc.userValues = make(map[any]any)
	}
	oldValue := bc.userValues[key]
	bc.userValues[key] = value
	return oldValue
}

// GetUserValues returns a copy of all user-set values in this context.
// If the parent is also a PluginContext, the values are merged with parent values
// (this context's values take precedence over parent values).
func (bc *RakshaContext) GetUserValues() map[any]any {
	result := make(map[any]any)

	// First, get parent's user values if parent is a PluginContext
	if parentCtx, ok := bc.parent.(*RakshaContext); ok {
		for k, v := range parentCtx.GetUserValues() {
			result[k] = v
		}
	}

	// Then overlay with our own values (our values take precedence)
	bc.valuesMu.RLock()
	for k, v := range bc.userValues {
		result[k] = v
	}
	bc.valuesMu.RUnlock()

	return result
}

// GetParentCtxWithUserValues returns a copy of the parent context with all user-set values merged in.
func (bc *RakshaContext) GetParentCtxWithUserValues() context.Context {
	parentCtx := bc.parent
	bc.valuesMu.RLock()
	for k, v := range bc.userValues {
		parentCtx = context.WithValue(parentCtx, k, v)
	}
	bc.valuesMu.RUnlock()
	return parentCtx
}

// PauseStream marks the active streaming response associated with this context
// as paused. While paused, chunks continue to flow through PostLLMHook (so
// plugins can still inspect them), but they are buffered instead of delivered
// to the client. Buffered chunks are flushed in order when ResumeStream is
// called. Idempotent. No-op if no Tracer or trace ID is present in ctx.
//
// Calling this method engages the pause/resume gate for the stream: provider
// send sites switch from a direct channel send to Tracer.GateSend. Streams that
// never call Pause/Resume/End pay no extra cost.
//
// Requires a real Tracer to be wired via the Raksha config (e.g.
// `framework/streaming/Accumulator`). Under `DefaultTracer()` (the
// `*NoOpTracer` fall-back used when `config.Tracer` is nil), this call is
// silently inert — chunks continue to flow direct to the client. See
// `DefaultTracer()` for the architectural reason core cannot ship a built-in
// gate impl.
func (bc *RakshaContext) PauseStream() {
	tr, _ := bc.Value(RakshaContextKeyTracer).(Tracer)
	tid, _ := bc.Value(RakshaContextKeyTraceID).(string)
	if tr == nil || tid == "" {
		return
	}
	bc.setReservedValue(RakshaContextKeyStreamGated, true)
	tr.PauseStream(tid)
}

// ResumeStream resumes a previously paused stream. Buffered chunks are flushed
// to the client in order, then live streaming continues. Idempotent. No-op if
// no Tracer or trace ID is present in ctx.
//
// Engages the pause/resume gate (see PauseStream). Requires a real Tracer
// (e.g. `framework/streaming/Accumulator`); inert under `DefaultTracer()`.
func (bc *RakshaContext) ResumeStream() {
	tr, _ := bc.Value(RakshaContextKeyTracer).(Tracer)
	tid, _ := bc.Value(RakshaContextKeyTraceID).(string)
	if tr == nil || tid == "" {
		return
	}
	bc.setReservedValue(RakshaContextKeyStreamGated, true)
	tr.ResumeStream(tid)
}

// EndStream terminates the active streaming response. Any buffered chunks are
// flushed first; if err is non-nil it is then delivered as a final error chunk.
// After EndStream returns, all further provider chunks for this stream are
// dropped (PostLLMHook still fires, but no client delivery happens). Idempotent.
// No-op if no Tracer or trace ID is present in ctx.
//
// Engages the pause/resume gate (see PauseStream). Requires a real Tracer
// (e.g. `framework/streaming/Accumulator`); inert under `DefaultTracer()`.
func (bc *RakshaContext) EndStream(err *RakshaError) {
	tr, _ := bc.Value(RakshaContextKeyTracer).(Tracer)
	tid, _ := bc.Value(RakshaContextKeyTraceID).(string)
	if tr == nil || tid == "" {
		return
	}
	bc.setReservedValue(RakshaContextKeyStreamGated, true)
	tr.EndStream(tid, err)
}

// IsStreamEnded reports whether the streaming response associated with this
// context has been ended via EndStream (or via a final/hard-error chunk
// flowing through the gate while Active). Read-only: does not engage the
// gate or create any tracer state. Returns false when no Tracer or trace ID
// is present in ctx, or when no accumulator exists for this stream.
func (bc *RakshaContext) IsStreamEnded() bool {
	tr, _ := bc.Value(RakshaContextKeyTracer).(Tracer)
	tid, _ := bc.Value(RakshaContextKeyTraceID).(string)
	if tr == nil || tid == "" {
		return false
	}
	return tr.IsStreamEnded(tid)
}

// IsStreamPaused reports whether the streaming response associated with this
// context is currently paused. Read-only: does not engage the gate or create
// any tracer state. Returns false when no Tracer or trace ID is present in
// ctx, or when no accumulator exists for this stream.
func (bc *RakshaContext) IsStreamPaused() bool {
	tr, _ := bc.Value(RakshaContextKeyTracer).(Tracer)
	tid, _ := bc.Value(RakshaContextKeyTraceID).(string)
	if tr == nil || tid == "" {
		return false
	}
	return tr.IsStreamPaused(tid)
}

// GetAccumulatedResponse returns a snapshot of the RakshaResponse assembled
// from chunks received so far on the streaming response associated with this
// context. Built on demand — useful from PostLLMHook (including while paused)
// to inspect the assembled output before deciding next steps. Read-only: does
// not engage the gate or mutate accumulator state. Returns nil if no Tracer
// or trace ID is present, no accumulator exists, no chunks have been
// accumulated yet, or the stream type is indeterminable.
func (bc *RakshaContext) GetAccumulatedResponse() *RakshaResponse {
	tr, _ := bc.Value(RakshaContextKeyTracer).(Tracer)
	tid, _ := bc.Value(RakshaContextKeyTraceID).(string)
	if tr == nil || tid == "" {
		return nil
	}
	return tr.GetAccumulatedResponse(tid)
}

// AppendRoutingEngineLog appends a routing engine log entry to the context.
// Parameters:
//   - ctx: The Raksha context
//   - engineName: Name of the routing engine (e.g., "governance", "routing-rule")
//   - message: Human-readable log message describing the decision/action
func (bc *RakshaContext) AppendRoutingEngineLog(engineName string, level LogLevel, message string) {
	entry := RoutingEngineLogEntry{
		Engine:    engineName,
		Level:     level,
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
	}
	AppendToContextList(bc, RakshaContextKeyRoutingEngineLogs, entry)
}

// GetRoutingEngineLogs retrieves all routing engine logs from the context.
// Parameters:
//   - ctx: The Raksha context
//
// Returns:
//   - []RoutingEngineLogEntry: Slice of routing engine log entries (nil if none)
func (bc *RakshaContext) GetRoutingEngineLogs() []RoutingEngineLogEntry {
	if val := bc.Value(RakshaContextKeyRoutingEngineLogs); val != nil {
		if logs, ok := val.([]RoutingEngineLogEntry); ok {
			return logs
		}
	}
	return nil
}

// AppendToContextList appends value to the context list at key, skipping the
// append when value already exists in the list. Downstream consumers of these
// lists (notably `routing_engines_used` → Prometheus labels) treat duplicate
// entries as bugs, so set semantics are enforced at the write site.
func AppendToContextList[T comparable](ctx *RakshaContext, key RakshaContextKey, value T) {
	if ctx == nil {
		return
	}
	existingValues, ok := ctx.Value(key).([]T)
	if !ok {
		existingValues = []T{}
	}
	if slices.Contains(existingValues, value) {
		return
	}
	ctx.SetValue(key, append(existingValues, value))
}

// WithPluginScope returns a scoped RakshaContext that shares the root's
// pluginLogs store and delegates Value/SetValue/Deadline/Err/Done operations
// to the root.
//
// Scoped contexts are NOT pool-reused. Plugins routinely pass the scoped ctx
// to stdlib helpers (context.WithDeadline, HTTP clients, vector store SDKs)
// which spawn watcher goroutines via context.propagateCancel — those watchers
// can read the scoped struct's fields long after the plugin returns. Reusing
// the struct across requests would race those reads. Allocating fresh is
// idiomatic Go context handling (the stdlib context types are not pooled
// either) and keeps the lifecycle race-free without atomics.
func (bc *RakshaContext) WithPluginScope(name *string) *RakshaContext {
	// Lazily initialize the plugin log store on the root context (CAS to avoid race)
	if bc.pluginLogs.Load() == nil {
		newStore := pluginLogStorePool.Get().(*pluginLogStore)
		if !bc.pluginLogs.CompareAndSwap(nil, newStore) {
			// Another goroutine initialized first — return unused store to pool
			pluginLogStorePool.Put(newStore)
		}
	}

	scoped := &RakshaContext{
		parent:        bc.parent,
		done:          bc.done,
		pluginScope:   name,
		valueDelegate: bc,
	}
	scoped.pluginLogs.Store(bc.pluginLogs.Load())
	return scoped
}

// ReleasePluginScope marks a scoped context as released. Safe no-op if called
// on a non-scoped context.
//
// We deliberately do NOT mutate the scoped struct's fields here. External
// watcher goroutines spawned via stdlib context helpers (e.g. propagateCancel
// from context.WithDeadline) may still hold references and read parent/done
// asynchronously. Mutating those fields would race the watchers. The struct
// becomes garbage and is reclaimed by the GC once all watchers have exited.
//
// We still release the plugin log store reference so it can be drained or
// reclaimed independently of the scope's lifetime.
func (bc *RakshaContext) ReleasePluginScope() {
	if bc.valueDelegate == nil {
		return // not a scoped context
	}
	bc.pluginLogs.Store(nil)
}

// SetTraceAttribute adds an attribute to the root span for the current trace.
// This is thread-safe and can be called concurrently.
func (bc *RakshaContext) SetTraceAttribute(key string, value any) {
	tr, _ := bc.Value(RakshaContextKeyTracer).(Tracer)
	tid, _ := bc.Value(RakshaContextKeyTraceID).(string)
	if tr == nil || tid == "" {
		return
	}
	handle := tr.GetSpanHandleByID(tid, nil)
	if handle == nil {
		return
	}
	tr.SetAttribute(handle, key, value)
}

// Log appends a structured log entry for the current plugin scope.
// No-op if the context is not scoped to a plugin or has no log store.
func (bc *RakshaContext) Log(level LogLevel, msg string) {
	store := bc.pluginLogs.Load()
	if bc.pluginScope == nil || store == nil {
		return
	}
	store.mu.Lock()
	store.logs = append(store.logs, PluginLogEntry{
		PluginName: *bc.pluginScope,
		Level:      level,
		Message:    msg,
		Timestamp:  time.Now().UnixMilli(),
	})
	store.mu.Unlock()
}

// GetPluginLogs returns a deep copy of all accumulated plugin log entries.
// Thread-safe. Returns nil if no logs have been recorded.
func (bc *RakshaContext) GetPluginLogs() []PluginLogEntry {
	store := bc.pluginLogs.Load()
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.logs) == 0 {
		return nil
	}
	copied := make([]PluginLogEntry, len(store.logs))
	copy(copied, store.logs)
	return copied
}

// DrainPluginLogs transfers ownership of the plugin log slice to the caller.
// The internal log store is returned to the pool after draining.
// Returns nil if no logs have been recorded.
// This should be called once on the root context after all plugin hooks have completed.
func (bc *RakshaContext) DrainPluginLogs() []PluginLogEntry {
	if bc.valueDelegate != nil {
		return nil // scoped contexts must not drain the shared log store
	}
	store := bc.pluginLogs.Load()
	if store == nil {
		return nil
	}
	bc.pluginLogs.Store(nil)

	store.mu.Lock()
	logs := store.logs
	// Reset with fresh pre-allocated slice before returning to pool
	store.logs = make([]PluginLogEntry, 0, 8)
	store.mu.Unlock()

	// Return the store to the pool for reuse
	pluginLogStorePool.Put(store)

	if len(logs) == 0 {
		return nil
	}
	return logs
}
