package openai

import (
	"bytes"
	"time"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// OpenAI File API Types

// OpenAIFileResponse represents an OpenAI file response.
type OpenAIFileResponse struct {
	ID            string              `json:"id"`
	Object        string              `json:"object"`
	Bytes         int64               `json:"bytes"`
	CreatedAt     int64               `json:"created_at"`
	Filename      string              `json:"filename"`
	Purpose       schemas.FilePurpose `json:"purpose"`
	Status        string              `json:"status,omitempty"`
	StatusDetails *string             `json:"status_details,omitempty"`
}

// OpenAIFileListResponse represents the response from listing files.
type OpenAIFileListResponse struct {
	Object  string               `json:"object"`
	Data    []OpenAIFileResponse `json:"data"`
	HasMore bool                 `json:"has_more,omitempty"`
}

// OpenAIFileDeleteResponse represents the response from deleting a file.
type OpenAIFileDeleteResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}

// ToUnifAIFileStatus converts OpenAI status to UnifAI status.
func ToUnifAIFileStatus(status string) schemas.FileStatus {
	switch status {
	case "uploaded":
		return schemas.FileStatusUploaded
	case "processed", "completed":
		return schemas.FileStatusProcessed
	case "processing", "in_progress":
		return schemas.FileStatusProcessing
	case "error", "failed":
		return schemas.FileStatusError
	case "deleted", "cancelled":
		return schemas.FileStatusDeleted
	default:
		return schemas.FileStatus(status)
	}
}

// ToUnifAIFileUploadResponse converts OpenAI file response to UnifAI file upload response.
func (r *OpenAIFileResponse) ToUnifAIFileUploadResponse(latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.UnifAIFileUploadResponse {
	resp := &schemas.UnifAIFileUploadResponse{
		ID:             r.ID,
		Object:         r.Object,
		Bytes:          r.Bytes,
		CreatedAt:      r.CreatedAt,
		Filename:       r.Filename,
		Purpose:        r.Purpose,
		Status:         ToUnifAIFileStatus(r.Status),
		StatusDetails:  r.StatusDetails,
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if sendBackRawRequest {
		resp.ExtraFields.RawRequest = rawRequest
	}

	if sendBackRawResponse {
		resp.ExtraFields.RawResponse = rawResponse
	}

	return resp
}

// ToUnifAIFileRetrieveResponse converts OpenAI file response to UnifAI file retrieve response.
func (r *OpenAIFileResponse) ToUnifAIFileRetrieveResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.UnifAIFileRetrieveResponse {
	resp := &schemas.UnifAIFileRetrieveResponse{
		ID:             r.ID,
		Object:         r.Object,
		Bytes:          r.Bytes,
		CreatedAt:      r.CreatedAt,
		Filename:       r.Filename,
		Purpose:        r.Purpose,
		Status:         ToUnifAIFileStatus(r.Status),
		StatusDetails:  r.StatusDetails,
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if sendBackRawRequest {
		resp.ExtraFields.RawRequest = rawRequest
	}

	if sendBackRawResponse {
		resp.ExtraFields.RawResponse = rawResponse
	}
	return resp
}

// ConvertRequestsToJSONL converts batch request items to JSONL format.
func ConvertRequestsToJSONL(requests []schemas.BatchRequestItem) ([]byte, error) {
	var buf bytes.Buffer
	for _, req := range requests {
		line, err := providerUtils.MarshalSorted(req)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}
