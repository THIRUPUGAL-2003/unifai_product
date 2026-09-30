package vllm

import (
	"github.com/bytedance/sonic"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
)

func HandleVLLMResponse[T any](responseBody []byte, response *T, requestBody []byte, sendBackRawRequest bool, sendBackRawResponse bool) (rawRequest interface{}, rawResponse interface{}, unifaiErr *schemas.UnifAIError) {
	var errorResp schemas.UnifAIError
	rawRequest, rawResponse, unifaiErr = providerUtils.HandleProviderResponse(responseBody, response, requestBody, sendBackRawRequest, sendBackRawResponse)
	if unifaiErr != nil {
		return rawRequest, rawResponse, unifaiErr
	}
	if err := sonic.Unmarshal(responseBody, &errorResp); err == nil && errorResp.Error != nil && errorResp.Error.Message != "" {
		return rawRequest, rawResponse, &errorResp
	}
	return rawRequest, rawResponse, nil
}
