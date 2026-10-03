// Package alerts is a process-wide fan-out for operational incidents (circuit breaker trips,
// budget exhaustion, rate limiting). Producers call Emit; the HTTP transport registers a
// sink that delivers events to the workspace's enabled alert channels.
package alerts

import (
	"strings"
	"sync"
	"time"
)

// Severity values understood by the delivery sink.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Event kinds emitted by the gateway.
const (
	KindCircuitBreakerOpen = "circuit_breaker_open"
	KindBudgetExceeded     = "budget_exceeded"
	KindRateLimited        = "rate_limited"
)

// Event is one operational incident.
type Event struct {
	Kind     string            `json:"kind"`
	Severity string            `json:"severity"`
	Title    string            `json:"title"`
	Message  string            `json:"message"`
	Fields   map[string]string `json:"fields,omitempty"`
	// DedupeKey groups repeats of the same incident; empty means Kind+Title.
	DedupeKey string    `json:"-"`
	Time      time.Time `json:"time"`
}

// Sink delivers an event. It runs on its own goroutine and must not block forever.
type Sink func(Event)

// DefaultDedupeWindow suppresses repeats of the same incident so a hot path (every request
// over budget) produces one alert, not thousands.
const DefaultDedupeWindow = 10 * time.Minute

var (
	mu       sync.Mutex
	sink     Sink
	lastSent = map[string]time.Time{}
	window   = DefaultDedupeWindow
)

// SetSink registers the delivery sink (nil disables delivery).
func SetSink(s Sink) {
	mu.Lock()
	sink = s
	mu.Unlock()
}

// SetDedupeWindow overrides the repeat-suppression window (tests).
func SetDedupeWindow(d time.Duration) {
	mu.Lock()
	window = d
	mu.Unlock()
}

// Emit queues an event for delivery unless the same incident was sent within the dedupe window.
// It never blocks the caller.
func Emit(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if strings.TrimSpace(e.Severity) == "" {
		e.Severity = SeverityWarning
	}
	key := e.DedupeKey
	if key == "" {
		key = e.Kind + "|" + e.Title
	}
	mu.Lock()
	s := sink
	if s == nil {
		mu.Unlock()
		return
	}
	if last, ok := lastSent[key]; ok && e.Time.Sub(last) < window {
		mu.Unlock()
		return
	}
	lastSent[key] = e.Time
	if len(lastSent) > 10000 {
		for k, t := range lastSent {
			if e.Time.Sub(t) >= window {
				delete(lastSent, k)
			}
		}
	}
	mu.Unlock()
	go s(e)
}
