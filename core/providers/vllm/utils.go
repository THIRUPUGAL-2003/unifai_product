package vllm

import (
	"github.com/bytedance/sonic"
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
)

func HandleVLLMResponse[T any](responseBody []byte, response *T, requestBody []byte, sendBackRawRequest bool, sendBackRawResponse bool) (rawRequest interface{}, rawResponse interface{}, gatewayErr *schemas.GatewayError) {
	var errorResp schemas.GatewayError
	rawRequest, rawResponse, gatewayErr = providerUtils.HandleProviderResponse(responseBody, response, requestBody, sendBackRawRequest, sendBackRawResponse)
	if gatewayErr != nil {
		return rawRequest, rawResponse, gatewayErr
	}
	if err := sonic.Unmarshal(responseBody, &errorResp); err == nil && errorResp.Error != nil && errorResp.Error.Message != "" {
		return rawRequest, rawResponse, &errorResp
	}
	return rawRequest, rawResponse, nil
}
