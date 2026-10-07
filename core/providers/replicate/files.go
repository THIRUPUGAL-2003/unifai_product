package replicate

import (
	"time"

	"github.com/gateway/gateway/core/schemas"
)

// Replicate File API Converters

// ToGatewayFileStatus converts Replicate file status to Gateway file status.
// Replicate doesn't explicitly provide status, so we infer from the response.
func ToGatewayFileStatus(fileResp *ReplicateFileResponse) schemas.FileStatus {
	// If file has all required fields and is accessible, it's processed
	if fileResp.ID != "" && fileResp.Size > 0 {
		return schemas.FileStatusProcessed
	}
	return schemas.FileStatusUploaded
}

// ToGatewayFileUploadResponse converts Replicate file response to Gateway file upload response.
func (r *ReplicateFileResponse) ToGatewayFileUploadResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.GatewayFileUploadResponse {
	resp := &schemas.GatewayFileUploadResponse{
		ID:             r.ID,
		Object:         "file",
		Bytes:          r.Size,
		CreatedAt:      ParseReplicateTimestamp(r.CreatedAt),
		Filename:       r.Name,
		Purpose:        schemas.FilePurposeBatch, // Replicate uses files primarily for batch/general purposes
		Status:         ToGatewayFileStatus(r),
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.GatewayResponseExtraFields{
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

// ToGatewayFileRetrieveResponse converts Replicate file response to Gateway file retrieve response.
func (r *ReplicateFileResponse) ToGatewayFileRetrieveResponse(providerName schemas.ModelProvider, latency time.Duration, sendBackRawRequest bool, sendBackRawResponse bool, rawRequest interface{}, rawResponse interface{}) *schemas.GatewayFileRetrieveResponse {
	resp := &schemas.GatewayFileRetrieveResponse{
		ID:             r.ID,
		Object:         "file",
		Bytes:          r.Size,
		CreatedAt:      ParseReplicateTimestamp(r.CreatedAt),
		Filename:       r.Name,
		Purpose:        schemas.FilePurposeBatch,
		Status:         ToGatewayFileStatus(r),
		StorageBackend: schemas.FileStorageAPI,
		ExtraFields: schemas.GatewayResponseExtraFields{
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
