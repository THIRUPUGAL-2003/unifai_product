package guardrails

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/plugins/prompts"
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
	ValidateInput(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) error
	ValidateOutput(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest, resp *schemas.UnifAIResponse) error
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

func (p *GuardrailsPlugin) PreRequestHook(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) error {
	return nil
}

func (p *GuardrailsPlugin) PreLLMHook(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (*schemas.UnifAIRequest, *schemas.LLMPluginShortCircuit, error) {
	if p.config == nil {
		return nil, nil, nil
	}

	// Prepare CEL variables
	modelName := ""
	if req.ChatRequest != nil {
		modelName = req.ChatRequest.Model
	}
	
	vars := map[string]interface{}{
		"request.model":     modelName,
		"request.prompt_id": promptIDFromContext(ctx),
	}

	for _, rule := range p.config.GuardrailRules {
		if !rule.Enabled || (rule.ApplyTo != "input" && rule.ApplyTo != "both") {
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

func (p *GuardrailsPlugin) PostLLMHook(ctx *schemas.UnifAIContext, resp *schemas.UnifAIResponse, err *schemas.UnifAIError) (*schemas.UnifAIResponse, *schemas.UnifAIError, error) {
	if p.config == nil || err != nil || resp == nil {
		return resp, err, nil
	}

	accumulateStreamOutput(ctx, resp)

	vars := map[string]interface{}{
		"request.model":     modelNameFromResponse(resp),
		"request.prompt_id": promptIDFromContext(ctx),
	}

	streamFinished := chatStreamFinished(resp)
	accumulated := ""
	if streamFinished {
		if v, ok := ctx.Value(guardrailsStreamAccumKey).(string); ok {
			accumulated = v
		}
	}

	for _, rule := range p.config.GuardrailRules {
		if !rule.Enabled || (rule.ApplyTo != "output" && rule.ApplyTo != "both") {
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

				if validateErr := provider.ValidateOutput(ctx, nil, resp); validateErr != nil {
					return nil, guardrailViolationError(fmt.Sprintf("Guardrail %q blocked output: %s", rule.Name, validateErr.Error())), nil
				}

				// Streaming playground path: validate full accumulated text on final chunk.
				if streamFinished && accumulated != "" {
					if rp, ok := provider.(*RegexProvider); ok {
						if matchErr := rp.MatchText(accumulated, "output"); matchErr != nil {
							return nil, guardrailViolationError(fmt.Sprintf("Guardrail %q blocked output: %s", rule.Name, matchErr.Error())), nil
						}
					}
				}
			}
		}
	}

	return resp, err, nil
}

const guardrailsStreamAccumKey schemas.UnifAIContextKey = "guardrails.stream_output_accum"

func accumulateStreamOutput(ctx *schemas.UnifAIContext, resp *schemas.UnifAIResponse) {
	if ctx == nil || resp == nil || resp.ChatResponse == nil {
		return
	}
	for _, choice := range resp.ChatResponse.Choices {
		if choice.ChatStreamResponseChoice == nil || choice.ChatStreamResponseChoice.Delta == nil {
			continue
		}
		delta := choice.ChatStreamResponseChoice.Delta
		chunk := ""
		if delta.Content != nil {
			chunk += *delta.Content
		}
		if delta.Refusal != nil {
			chunk += *delta.Refusal
		}
		if chunk == "" {
			continue
		}
		prev, _ := ctx.Value(guardrailsStreamAccumKey).(string)
		ctx.SetValue(guardrailsStreamAccumKey, prev+chunk)
	}
}

func chatStreamFinished(resp *schemas.UnifAIResponse) bool {
	if resp == nil || resp.ChatResponse == nil {
		return false
	}
	for _, choice := range resp.ChatResponse.Choices {
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			return true
		}
		if choice.ChatStreamResponseChoice != nil {
			// Some providers omit finish_reason on intermediate chunks only.
			continue
		}
	}
	return false
}

func promptIDFromContext(ctx *schemas.UnifAIContext) string {
	if ctx == nil {
		return ""
	}
	if promptID := unifai.GetStringFromContext(ctx, prompts.PromptIDKey); promptID != "" {
		return promptID
	}
	return unifai.GetStringFromContext(ctx, schemas.UnifAIContextKeySelectedPromptID)
}

func modelNameFromResponse(resp *schemas.UnifAIResponse) string {
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

func guardrailViolationError(message string) *schemas.UnifAIError {
	statusCode := 400
	code := "guardrail_violation"
	return &schemas.UnifAIError{
		IsUnifAIError: true,
		StatusCode:    &statusCode,
		Error: &schemas.ErrorField{
			Message: message,
			Code:    &code,
		},
	}
}
