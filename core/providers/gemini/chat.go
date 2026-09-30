package gemini

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/unifai/unifai/core/schemas"
)

// ToGeminiChatCompletionRequest converts a UnifAIChatRequest to Gemini's generation request format for chat completion
func ToGeminiChatCompletionRequest(ctx *schemas.UnifAIContext, unifaiReq *schemas.UnifAIChatRequest) (*GeminiGenerationRequest, error) {
	return ToGeminiChatCompletionRequestWithImageURLSchemes(ctx, unifaiReq, defaultGeminiImageURLSchemes...)
}

// ToGeminiChatCompletionRequestWithImageURLSchemes converts a UnifAIChatRequest
// to Gemini format using the provider-specific allowlist for non-data image URLs.
func ToGeminiChatCompletionRequestWithImageURLSchemes(ctx *schemas.UnifAIContext, unifaiReq *schemas.UnifAIChatRequest, allowedImageURLSchemes ...string) (*GeminiGenerationRequest, error) {
	if unifaiReq == nil {
		return nil, nil
	}

	unifaiReq.Model = NormalizeModelName(unifaiReq.Model)

	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: unifaiReq.Model,
	}

	// Canonical model for capability gating only; wire model is untouched.
	capModel := NormalizeModelName(schemas.ResolveCanonicalModel(ctx, unifaiReq.Model))

	// Convert parameters to generation config
	if unifaiReq.Params != nil {
		geminiReq.ExtraParams = unifaiReq.Params.ExtraParams
		var err error
		geminiReq.GenerationConfig, err = convertParamsToGenerationConfig(unifaiReq.Params, []string{}, capModel)
		if err != nil {
			return nil, err
		}
		// Handle tool-related parameters
		if len(unifaiReq.Params.Tools) > 0 {
			geminiReq.Tools, err = convertUnifAIToolsToGemini(unifaiReq.Params.Tools)
			if err != nil {
				return nil, err
			}

			// Convert tool choice to tool config
			if unifaiReq.Params.ToolChoice != nil {
				geminiReq.ToolConfig = convertToolChoiceToToolConfig(unifaiReq.Params.ToolChoice)
			}
		}

		if unifaiReq.Params.ServiceTier != nil {
			geminiReq.ServiceTier = mapUnifAIServiceTierToGemini(*unifaiReq.Params.ServiceTier)
		}

		// Handle extra parameters
		if unifaiReq.Params.ExtraParams != nil {
			// Safety settings
			if safetySettings, ok := schemas.SafeExtractFromMap(unifaiReq.Params.ExtraParams, "safety_settings"); ok {
				delete(geminiReq.ExtraParams, "safety_settings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			}

			// Cached content
			if cachedContent, ok := schemas.SafeExtractString(unifaiReq.Params.ExtraParams["cached_content"]); ok {
				delete(geminiReq.ExtraParams, "cached_content")
				geminiReq.CachedContent = cachedContent
			}

			// Labels
			if labels, ok := schemas.SafeExtractFromMap(unifaiReq.Params.ExtraParams, "labels"); ok {
				delete(geminiReq.ExtraParams, "labels")
				if labelMap, ok := schemas.SafeExtractStringMap(labels); ok {
					geminiReq.Labels = labelMap
				}
			}
		}
	}
	// Convert chat completion messages to Gemini format
	contents, systemInstruction, err := convertUnifAIMessagesToGemini(unifaiReq.Input, allowedImageURLSchemes...)
	if err != nil {
		return nil, err
	}
	if systemInstruction != nil {
		geminiReq.SystemInstruction = systemInstruction
	}
	geminiReq.Contents = contents
	return geminiReq, nil
}

// ToUnifAIChatResponse converts a GenerateContentResponse to a UnifAIChatResponse
func (response *GenerateContentResponse) ToUnifAIChatResponse() *schemas.UnifAIChatResponse {
	unifaiResp := &schemas.UnifAIChatResponse{
		ID:     response.ResponseID,
		Model:  response.ModelVersion,
		Object: "chat.completion",
	}

	// Set creation timestamp if available
	if !response.CreateTime.IsZero() {
		unifaiResp.Created = int(response.CreateTime.Unix())
	}

	// Handle empty candidates (filtered/malformed responses)
	if len(response.Candidates) == 0 {
		finishReason := ConvertGeminiFinishReasonToUnifAI(FinishReasonMalformedFunctionCall)
		return createErrorResponse(response, finishReason, false)
	}

	candidate := response.Candidates[0]

	// Check for filtered finish reasons that indicate errors
	if isErrorFinishReason(candidate.FinishReason) {
		finishReason := ConvertGeminiFinishReasonToUnifAI(candidate.FinishReason)
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
					Type:  schemas.UnifAIReasoningDetailsTypeText,
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
						Type:      schemas.UnifAIReasoningDetailsTypeEncrypted,
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
						Type:      schemas.UnifAIReasoningDetailsTypeEncrypted,
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
					Type:      schemas.UnifAIReasoningDetailsTypeEncrypted,
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

		// Convert finish reason to UnifAI format.
		// Gemini uses "STOP" for both normal text completions and tool call responses —
		// it has no dedicated finish reason for tool calls. Override to "tool_calls" when
		// tool calls are present so downstream consumers see a uniform signal.
		finishReason := ConvertGeminiFinishReasonToUnifAI(candidate.FinishReason)
		if len(toolCalls) > 0 && finishReason == "stop" {
			finishReason = "tool_calls"
		}

		unifaiResp.Choices = append(unifaiResp.Choices, schemas.UnifAIResponseChoice{
			Index:        0,
			FinishReason: &finishReason,
			LogProbs:     ConvertGeminiLogprobsResultToUnifAI(candidate.LogprobsResult),
			ChatNonStreamResponseChoice: &schemas.ChatNonStreamResponseChoice{
				Message: message,
			},
		})
	}

	// Set usage information
	unifaiResp.Usage = ConvertGeminiUsageMetadataToChatUsage(response.UsageMetadata)

	if response.UsageMetadata != nil {
		if t := mapGeminiTrafficTypeToUnifAI(response.UsageMetadata.TrafficType); t != nil {
			unifaiResp.ServiceTier = t
		} else if response.UsageMetadata.ServiceTier != "" {
			tier := mapGeminiServiceTierToUnifAI(response.UsageMetadata.ServiceTier)
			unifaiResp.ServiceTier = &tier
		}
	}

	return unifaiResp
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

// ToUnifAIChatCompletionStream converts a Gemini streaming response to a UnifAI Chat Completion Stream response
// Returns the response, error (if any), and a boolean indicating if this is the last chunk
func (response *GenerateContentResponse) ToUnifAIChatCompletionStream(state *GeminiStreamState) (*schemas.UnifAIChatResponse, *schemas.UnifAIError, bool) {
	if response == nil {
		return nil, nil, false
	}

	if state == nil {
		state = NewGeminiStreamState()
	}

	// Handle empty candidates (filtered/malformed responses)
	if len(response.Candidates) == 0 {
		finishReason := ConvertGeminiFinishReasonToUnifAI(FinishReasonMalformedFunctionCall)
		return createErrorResponse(response, finishReason, true), nil, true
	}

	candidate := response.Candidates[0]

	// Check for filtered finish reasons that indicate errors
	if isErrorFinishReason(candidate.FinishReason) {
		finishReason := ConvertGeminiFinishReasonToUnifAI(candidate.FinishReason)
		return createErrorResponse(response, finishReason, true), nil, true
	}

	// Determine if this is the last chunk based on finish reason and usage metadata
	isLastChunk := candidate.FinishReason != "" && response.UsageMetadata != nil

	// Create the streaming response
	streamResponse := &schemas.UnifAIChatResponse{
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
					Type:  schemas.UnifAIReasoningDetailsTypeText,
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
						Type:      schemas.UnifAIReasoningDetailsTypeEncrypted,
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
					Type:      schemas.UnifAIReasoningDetailsTypeEncrypted,
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
		reason := ConvertGeminiFinishReasonToUnifAI(candidate.FinishReason)
		// Gemini uses "STOP" for both text completions and tool call responses.
		// Override to "tool_calls" when tool calls were seen in this stream for uniformity.
		if (len(delta.ToolCalls) > 0 || state.hadToolCalls) && reason == "stop" {
			reason = "tool_calls"
		}
		finishReason = &reason
	}

	choice := schemas.UnifAIResponseChoice{
		Index:        int(candidate.Index),
		FinishReason: finishReason,
		LogProbs:     ConvertGeminiLogprobsResultToUnifAI(candidate.LogprobsResult),
		ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{
			Delta: delta,
		},
	}

	streamResponse.Choices = []schemas.UnifAIResponseChoice{choice}

	// Add usage information if this is the last chunk
	if isLastChunk && response.UsageMetadata != nil {
		streamResponse.Usage = ConvertGeminiUsageMetadataToChatUsage(response.UsageMetadata)
		if t := mapGeminiTrafficTypeToUnifAI(response.UsageMetadata.TrafficType); t != nil {
			streamResponse.ServiceTier = t
		} else if response.UsageMetadata.ServiceTier != "" {
			tier := mapGeminiServiceTierToUnifAI(response.UsageMetadata.ServiceTier)
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

// createErrorResponse creates a complete UnifAIChatResponse for error cases
func createErrorResponse(response *GenerateContentResponse, finishReason string, isStream bool) *schemas.UnifAIChatResponse {
	var choice schemas.UnifAIResponseChoice
	if isStream {
		choice = schemas.UnifAIResponseChoice{
			Index:        0,
			FinishReason: &finishReason,
			ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{
				Delta: &schemas.ChatStreamResponseChoiceDelta{},
			},
		}
	} else {
		choice = schemas.UnifAIResponseChoice{
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

	errorResp := &schemas.UnifAIChatResponse{
		ID:      response.ResponseID,
		Model:   response.ModelVersion,
		Object:  objectType,
		Choices: []schemas.UnifAIResponseChoice{choice},
		Usage:   ConvertGeminiUsageMetadataToChatUsage(response.UsageMetadata),
	}

	if !response.CreateTime.IsZero() {
		errorResp.Created = int(response.CreateTime.Unix())
	}

	return errorResp
}
