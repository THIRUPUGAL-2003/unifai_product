package guardrails

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	gateway "github.com/gateway/gateway/core"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/plugins/prompts"
)

const PluginName = "guardrails"

// Ensure interface compliance
var _ schemas.LLMPlugin = (*GuardrailsPlugin)(nil)

type GuardrailsPlugin struct {
	config *Config
	logger schemas.Logger

	// Pre-compiled CEL programs mapped by rule ID
	celPrograms map[int]cel.Program

	// Initialized providers mapped by their ID
	providers map[int]Provider
}

// Config passed during initialization is now defined in config.go

// Provider interface for all guardrail types
type Provider interface {
	ValidateInput(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) error
	ValidateOutput(ctx *schemas.GatewayContext, req *schemas.GatewayRequest, resp *schemas.GatewayResponse) error
}

func Init(ctx context.Context, config *Config, logger schemas.Logger) (schemas.BasePlugin, error) {
	plugin := &GuardrailsPlugin{
		config:      config,
		logger:      logger,
		celPrograms: make(map[int]cel.Program),
		providers:   make(map[int]Provider),
	}

	if err := plugin.initializeProviders(); err != nil {
		logger.Error("failed to initialize guardrail providers: %v", err)
		return nil, err
	}
	if err := plugin.compileRules(); err != nil {
		logger.Error("failed to compile guardrail rules: %v", err)
		return nil, err
	}

	return plugin, nil
}

func (p *GuardrailsPlugin) initializeProviders() error {
	if p.config == nil {
		return nil
	}
	for _, providerCfg := range p.config.GuardrailProviders {
		if !providerCfg.Enabled {
			continue
		}

		switch providerCfg.ProviderName {
		case "regex":
			provider, err := NewRegexProvider(providerCfg)
			if err != nil {
				return fmt.Errorf("failed to initialize regex provider config %d: %w", providerCfg.ID, err)
			}
			p.providers[providerCfg.ID] = provider
		default:
			// For MVP, we ignore unknown providers instead of erroring
			continue
		}
	}
	return nil
}

func (p *GuardrailsPlugin) compileRules() error {
	if p.config == nil {
		return nil
	}

	// Create a CEL environment with variables for the request
	env, err := cel.NewEnv(
		cel.Variable("request.model", cel.StringType),
		cel.Variable("request.prompt_id", cel.StringType),
		cel.Variable("request.virtual_key_id", cel.StringType),
		cel.Variable("request.virtual_key_name", cel.StringType),
	)
	if err != nil {
		return fmt.Errorf("failed to create CEL env: %w", err)
	}

	for _, rule := range p.config.GuardrailRules {
		if !rule.Enabled {
			continue
		}

		ast, issues := env.Compile(rule.CELExpression)
		if issues != nil && issues.Err() != nil {
			return fmt.Errorf("invalid CEL expression for rule %d: %w", rule.ID, issues.Err())
		}

		prg, err := env.Program(ast)
		if err != nil {
			return fmt.Errorf("failed to create CEL program for rule %d: %w", rule.ID, err)
		}

		p.celPrograms[rule.ID] = prg
	}

	return nil
}

// GetName returns the name of the plugin
func (p *GuardrailsPlugin) GetName() string {
	return PluginName
}

// Cleanup cleans up the plugin resources
func (p *GuardrailsPlugin) Cleanup() error {
	return nil
}

func (p *GuardrailsPlugin) PreRequestHook(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) error {
	return nil
}

func (p *GuardrailsPlugin) PreLLMHook(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) (*schemas.GatewayRequest, *schemas.LLMPluginShortCircuit, error) {
	if p.config == nil {
		return nil, nil, nil
	}

	// Prepare CEL variables
	modelName := ""
	if req.ChatRequest != nil {
		modelName = req.ChatRequest.Model
	} else if req.ResponsesRequest != nil {
		modelName = req.ResponsesRequest.Model
	}
	if ctx != nil {
		ctx.SetValue(guardrailsRequestModelKey, modelName)
	}

	currentVKID := virtualKeyIDFromContext(ctx)
	rawVK := rawVirtualKeyFromContext(ctx)
	currentVKName := virtualKeyNameFromContext(ctx)

	vars := map[string]interface{}{
		"request.model":            modelName,
		"request.prompt_id":        promptIDFromContext(ctx),
		"request.virtual_key_id":   currentVKID,
		"request.virtual_key_name": currentVKName,
	}

	for _, rule := range p.config.GuardrailRules {
		if !rule.Enabled || (rule.ApplyTo != "input" && rule.ApplyTo != "both") {
			continue
		}

		if !ruleMatchesVirtualKey(rule, currentVKID, rawVK) {
			continue
		}

		prg, ok := p.celPrograms[rule.ID]
		if !ok {
			continue
		}

		out, _, err := prg.Eval(vars)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("guardrail CEL eval failed for rule %d (%s): %v", rule.ID, rule.Name, err)
			}
			continue
		}

		if match, ok := out.Value().(bool); ok && match {
			// Rule matched, run configured providers
			for _, providerID := range rule.ProviderConfigIDs {
				provider, ok := p.providers[providerID]
				if !ok {
					continue
				}

				if err := provider.ValidateInput(ctx, req); err != nil {
					return req, &schemas.LLMPluginShortCircuit{
						Error: guardrailViolationError(fmt.Sprintf("Guardrail %q blocked input: %s", rule.Name, err.Error())),
					}, nil
				}
			}
		}
	}

	return req, nil, nil
}

func (p *GuardrailsPlugin) PostLLMHook(ctx *schemas.GatewayContext, resp *schemas.GatewayResponse, err *schemas.GatewayError) (*schemas.GatewayResponse, *schemas.GatewayError, error) {
	if p.config == nil || err != nil || resp == nil {
		return resp, err, nil
	}

	if streamBlocked(ctx) {
		return nil, skipStreamChunkError(), nil
	}

	isStream, chunkText := streamChunkText(resp)
	scanWindow := ""
	if isStream && chunkText != "" {
		scanWindow = appendStreamWindow(ctx, chunkText)
	}

	currentVKID := virtualKeyIDFromContext(ctx)
	rawVK := rawVirtualKeyFromContext(ctx)
	currentVKName := virtualKeyNameFromContext(ctx)

	modelName := modelNameFromResponse(resp)
	if modelName == "" && ctx != nil {
		modelName, _ = ctx.Value(guardrailsRequestModelKey).(string)
	}
	vars := map[string]interface{}{
		"request.model":            modelName,
		"request.prompt_id":        promptIDFromContext(ctx),
		"request.virtual_key_id":   currentVKID,
		"request.virtual_key_name": currentVKName,
	}

	for _, rule := range p.config.GuardrailRules {
		if !rule.Enabled || (rule.ApplyTo != "output" && rule.ApplyTo != "both") {
			continue
		}

		if !ruleMatchesVirtualKey(rule, currentVKID, rawVK) {
			continue
		}

		prg, ok := p.celPrograms[rule.ID]
		if !ok {
			continue
		}

		out, _, evalErr := prg.Eval(vars)
		if evalErr != nil {
			if p.logger != nil {
				p.logger.Warn("guardrail CEL eval failed for rule %d (%s): %v", rule.ID, rule.Name, evalErr)
			}
			continue
		}

		if match, ok := out.Value().(bool); ok && match {
			for _, providerID := range rule.ProviderConfigIDs {
				provider, ok := p.providers[providerID]
				if !ok {
					continue
				}

				if !isStream {
					if validateErr := provider.ValidateOutput(ctx, nil, resp); validateErr != nil {
						return nil, guardrailViolationError(fmt.Sprintf("Guardrail %q blocked output: %s", rule.Name, validateErr.Error())), nil
					}
					continue
				}
				if scanWindow == "" {
					continue
				}
				if rp, ok := provider.(*RegexProvider); ok {
					if matchErr := rp.MatchText(scanWindow, "output"); matchErr != nil {
						// The matching chunk is replaced by this error and every later chunk is dropped,
						// so the completed match never reaches the client.
						ctx.SetValue(guardrailsStreamBlockedKey, true)
						return nil, guardrailViolationError(fmt.Sprintf("Guardrail %q blocked output: %s", rule.Name, matchErr.Error())), nil
					}
				}
			}
		}
	}

	return resp, err, nil
}

const (
	guardrailsStreamTailKey    schemas.GatewayContextKey = "guardrails.stream_output_tail"
	guardrailsStreamBlockedKey schemas.GatewayContextKey = "guardrails.stream_blocked"
	guardrailsRequestModelKey  schemas.GatewayContextKey = "guardrails.request_model"
)

// streamScanOverlap is how much already-streamed text is re-scanned with each new chunk,
// so a pattern split across chunks still matches. Patterns longer than this can be missed.
const streamScanOverlap = 4096

// streamChunkText reports whether resp is a streaming chunk and returns its output text
// (chat deltas, or Responses/Anthropic output_text and refusal deltas).
func streamChunkText(resp *schemas.GatewayResponse) (bool, string) {
	if resp == nil {
		return false, ""
	}
	if sr := resp.ResponsesStreamResponse; sr != nil {
		if sr.Type == schemas.ResponsesStreamResponseTypeOutputTextDelta || sr.Type == schemas.ResponsesStreamResponseTypeRefusalDelta {
			if sr.Delta != nil {
				return true, *sr.Delta
			}
		}
		return true, ""
	}
	if resp.ChatResponse == nil {
		return false, ""
	}
	isStream := false
	var sb strings.Builder
	for _, choice := range resp.ChatResponse.Choices {
		if choice.ChatStreamResponseChoice == nil {
			continue
		}
		isStream = true
		if delta := choice.ChatStreamResponseChoice.Delta; delta != nil {
			if delta.Content != nil {
				sb.WriteString(*delta.Content)
			}
			if delta.Refusal != nil {
				sb.WriteString(*delta.Refusal)
			}
		}
	}
	return isStream, sb.String()
}

// appendStreamWindow returns the text to scan for this chunk (recent tail + chunk) and
// keeps only the last streamScanOverlap bytes for the next chunk.
func appendStreamWindow(ctx *schemas.GatewayContext, chunk string) string {
	if ctx == nil {
		return chunk
	}
	prev, _ := ctx.Value(guardrailsStreamTailKey).(string)
	window := prev + chunk
	tail := window
	if len(tail) > streamScanOverlap {
		tail = tail[len(tail)-streamScanOverlap:]
	}
	ctx.SetValue(guardrailsStreamTailKey, tail)
	return window
}

func streamBlocked(ctx *schemas.GatewayContext) bool {
	if ctx == nil {
		return false
	}
	blocked, _ := ctx.Value(guardrailsStreamBlockedKey).(bool)
	return blocked
}

func skipStreamChunkError() *schemas.GatewayError {
	skip := true
	return &schemas.GatewayError{
		IsGatewayError: true,
		Error:         &schemas.ErrorField{Message: "stream blocked by guardrail"},
		StreamControl: &schemas.StreamControl{SkipStream: &skip},
	}
}

func promptIDFromContext(ctx *schemas.GatewayContext) string {
	if ctx == nil {
		return ""
	}
	if promptID := gateway.GetStringFromContext(ctx, prompts.PromptIDKey); promptID != "" {
		return promptID
	}
	return gateway.GetStringFromContext(ctx, schemas.GatewayContextKeySelectedPromptID)
}

func modelNameFromResponse(resp *schemas.GatewayResponse) string {
	if resp == nil {
		return ""
	}
	if resp.ChatResponse != nil && resp.ChatResponse.Model != "" {
		return resp.ChatResponse.Model
	}
	if resp.ResponsesResponse != nil && resp.ResponsesResponse.Model != "" {
		return resp.ResponsesResponse.Model
	}
	return ""
}

func guardrailViolationError(message string) *schemas.GatewayError {
	statusCode := 400
	code := "guardrail_violation"
	return &schemas.GatewayError{
		IsGatewayError: true,
		StatusCode:    &statusCode,
		Error: &schemas.ErrorField{
			Message: message,
			Code:    &code,
		},
	}
}

func virtualKeyIDFromContext(ctx *schemas.GatewayContext) string {
	if ctx == nil {
		return ""
	}
	if vkID := gateway.GetStringFromContext(ctx, schemas.GatewayContextKeyGovernanceVirtualKeyID); vkID != "" {
		return vkID
	}
	if v, ok := ctx.Value(schemas.GatewayContextKeyVirtualKey).(string); ok && v != "" {
		return v
	}
	return ""
}

func rawVirtualKeyFromContext(ctx *schemas.GatewayContext) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(schemas.GatewayContextKeyVirtualKey).(string); ok && v != "" {
		return v
	}
	return ""
}

func virtualKeyNameFromContext(ctx *schemas.GatewayContext) string {
	if ctx == nil {
		return ""
	}
	return gateway.GetStringFromContext(ctx, schemas.GatewayContextKeyGovernanceVirtualKeyName)
}

func ruleMatchesVirtualKey(rule GuardrailRule, currentVKID string, rawVK string) bool {
	if len(rule.VirtualKeyIDs) == 0 {
		return true // rule applies to all virtual keys
	}
	if currentVKID == "" && rawVK == "" {
		return false // rule requires specific virtual keys, but request has no virtual key
	}
	for _, id := range rule.VirtualKeyIDs {
		if (currentVKID != "" && id == currentVKID) || (rawVK != "" && id == rawVK) {
			return true
		}
	}
	return false
}
