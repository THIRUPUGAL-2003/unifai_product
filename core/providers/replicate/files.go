package replicate

import (
	"time"

	"github.com/raksha/raksha/core/schemas"
)

// Replicate File API Converters

// ToRakshaFileStatus converts Replicate file status to Raksha file status.
// Replicate doesn't explicitly provide status, so we infer from the response.
func ToRakshaFileStatus(fileResp *ReplicateFileResponse) schemas.FileStatus {
	// If file has all required fields and is accessible, it's processed
	if fileResp.ID != "" && fileResp.Size > 0 {
		return schemas.FileStatusProcessed
	}
	return schemas.FileStatusUploaded
}

// ToRakshaFileUploadResponse converts Replicate file response to Raksha file upload response.
func (r *ReplicateFileResponse) ToRakshaFileUploadResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.RakshaFileUploadResponse {
	resp := &schemas.RakshaFileUploadResponse{
		ID:             r.ID,
		Object:         "file",
		Bytes:          r.Size,
		CreatedAt:      ParseReplicateTimestamp(r.CreatedAt),
		Filename:       r.Name,
		Purpose:        schemas.FilePurposeBatch, // Replicate uses files primarily for batch/general purposes
		Status:         ToRakshaFileStatus(r),
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.RakshaResponseExtraFields{
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

// ToRakshaFileRetrieveResponse converts Replicate file response to Raksha file retrieve response.
func (r *ReplicateFileResponse) ToRakshaFileRetrieveResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.RakshaFileRetrieveResponse {
	resp := &schemas.RakshaFileRetrieveResponse{
		ID:             r.ID,
		Object:         "file",
		Bytes:          r.Size,
		CreatedAt:      ParseReplicateTimestamp(r.CreatedAt),
		Filename:       r.Name,
		Purpose:        schemas.FilePurposeBatch,
		Status:         ToRakshaFileStatus(r),
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.RakshaResponseExtraFields{
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
