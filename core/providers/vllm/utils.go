package vllm

import (
	"github.com/bytedance/sonic"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
)

func HandleVLLMResponse[T any](responseBody []byte, response *T, requestBody []byte, sendBackRawRequest bool, sendBackRawResponse bool) (rawRequest interface{}, rawResponse interface{}, rakshaErr *schemas.RakshaError) {
	var errorResp schemas.RakshaError
	rawRequest, rawResponse, rakshaErr = providerUtils.HandleProviderResponse(responseBody, response, requestBody, sendBackRawRequest, sendBackRawResponse)
	if rakshaErr != nil {
		return rawRequest, rawResponse, rakshaErr
	}
	if err := sonic.Unmarshal(responseBody, &errorResp); err == nil && errorResp.Error != nil && errorResp.Error.Message != "" {
		return rawRequest, rawResponse, &errorResp
	}
	return rawRequest, rawResponse, nil
}
