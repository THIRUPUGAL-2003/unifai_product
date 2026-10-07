package huggingface

import (
	"fmt"

	"github.com/gateway/gateway/core/schemas"
)

// ToHuggingFaceResponsesRequest converts a Gateway Responses request into the Hugging Face
// chat-completions payload that the provider already understands.
func ToHuggingFaceResponsesRequest(gatewayReq *schemas.GatewayResponsesRequest) (*HuggingFaceChatRequest, error) {
	if gatewayReq == nil {
		return nil, nil
	}

	chatReq := gatewayReq.ToChatRequest()
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

// ToGatewayResponsesResponseFromHuggingFace converts a Gateway chat response into the
// Gateway Responses response shape, preserving provider metadata.
func ToGatewayResponsesResponseFromHuggingFace(resp *schemas.GatewayChatResponse, requestedModel string) (*schemas.GatewayResponsesResponse, error) {
	if resp == nil {
		return nil, nil
	}

	// Ensure model is set
	if resp.Model == "" {
		resp.Model = requestedModel
	}

	responsesResp := resp.ToGatewayResponsesResponse()
	if responsesResp != nil {
	}

	return responsesResp, nil
}
