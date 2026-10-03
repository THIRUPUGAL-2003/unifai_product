package replicate

import (
	"fmt"
	"strings"
	"time"

	"github.com/raksha/raksha/core/schemas"
)

func ToReplicateResponsesRequest(rakshaReq *schemas.RakshaResponsesRequest) (*ReplicatePredictionRequest, error) {
	if rakshaReq == nil {
		return nil, fmt.Errorf("raksha request is nil")
	}

	input := &ReplicatePredictionRequestInput{}

	if strings.HasPrefix(rakshaReq.Model, "openai/") && strings.Contains(rakshaReq.Model, "gpt-5-structured") {
		// handle responses style request
		if len(rakshaReq.Input) > 0 {
			input.InputItemList = rakshaReq.Input
		}
		if rakshaReq.Params != nil {
			if rakshaReq.Params.Instructions != nil {
				input.Instructions = rakshaReq.Params.Instructions
			}
			if rakshaReq.Params.Tools != nil {
				input.Tools = rakshaReq.Params.Tools
			}
			if rakshaReq.Params.MaxOutputTokens != nil {
				input.MaxOutputTokens = rakshaReq.Params.MaxOutputTokens
			}
			if rakshaReq.Params.Text != nil {
				input.JsonSchema = rakshaReq.Params.Text
			}
			if rakshaReq.Params.ExtraParams != nil {
				input.ExtraParams = rakshaReq.Params.ExtraParams
			}
		}
	} else {
		// handle chat style request (same logic as chat converter)
		if len(rakshaReq.Input) > 0 {
			// if model is from openai family, use messages
			if strings.HasPrefix(rakshaReq.Model, string(schemas.OpenAI)) {
				input.Messages = schemas.ToChatMessages(rakshaReq.Input)
			} else {
				// convert input to prompt and system prompt
				var systemPrompt string
				var conversationParts []string
				var imageInput []string

				for _, msg := range rakshaReq.Input {
					if msg.Content == nil {
						continue
					}

					// Get message content as string
					var contentStr string
					if msg.Content.ContentStr != nil {
						contentStr = *msg.Content.ContentStr
					} else if msg.Content.ContentBlocks != nil {
						// Concatenate text blocks only
						var textParts []string
						for _, block := range msg.Content.ContentBlocks {
							if block.Text != nil && *block.Text != "" {
								textParts = append(textParts, *block.Text)
							}
							if block.ResponsesInputMessageContentBlockImage != nil && block.ResponsesInputMessageContentBlockImage.ImageURL != nil && *block.ResponsesInputMessageContentBlockImage.ImageURL != "" {
								imageInput = append(imageInput, *block.ResponsesInputMessageContentBlockImage.ImageURL)
							}
						}
						contentStr = strings.Join(textParts, "\n")
					}

					if contentStr == "" {
						continue
					}

					// Handle different roles
					if msg.Role != nil {
						switch *msg.Role {
						case schemas.ResponsesInputMessageRoleSystem:
							if systemPrompt == "" {
								systemPrompt = contentStr
							} else {
								systemPrompt += "\n" + contentStr
							}
						case schemas.ResponsesInputMessageRoleUser:
							conversationParts = append(conversationParts, contentStr)
						case schemas.ResponsesInputMessageRoleAssistant:
							// For assistant messages, we can include them in the conversation context
							conversationParts = append(conversationParts, contentStr)
						}
					}
				}

				// Set system prompt if present and model supports it
				modelSupportsSystemPrompt := supportsSystemPrompt(rakshaReq.Model)

				if systemPrompt != "" {
					if modelSupportsSystemPrompt {
						// Model supports system_prompt field
						input.SystemPrompt = &systemPrompt
					} else {
						// Model doesn't support system_prompt - prepend to prompt
						if len(conversationParts) > 0 {
							// Prepend system prompt to conversation
							conversationParts = append([]string{systemPrompt}, conversationParts...)
						} else {
							// No conversation parts, use system prompt as the prompt
							conversationParts = []string{systemPrompt}
						}
					}
				}

				// Build the final prompt from conversation parts
				if len(conversationParts) > 0 {
					prompt := strings.Join(conversationParts, "\n\n")
					input.Prompt = &prompt
				}

				if len(imageInput) > 0 {
					input.ImageInput = imageInput
				}
			}
		}

		// Map parameters if present
		if rakshaReq.Params != nil {
			params := rakshaReq.Params

			// Temperature
			if params.Temperature != nil {
				input.Temperature = params.Temperature
			}

			// Top P
			if params.TopP != nil {
				input.TopP = params.TopP
			}

			// Max tokens - use max_completion_tokens if available
			if params.MaxOutputTokens != nil {
				if strings.HasPrefix(rakshaReq.Model, string(schemas.OpenAI)) {
					input.MaxCompletionTokens = params.MaxOutputTokens
				} else {
					input.MaxTokens = params.MaxOutputTokens
				}
			}

			// Reasoning effort
			if params.Reasoning != nil {
				if params.Reasoning.Effort != nil {
					input.ReasoningEffort = params.Reasoning.Effort
				}
			}

			if params.Instructions != nil && *params.Instructions != "" {
				if supportsSystemPrompt(rakshaReq.Model) {
					if input.SystemPrompt == nil {
						input.SystemPrompt = params.Instructions
					}
				} else {
					if input.Prompt != nil && *input.Prompt != "" {
						prefixed := *params.Instructions + "\n\n" + *input.Prompt
						input.Prompt = schemas.Ptr(prefixed)
					} else if input.Prompt == nil {
						input.Prompt = params.Instructions
					}
				}
			}

			if params.ExtraParams != nil {
				input.ExtraParams = params.ExtraParams
			}
		}
	}

	// Check if model is a version ID and set version field accordingly
	req := &ReplicatePredictionRequest{
		Input: input,
	}

	if isVersionID(rakshaReq.Model) {
		req.Version = &rakshaReq.Model
	}

	if rakshaReq.Params != nil && rakshaReq.Params.ExtraParams != nil {
		if webhook, ok := schemas.SafeExtractStringPointer(rakshaReq.Params.ExtraParams["webhook"]); ok {
			req.Webhook = webhook
		}
		if webhookEventsFilter, ok := schemas.SafeExtractStringSlice(rakshaReq.Params.ExtraParams["webhook_events_filter"]); ok {
			req.WebhookEventsFilter = webhookEventsFilter
		}
	}

	return req, nil
}

func (response *ReplicatePredictionResponse) ToRakshaResponsesResponse() *schemas.RakshaResponsesResponse {
	if response == nil {
		return nil
	}

	// Parse timestamps
	createdAt := ParseReplicateTimestamp(response.CreatedAt)
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	var completedAt *int
	if response.CompletedAt != nil {
		completed := int(ParseReplicateTimestamp(*response.CompletedAt))
		if completed > 0 {
			completedAt = &completed
		}
	}

	// Initialize Raksha response
	rakshaResponse := &schemas.RakshaResponsesResponse{
		ID:          schemas.Ptr(response.ID),
		Model:       response.Model,
		CreatedAt:   int(createdAt),
		CompletedAt: completedAt,
	}

	// Convert output to ResponsesMessage
	var outputMessages []schemas.ResponsesMessage
	if response.Output != nil {
		var contentStr *string

		// Handle different output types
		if response.Output.OutputStr != nil {
			contentStr = response.Output.OutputStr
		} else if response.Output.OutputArray != nil {
			// Join array of strings into a single string
			joined := strings.Join(response.Output.OutputArray, "")
			contentStr = &joined
		} else if response.Output.OutputObject != nil && response.Output.OutputObject.Text != nil {
			// Use text field from OutputObject
			contentStr = response.Output.OutputObject.Text
		}

		if contentStr != nil && *contentStr != "" {
			messageType := schemas.ResponsesMessageTypeMessage
			role := schemas.ResponsesInputMessageRoleAssistant

			outputMsg := schemas.ResponsesMessage{
				Type: &messageType,
				Role: &role,
				Content: &schemas.ResponsesMessageContent{
					ContentStr: contentStr,
				},
			}
			outputMessages = append(outputMessages, outputMsg)
		}
	}

	rakshaResponse.Output = outputMessages

	// Set status based on prediction status
	var status string
	switch response.Status {
	case ReplicatePredictionStatusSucceeded:
		status = "completed"
	case ReplicatePredictionStatusFailed:
		status = "failed"
	case ReplicatePredictionStatusCanceled:
		status = "cancelled"
	case ReplicatePredictionStatusProcessing:
		status = "in_progress"
	case ReplicatePredictionStatusStarting:
		status = "queued"
	default:
		status = string(response.Status)
	}
	rakshaResponse.Status = &status

	// Set error if present
	if response.Error != nil && *response.Error != "" {
		rakshaResponse.Error = &schemas.ResponsesResponseError{
			Code:    "provider_error",
			Message: *response.Error,
		}
	}

	// Extract usage information from logs
	if response.Logs != nil {
		inputTokens, outputTokens, totalTokens, found := parseTokenUsageFromLogs(response.Logs, schemas.ResponsesRequest)
		if found {
			rakshaResponse.Usage = &schemas.ResponsesResponseUsage{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				TotalTokens:  totalTokens,
			}
		}
	}

	return rakshaResponse
}
