package huggingface

import (
	"fmt"

	"github.com/unifai/unifai/core/schemas"
)

// ToHuggingFaceResponsesRequest converts a UnifAI Responses request into the Hugging Face
// chat-completions payload that the provider already understands.
func ToHuggingFaceResponsesRequest(unifaiReq *schemas.UnifAIResponsesRequest) (*HuggingFaceChatRequest, error) {
	if unifaiReq == nil {
		return nil, nil
	}

	chatReq := unifaiReq.ToChatRequest()
	if chatReq == nil {
		return nil, fmt.Errorf("failed to convert responses request to chat request")
	}

	hfReq, err := ToHuggingFaceChatCompletionRequest(chatReq)
	if err != nil {
		return nil, err
	}
	if hfReq == nil {
		return nil, fmt.Errorf("failed to convert chat request to Hugging Face request")
	}

	return hfReq, nil
}

// ToUnifAIResponsesResponseFromHuggingFace converts a UnifAI chat response into the
// UnifAI Responses response shape, preserving provider metadata.
func ToUnifAIResponsesResponseFromHuggingFace(resp *schemas.UnifAIChatResponse, requestedModel string) (*schemas.UnifAIResponsesResponse, error) {
	if resp == nil {
		return nil, nil
	}

	// Ensure model is set
	if resp.Model == "" {
		resp.Model = requestedModel
	}

	responsesResp := resp.ToUnifAIResponsesResponse()
	if responsesResp != nil {
	}

	return responsesResp, nil
}
