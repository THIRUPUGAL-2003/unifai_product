package huggingface

import (
	"fmt"

	"github.com/raksha/raksha/core/schemas"
)

// ToHuggingFaceResponsesRequest converts a Raksha Responses request into the Hugging Face
// chat-completions payload that the provider already understands.
func ToHuggingFaceResponsesRequest(rakshaReq *schemas.RakshaResponsesRequest) (*HuggingFaceChatRequest, error) {
	if rakshaReq == nil {
		return nil, nil
	}

	chatReq := rakshaReq.ToChatRequest()
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

// ToRakshaResponsesResponseFromHuggingFace converts a Raksha chat response into the
// Raksha Responses response shape, preserving provider metadata.
func ToRakshaResponsesResponseFromHuggingFace(resp *schemas.RakshaChatResponse, requestedModel string) (*schemas.RakshaResponsesResponse, error) {
	if resp == nil {
		return nil, nil
	}

	// Ensure model is set
	if resp.Model == "" {
		resp.Model = requestedModel
	}

	responsesResp := resp.ToRakshaResponsesResponse()
	if responsesResp != nil {
	}

	return responsesResp, nil
}
