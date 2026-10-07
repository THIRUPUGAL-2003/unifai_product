package gemini

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/gateway/gateway/core/schemas"
)

// ToGeminiChatCompletionRequest converts a GatewayChatRequest to Gemini's generation request format for chat completion
func ToGeminiChatCompletionRequest(ctx *schemas.GatewayContext, gatewayReq *schemas.GatewayChatRequest) (*GeminiGenerationRequest, error) {
	return ToGeminiChatCompletionRequestWithImageURLSchemes(ctx, gatewayReq, defaultGeminiImageURLSchemes...)
}

// ToGeminiChatCompletionRequestWithImageURLSchemes converts a GatewayChatRequest
// to Gemini format using the provider-specific allowlist for non-data image URLs.
func ToGeminiChatCompletionRequestWithImageURLSchemes(ctx *schemas.GatewayContext, gatewayReq *schemas.GatewayChatRequest, allowedImageURLSchemes ...string) (*GeminiGenerationRequest, error) {
	if gatewayReq == nil {
		return nil, nil
	}

	gatewayReq.Model = NormalizeModelName(gatewayReq.Model)

	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: gatewayReq.Model,
	}

	// Canonical model for capability gating only; wire model is untouched.
	capModel := NormalizeModelName(schemas.ResolveCanonicalModel(ctx, gatewayReq.Model))

	// Convert parameters to generation config
	if gatewayReq.Params != nil {
		geminiReq.ExtraParams = gatewayReq.Params.ExtraParams
		var err error
		geminiReq.GenerationConfig, err = convertParamsToGenerationConfig(gatewayReq.Params, []string{}, capModel)
		if err != nil {
			return nil, err
		}
		// Handle tool-related parameters
		if len(gatewayReq.Params.Tools) > 0 {
			geminiReq.Tools, err = convertGatewayToolsToGemini(gatewayReq.Params.Tools)
			if err != nil {
				return nil, err
			}

			// Convert tool choice to tool config
			if gatewayReq.Params.ToolChoice != nil {
				geminiReq.ToolConfig = convertToolChoiceToToolConfig(gatewayReq.Params.ToolChoice)
			}
		}

		if gatewayReq.Params.ServiceTier != nil {
			geminiReq.ServiceTier = mapGatewayServiceTierToGemini(*gatewayReq.Params.ServiceTier)
		}

		// Handle extra parameters
		if gatewayReq.Params.ExtraParams != nil {
			// Safety settings
			if safetySettings, ok := schemas.SafeExtractFromMap(gatewayReq.Params.ExtraParams, "safety_settings"); ok {
				delete(geminiReq.ExtraParams, "safety_settings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			}

			// Cached content
			if cachedContent, ok := schemas.SafeExtractString(gatewayReq.Params.ExtraParams["cached_content"]); ok {
				delete(geminiReq.ExtraParams, "cached_content")
				geminiReq.CachedContent = cachedContent
			}

			// Labels
			if labels, ok := schemas.SafeExtractFromMap(gatewayReq.Params.ExtraParams, "labels"); ok {
				delete(geminiReq.ExtraParams, "labels")
				if labelMap, ok := schemas.SafeExtractStringMap(labels); ok {
					geminiReq.Labels = labelMap
				}
			}
		}
	}
	// Convert chat completion messages to Gemini format
	contents, systemInstruction, err := convertGatewayMessagesToGemini(gatewayReq.Input, allowedImageURLSchemes...)
	if err != nil {
		return nil, err
	}
	if systemInstruction != nil {
		geminiReq.SystemInstruction = systemInstruction
	}
	geminiReq.Contents = contents
	return geminiReq, nil
}

// ToGatewayChatResponse converts a GenerateContentResponse to a GatewayChatResponse
func (response *GenerateContentResponse) ToGatewayChatResponse() *schemas.GatewayChatResponse {
	gatewayResp := &schemas.GatewayChatResponse{
		ID:     response.ResponseID,
		Model:  response.ModelVersion,
		Object: "chat.completion",
	}

	// Set creation timestamp if available
	if !response.CreateTime.IsZero() {
		gatewayResp.Created = int(response.CreateTime.Unix())
	}

	// Handle empty candidates (filtered/malformed responses)
	if len(response.Candidates) == 0 {
		finishReason := ConvertGeminiFinishReasonToGateway(FinishReasonMalformedFunctionCall)
		return createErrorResponse(response, finishReason, false)
	}

	candidate := response.Candidates[0]

	// Check for filtered finish reasons that indicate errors
	if isErrorFinishReason(candidate.FinishReason) {
		finishReason := ConvertGeminiFinishReasonToGateway(candidate.FinishReason)
		return createErrorResponse(response, finishReason, false)
	}

	// Collect all content and tool calls into a single message
	var toolCalls []schemas.ChatAssistantMessageToolCall
	var contentBlocks []schemas.ChatContentBlock
	var reasoningDetails []schemas.ChatReasoningDetails
	var contentStr *string

	// Process candidate content to extract text, tool calls, and reasoning
	if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
		for _, part := range candidate.Content.Parts {
			// Handle thought/reasoning text separately - add to reasoning details
			if part.Text != "" && part.Thought {
				reasoningDetails = append(reasoningDetails, schemas.ChatReasoningDetails{
					Index: len(reasoningDetails),
					Type:  schemas.GatewayReasoningDetailsTypeText,
					Text:  &part.Text,
				})
				continue
			}
			// Handle regular text
			if part.Text != "" {
				contentBlocks = append(contentBlocks, schemas.ChatContentBlock{
					Type: schemas.ChatContentBlockTypeText,
					Text: &part.Text,
				})
				// Add thought signature to reasoning details if present with text
				if len(part.ThoughtSignature) > 0 {
					thoughtSig := base64.StdEncoding.EncodeToString(part.ThoughtSignature)
					reasoningDetails = append(reasoningDetails, schemas.ChatReasoningDetails{
						Index:     len(reasoningDetails),
						Type:      schemas.GatewayReasoningDetailsTypeEncrypted,
						Signature: &thoughtSig,
					})
				}
			}
			if part.FunctionCall != nil {
				function := schemas.ChatAssistantMessageToolCallFunction{
					Name: &part.FunctionCall.Name,
				}

				if len(part.FunctionCall.Args) > 0 {
					function.Arguments = string(part.FunctionCall.Args)
				}

				callID := part.FunctionCall.Name
				if part.FunctionCall.ID != "" {
					callID = part.FunctionCall.ID
				}

				// Embed thought signature into CallID if present (matches responses.go pattern)
				if len(part.ThoughtSignature) > 0 && !strings.Contains(callID, thoughtSignatureSeparator) {
					encoded := base64.RawURLEncoding.EncodeToString(part.ThoughtSignature)
					callID = fmt.Sprintf("%s%s%s", callID, thoughtSignatureSeparator, encoded)
				}

				toolCall := schemas.ChatAssistantMessageToolCall{
					Index:    uint16(len(toolCalls)),
					Type:     schemas.Ptr(string(schemas.ChatToolChoiceTypeFunction)),
					ID:       &callID,
					Function: function,
				}

				toolCalls = append(toolCalls, toolCall)

				// Also add to reasoning details for backward compatibility
				if len(part.ThoughtSignature) > 0 {
					thoughtSig := base64.StdEncoding.EncodeToString(part.ThoughtSignature)
					// Extract base ID without signature for reasoning detail lookup
					baseCallID := callID
					if strings.Contains(callID, thoughtSignatureSeparator) {
						parts := strings.SplitN(callID, thoughtSignatureSeparator, 2)
						if len(parts) == 2 {
							baseCallID = parts[0]
						}
					}
					reasoningDetails = append(reasoningDetails, schemas.ChatReasoningDetails{
						Index:     len(reasoningDetails),
						Type:      schemas.GatewayReasoningDetailsTypeEncrypted,
						Signature: &thoughtSig,
						ID:        schemas.Ptr(fmt.Sprintf("tool_call_%s", baseCallID)),
					})
				}
			}

			if part.FunctionResponse != nil {
				// Extract the output from the response
				output := extractFunctionResponseOutput(part.FunctionResponse)

				// Add as text content block
				if output != "" {
					contentBlocks = append(contentBlocks, schemas.ChatContentBlock{
						Type: schemas.ChatContentBlockTypeText,
						Text: &output,
					})
				}
			}

			// Handle code execution results
			if part.CodeExecutionResult != nil {
				output := part.CodeExecutionResult.Output
				if part.CodeExecutionResult.Outcome != OutcomeOK {
					output = "Error: " + output
				}
				if output != "" {
					contentBlocks = append(contentBlocks, schemas.ChatContentBlock{
						Type: schemas.ChatContentBlockTypeText,
						Text: &output,
					})
				}
			}

			// Handle executable code
			if part.ExecutableCode != nil {
				codeContent := "```" + part.ExecutableCode.Language + "\n" + part.ExecutableCode.Code + "\n```"
				contentBlocks = append(contentBlocks, schemas.ChatContentBlock{
					Type: schemas.ChatContentBlockTypeText,
					Text: &codeContent,
				})
			}

			// Handle standalone thought signature (not associated with function call or text)
			if len(part.ThoughtSignature) > 0 && part.FunctionCall == nil && part.Text == "" {
				thoughtSig := base64.StdEncoding.EncodeToString(part.ThoughtSignature)
				reasoningDetails = append(reasoningDetails, schemas.ChatReasoningDetails{
					Index:     len(reasoningDetails),
					Type:      schemas.GatewayReasoningDetailsTypeEncrypted,
					Signature: &thoughtSig,
				})
			}
		}

		// Build the choice with message
		message := &schemas.ChatMessage{
			Role: schemas.ChatMessageRoleAssistant,
		}

		if len(contentBlocks) == 1 && contentBlocks[0].Type == schemas.ChatContentBlockTypeText {
			contentStr = contentBlocks[0].Text
			contentBlocks = nil
		}

		message.Content = &schemas.ChatMessageContent{
			ContentStr:    contentStr,
			ContentBlocks: contentBlocks,
		}

		if len(toolCalls) > 0 || len(reasoningDetails) > 0 {
			message.ChatAssistantMessage = &schemas.ChatAssistantMessage{
				ToolCalls:        toolCalls,
				ReasoningDetails: reasoningDetails,
			}
		}

		// Convert finish reason to Gateway format.
		// Gemini uses "STOP" for both normal text completions and tool call responses —
		// it has no dedicated finish reason for tool calls. Override to "tool_calls" when
		// tool calls are present so downstream consumers see a uniform signal.
		finishReason := ConvertGeminiFinishReasonToGateway(candidate.FinishReason)
		if len(toolCalls) > 0 && finishReason == "stop" {
			finishReason = "tool_calls"
		}

		gatewayResp.Choices = append(gatewayResp.Choices, schemas.GatewayResponseChoice{
			Index:        0,
			FinishReason: &finishReason,
			LogProbs:     ConvertGeminiLogprobsResultToGateway(candidate.LogprobsResult),
			ChatNonStreamResponseChoice: &schemas.ChatNonStreamResponseChoice{
				Message: message,
			},
		})
	}

	// Set usage information
	gatewayResp.Usage = ConvertGeminiUsageMetadataToChatUsage(response.UsageMetadata)

	if response.UsageMetadata != nil {
		if t := mapGeminiTrafficTypeToGateway(response.UsageMetadata.TrafficType); t != nil {
			gatewayResp.ServiceTier = t
		} else if response.UsageMetadata.ServiceTier != "" {
			tier := mapGeminiServiceTierToGateway(response.UsageMetadata.ServiceTier)
			gatewayResp.ServiceTier = &tier
		}
	}

	return gatewayResp
}

// GeminiStreamState tracks tool-call index across streaming chunks.
type GeminiStreamState struct {
	nextToolCallIndex int
	hadToolCalls      bool // true if any tool calls were seen in this stream
}

// NewGeminiStreamState returns initialised stream state for one streaming response.
func NewGeminiStreamState() *GeminiStreamState {
	return &GeminiStreamState{}
}

// ToGatewayChatCompletionStream converts a Gemini streaming response to a Gateway Chat Completion Stream response
// Returns the response, error (if any), and a boolean indicating if this is the last chunk
func (response *GenerateContentResponse) ToGatewayChatCompletionStream(state *GeminiStreamState) (*schemas.GatewayChatResponse, *schemas.GatewayError, bool) {
	if response == nil {
		return nil, nil, false
	}

	if state == nil {
		state = NewGeminiStreamState()
	}

	// Handle empty candidates (filtered/malformed responses)
	if len(response.Candidates) == 0 {
		finishReason := ConvertGeminiFinishReasonToGateway(FinishReasonMalformedFunctionCall)
		return createErrorResponse(response, finishReason, true), nil, true
	}

	candidate := response.Candidates[0]

	// Check for filtered finish reasons that indicate errors
	if isErrorFinishReason(candidate.FinishReason) {
		finishReason := ConvertGeminiFinishReasonToGateway(candidate.FinishReason)
		return createErrorResponse(response, finishReason, true), nil, true
	}

	// Determine if this is the last chunk based on finish reason and usage metadata
	isLastChunk := candidate.FinishReason != "" && response.UsageMetadata != nil

	// Create the streaming response
	streamResponse := &schemas.GatewayChatResponse{
		ID:     response.ResponseID,
		Model:  response.ModelVersion,
		Object: "chat.completion.chunk",
	}

	// Set creation timestamp if available
	if !response.CreateTime.IsZero() {
		streamResponse.Created = int(response.CreateTime.Unix())
	}

	// Build delta content
	delta := &schemas.ChatStreamResponseChoiceDelta{}

	// Process content parts
	if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
		// Set role from the first chunk (Gemini uses "model" for assistant)
		if candidate.Content.Role != "" {
			role := candidate.Content.Role
			if role == string(RoleModel) {
				role = string(schemas.ChatMessageRoleAssistant)
			}
			delta.Role = &role
		}

		var textContent string
		var toolCalls []schemas.ChatAssistantMessageToolCall
		var reasoningDetails []schemas.ChatReasoningDetails

		for _, part := range candidate.Content.Parts {
			switch {
			case part.Text != "" && part.Thought:
				// Thought/reasoning content - add to reasoning details
				reasoningDetails = append(reasoningDetails, schemas.ChatReasoningDetails{
					Index: len(reasoningDetails),
					Type:  schemas.GatewayReasoningDetailsTypeText,
					Text:  &part.Text,
				})

			case part.Text != "":
				// Regular text content
				textContent += part.Text

			case part.FunctionCall != nil:
				// Function call
				jsonArgs := ""
				if len(part.FunctionCall.Args) > 0 {
					jsonArgs = string(part.FunctionCall.Args)
				}

				// Use ID if available, otherwise use function name
				callID := part.FunctionCall.Name
				if part.FunctionCall.ID != "" {
					callID = part.FunctionCall.ID
				}

				// Embed thought signature into CallID if present
				if len(part.ThoughtSignature) > 0 && !strings.Contains(callID, thoughtSignatureSeparator) {
					encoded := base64.RawURLEncoding.EncodeToString(part.ThoughtSignature)
					callID = fmt.Sprintf("%s%s%s", callID, thoughtSignatureSeparator, encoded)
				}

				toolCallIdx := state.nextToolCallIndex
				state.nextToolCallIndex++

				toolCall := schemas.ChatAssistantMessageToolCall{
					Index: uint16(toolCallIdx),
					Type:  schemas.Ptr(string(schemas.ChatToolTypeFunction)),
					ID:    &callID,
					Function: schemas.ChatAssistantMessageToolCallFunction{
						Name:      &part.FunctionCall.Name,
						Arguments: jsonArgs,
					},
				}

				toolCalls = append(toolCalls, toolCall)

				// Also add thought signature to reasoning details if present
				if len(part.ThoughtSignature) > 0 {
					thoughtSig := base64.StdEncoding.EncodeToString(part.ThoughtSignature)
					// Extract base ID without signature for reasoning detail lookup
					baseCallID := callID
					if strings.Contains(callID, thoughtSignatureSeparator) {
						parts := strings.SplitN(callID, thoughtSignatureSeparator, 2)
						if len(parts) == 2 {
							baseCallID = parts[0]
						}
					}
					reasoningDetails = append(reasoningDetails, schemas.ChatReasoningDetails{
						Index:     len(reasoningDetails),
						Type:      schemas.GatewayReasoningDetailsTypeEncrypted,
						Signature: &thoughtSig,
						ID:        schemas.Ptr(fmt.Sprintf("tool_call_%s", baseCallID)),
					})
				}

			case part.FunctionResponse != nil:
				// Extract the output from the response and add to text content
				output := extractFunctionResponseOutput(part.FunctionResponse)
				if output != "" {
					textContent += output
				}
			case part.CodeExecutionResult != nil:
				output := part.CodeExecutionResult.Output
				if part.CodeExecutionResult.Outcome != OutcomeOK {
					output = "Error: " + output
				}
				if output != "" {
					textContent += output
				}
			case part.ExecutableCode != nil:
				codeContent := "```" + part.ExecutableCode.Language + "\n" + part.ExecutableCode.Code + "\n```"
				textContent += codeContent
			}

			// Handle thought signature separately (not part of the switch since it can co-exist with other types)
			if len(part.ThoughtSignature) > 0 && part.FunctionCall == nil {
				thoughtSig := base64.StdEncoding.EncodeToString(part.ThoughtSignature)
				reasoningDetails = append(reasoningDetails, schemas.ChatReasoningDetails{
					Index:     len(reasoningDetails),
					Type:      schemas.GatewayReasoningDetailsTypeEncrypted,
					Signature: &thoughtSig,
				})
			}
		}

		// Set text content if present
		if textContent != "" {
			delta.Content = &textContent
		}

		// Set reasoning details if present
		if len(reasoningDetails) > 0 {
			delta.ReasoningDetails = reasoningDetails
		}

		// Set tool calls if present
		if len(toolCalls) > 0 {
			delta.ToolCalls = toolCalls
			state.hadToolCalls = true
		}
	}

	// Check if delta has any content - if not and it's not the last chunk, skip it
	hasDeltaContent := delta.Role != nil || delta.Content != nil || len(delta.ToolCalls) > 0 || len(delta.ReasoningDetails) > 0
	if !hasDeltaContent && !isLastChunk {
		return nil, nil, false
	}

	// Build the choice
	var finishReason *string
	if isLastChunk && candidate.FinishReason != "" {
		reason := ConvertGeminiFinishReasonToGateway(candidate.FinishReason)
		// Gemini uses "STOP" for both text completions and tool call responses.
		// Override to "tool_calls" when tool calls were seen in this stream for uniformity.
		if (len(delta.ToolCalls) > 0 || state.hadToolCalls) && reason == "stop" {
			reason = "tool_calls"
		}
		finishReason = &reason
	}

	choice := schemas.GatewayResponseChoice{
		Index:        int(candidate.Index),
		FinishReason: finishReason,
		LogProbs:     ConvertGeminiLogprobsResultToGateway(candidate.LogprobsResult),
		ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{
			Delta: delta,
		},
	}

	streamResponse.Choices = []schemas.GatewayResponseChoice{choice}

	// Add usage information if this is the last chunk
	if isLastChunk && response.UsageMetadata != nil {
		streamResponse.Usage = ConvertGeminiUsageMetadataToChatUsage(response.UsageMetadata)
		if t := mapGeminiTrafficTypeToGateway(response.UsageMetadata.TrafficType); t != nil {
			streamResponse.ServiceTier = t
		} else if response.UsageMetadata.ServiceTier != "" {
			tier := mapGeminiServiceTierToGateway(response.UsageMetadata.ServiceTier)
			streamResponse.ServiceTier = &tier
		}
	}

	return streamResponse, nil, isLastChunk
}

// isErrorFinishReason checks if a finish reason indicates a filtered or error response
func isErrorFinishReason(reason FinishReason) bool {
	return reason == FinishReasonSafety ||
		reason == FinishReasonRecitation ||
		reason == FinishReasonMalformedFunctionCall ||
		reason == FinishReasonBlocklist ||
		reason == FinishReasonProhibitedContent ||
		reason == FinishReasonSPII ||
		reason == FinishReasonImageSafety ||
		reason == FinishReasonUnexpectedToolCall ||
		reason == FinishReasonMissingThoughtSignature ||
		reason == FinishReasonMalformedResponse ||
		reason == FinishReasonImageProhibitedContent ||
		reason == FinishReasonImageRecitation ||
		reason == FinishReasonTooManyToolCalls ||
		reason == FinishReasonNoImage
}

// createErrorResponse creates a complete GatewayChatResponse for error cases
func createErrorResponse(response *GenerateContentResponse, finishReason string, isStream bool) *schemas.GatewayChatResponse {
	var choice schemas.GatewayResponseChoice
	if isStream {
		choice = schemas.GatewayResponseChoice{
			Index:        0,
			FinishReason: &finishReason,
			ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{
				Delta: &schemas.ChatStreamResponseChoiceDelta{},
			},
		}
	} else {
		choice = schemas.GatewayResponseChoice{
			Index:        0,
			FinishReason: &finishReason,
			ChatNonStreamResponseChoice: &schemas.ChatNonStreamResponseChoice{
				Message: &schemas.ChatMessage{
					Role:    schemas.ChatMessageRoleAssistant,
					Content: &schemas.ChatMessageContent{},
				},
			},
		}
	}

	objectType := "chat.completion"
	if isStream {
		objectType = "chat.completion.chunk"
	}

	errorResp := &schemas.GatewayChatResponse{
		ID:      response.ResponseID,
		Model:   response.ModelVersion,
		Object:  objectType,
		Choices: []schemas.GatewayResponseChoice{choice},
		Usage:   ConvertGeminiUsageMetadataToChatUsage(response.UsageMetadata),
	}

	if !response.CreateTime.IsZero() {
		errorResp.Created = int(response.CreateTime.Unix())
	}

	return errorResp
}
