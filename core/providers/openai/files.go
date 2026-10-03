package openai

import (
	"bytes"
	"time"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
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

// ToRakshaFileStatus converts OpenAI status to Raksha status.
func ToRakshaFileStatus(status string) schemas.FileStatus {
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

// ToRakshaFileUploadResponse converts OpenAI file response to Raksha file upload response.
func (r *OpenAIFileResponse) ToRakshaFileUploadResponse(latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.RakshaFileUploadResponse {
	resp := &schemas.RakshaFileUploadResponse{
		ID:             r.ID,
		Object:         r.Object,
		Bytes:          r.Bytes,
		CreatedAt:      r.CreatedAt,
		Filename:       r.Filename,
		Purpose:        r.Purpose,
		Status:         ToRakshaFileStatus(r.Status),
		StatusDetails:  r.StatusDetails,
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.RakshaResponseExtraFields{
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

// ToRakshaFileRetrieveResponse converts OpenAI file response to Raksha file retrieve response.
func (r *OpenAIFileResponse) ToRakshaFileRetrieveResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.RakshaFileRetrieveResponse {
	resp := &schemas.RakshaFileRetrieveResponse{
		ID:             r.ID,
		Object:         r.Object,
		Bytes:          r.Bytes,
		CreatedAt:      r.CreatedAt,
		Filename:       r.Filename,
		Purpose:        r.Purpose,
		Status:         ToRakshaFileStatus(r.Status),
		StatusDetails:  r.StatusDetails,
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.RakshaResponseExtraFields{
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
