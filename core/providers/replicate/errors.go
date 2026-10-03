package replicate

import (
	"github.com/bytedance/sonic"
	schemas "github.com/raksha/raksha/core/schemas"
)

// parseReplicateError parses Replicate API error response
func parseReplicateError(body []byte, statusCode int) *schemas.RakshaError {
	var replicateErr ReplicateError
	if err := sonic.Unmarshal(body, &replicateErr); err == nil && replicateErr.Detail != "" {
		return &schemas.RakshaError{
			IsRakshaError: false,
			StatusCode:     &statusCode,
			Error: &schemas.ErrorField{
				Message: replicateErr.Detail,
			},
		}
	}

	// Fallback to generic error
	return &schemas.RakshaError{
		IsRakshaError: false,
		StatusCode:     &statusCode,
		Error: &schemas.ErrorField{
			Message: string(body),
		},
	}
}
