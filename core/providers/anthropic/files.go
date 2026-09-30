package anthropic

import (
	"time"

	"github.com/unifai/unifai/core/schemas"
)

// ToAnthropicFileUploadResponse converts a UnifAI file upload response to Anthropic format.
func ToAnthropicFileUploadResponse(resp *schemas.UnifAIFileUploadResponse) *AnthropicFileResponse {
	return &AnthropicFileResponse{
		ID:        resp.ID,
		Type:      resp.Object,
		Filename:  resp.Filename,
		MimeType:  "",
		SizeBytes: resp.Bytes,
		CreatedAt: formatAnthropicFileTimestamp(resp.CreatedAt),
	}
}

// ToAnthropicFileListResponse converts a UnifAI file list response to Anthropic format.
func ToAnthropicFileListResponse(resp *schemas.UnifAIFileListResponse) *AnthropicFileListResponse {
	data := make([]AnthropicFileResponse, len(resp.Data))
	for i, file := range resp.Data {
		data[i] = AnthropicFileResponse{
			ID:        file.ID,
			Type:      file.Object,
			Filename:  file.Filename,
			MimeType:  "",
			SizeBytes: file.Bytes,
			CreatedAt: formatAnthropicFileTimestamp(file.CreatedAt),
		}
	}

	return &AnthropicFileListResponse{
		Data:    data,
		HasMore: resp.HasMore,
	}
}

// ToAnthropicFileRetrieveResponse converts a UnifAI file retrieve response to Anthropic format.
func ToAnthropicFileRetrieveResponse(resp *schemas.UnifAIFileRetrieveResponse) *AnthropicFileResponse {
	return &AnthropicFileResponse{
		ID:        resp.ID,
		Type:      resp.Object,
		Filename:  resp.Filename,
		MimeType:  "", // Not supported in UnifAI responses
		SizeBytes: resp.Bytes,
		CreatedAt: formatAnthropicFileTimestamp(resp.CreatedAt),
	}
}

// ToAnthropicFileDeleteResponse converts a UnifAI file delete response to Anthropic format.
func ToAnthropicFileDeleteResponse(resp *schemas.UnifAIFileDeleteResponse) *AnthropicFileDeleteResponse {
	respType := "file"
	if resp.Deleted {
		respType = "file_deleted"
	}
	return &AnthropicFileDeleteResponse{
		ID:   resp.ID,
		Type: respType,
	}
}

// formatAnthropicFileTimestamp converts Unix timestamp to Anthropic ISO timestamp format.
func formatAnthropicFileTimestamp(unixTime int64) string {
	if unixTime == 0 {
		return ""
	}
	return time.Unix(unixTime, 0).UTC().Format(time.RFC3339)
}
