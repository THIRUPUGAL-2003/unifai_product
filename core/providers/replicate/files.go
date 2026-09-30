package replicate

import (
	"time"

	"github.com/unifai/unifai/core/schemas"
)

// Replicate File API Converters

// ToUnifAIFileStatus converts Replicate file status to UnifAI file status.
// Replicate doesn't explicitly provide status, so we infer from the response.
func ToUnifAIFileStatus(fileResp *ReplicateFileResponse) schemas.FileStatus {
	// If file has all required fields and is accessible, it's processed
	if fileResp.ID != "" && fileResp.Size > 0 {
		return schemas.FileStatusProcessed
	}
	return schemas.FileStatusUploaded
}

// ToUnifAIFileUploadResponse converts Replicate file response to UnifAI file upload response.
func (r *ReplicateFileResponse) ToUnifAIFileUploadResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.UnifAIFileUploadResponse {
	resp := &schemas.UnifAIFileUploadResponse{
		ID:             r.ID,
		Object:         "file",
		Bytes:          r.Size,
		CreatedAt:      ParseReplicateTimestamp(r.CreatedAt),
		Filename:       r.Name,
		Purpose:        schemas.FilePurposeBatch, // Replicate uses files primarily for batch/general purposes
		Status:         ToUnifAIFileStatus(r),
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency:     latency.Milliseconds(),
		},
	}

	// Add ExpiresAt if present
	if r.ExpiresAt != "" {
		expiresAt := ParseReplicateTimestamp(r.ExpiresAt)
		if expiresAt > 0 {
			resp.ExpiresAt = &expiresAt
		}
	}

	if sendBackRawRequest {
		resp.ExtraFields.RawRequest = rawRequest
	}

	if sendBackRawResponse {
		resp.ExtraFields.RawResponse = rawResponse
	}

	return resp
}

// ToUnifAIFileRetrieveResponse converts Replicate file response to UnifAI file retrieve response.
func (r *ReplicateFileResponse) ToUnifAIFileRetrieveResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.UnifAIFileRetrieveResponse {
	resp := &schemas.UnifAIFileRetrieveResponse{
		ID:             r.ID,
		Object:         "file",
		Bytes:          r.Size,
		CreatedAt:      ParseReplicateTimestamp(r.CreatedAt),
		Filename:       r.Name,
		Purpose:        schemas.FilePurposeBatch,
		Status:         ToUnifAIFileStatus(r),
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency:     latency.Milliseconds(),
		},
	}

	// Add ExpiresAt if present
	if r.ExpiresAt != "" {
		expiresAt := ParseReplicateTimestamp(r.ExpiresAt)
		if expiresAt > 0 {
			resp.ExpiresAt = &expiresAt
		}
	}

	if sendBackRawRequest {
		resp.ExtraFields.RawRequest = rawRequest
	}

	if sendBackRawResponse {
		resp.ExtraFields.RawResponse = rawResponse
	}

	return resp
}
