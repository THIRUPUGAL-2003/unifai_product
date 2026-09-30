// Package schemas defines the core schemas and types used by the UnifAI system.
package schemas

// FilePurpose represents the purpose of an uploaded file.
type FilePurpose string

const (
	FilePurposeBatch       FilePurpose = "batch"
	FilePurposeAssistants  FilePurpose = "assistants"
	FilePurposeFineTune    FilePurpose = "fine-tune"
	FilePurposeVision      FilePurpose = "vision"
	FilePurposeBatchOutput FilePurpose = "batch_output"
	FilePurposeUserData    FilePurpose = "user_data"
	FilePurposeResponses   FilePurpose = "responses"
	FilePurposeEvals       FilePurpose = "evals"
)

// FileStatus represents the status of a file.
type FileStatus string

const (
	FileStatusUploaded      FileStatus = "uploaded"
	FileStatusProcessed     FileStatus = "processed"
	FileStatusProcessing    FileStatus = "processing"
	FileStatusPendingUpload FileStatus = "pending_upload" // resumable session minted, bytes not yet received (Vertex)
	FileStatusError         FileStatus = "error"
	FileStatusDeleted       FileStatus = "deleted"
)

// FileStorageBackend represents the storage backend type.
type FileStorageBackend string

const (
	FileStorageAPI    FileStorageBackend = "api"    // OpenAI/Azure REST API
	FileStorageS3     FileStorageBackend = "s3"     // AWS S3
	FileStorageGCS    FileStorageBackend = "gcs"    // Google Cloud Storage
	FileStorageMemory FileStorageBackend = "memory" // In-memory (for Anthropic virtual files)
)

// FileObject represents a file object returned by the API.
type FileObject struct {
	ID            string      `json:"id"`
	Object        string      `json:"object,omitempty"` // "file"
	Bytes         int64       `json:"bytes"`
	CreatedAt     int64       `json:"created_at"`
	UpdatedAt     int64       `json:"updated_at,omitempty"`
	Filename      string      `json:"filename"`
	Purpose       FilePurpose `json:"purpose"`
	Status        FileStatus  `json:"status,omitempty"`
	StatusDetails *string     `json:"status_details,omitempty"`
	ExpiresAt     *int64      `json:"expires_at,omitempty"`
}

// UnifAIFileUploadRequest represents a request to upload a file.
type UnifAIFileUploadRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	// File content
	File        []byte      `json:"-"`                      // Raw file content (not serialized)
	Filename    string      `json:"filename"`               // Original filename
	Purpose     FilePurpose `json:"purpose"`                // Purpose of the file (e.g., "batch")
	ContentType *string     `json:"content_type,omitempty"` // MIME type of the file

	// Storage configuration (for S3/GCS backends)
	StorageConfig *FileStorageConfig `json:"storage_config,omitempty"`

	// Expiration configuration (OpenAI only)
	ExpiresAfter *FileExpiresAfter `json:"expires_after,omitempty"`

	// Extra parameters for provider-specific features
	ExtraParams map[string]interface{} `json:"-"`
}

// S3StorageConfig represents AWS S3 storage configuration.
type S3StorageConfig struct {
	Bucket string `json:"bucket,omitempty"`
	Region string `json:"region,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}

// GCSStorageConfig represents Google Cloud Storage configuration.
type GCSStorageConfig struct {
	Bucket  string `json:"bucket,omitempty"`
	Project string `json:"project,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
}

// FileExpiresAfter represents an expiration configuration for uploaded files.
type FileExpiresAfter struct {
	Anchor  string `json:"anchor"`  // e.g., "created_at"
	Seconds int    `json:"seconds"` // 3600-2592000 (1 hour to 30 days)
}

// FileStorageConfig represents storage configuration for cloud storage backends.
type FileStorageConfig struct {
	S3  *S3StorageConfig  `json:"s3,omitempty"`
	GCS *GCSStorageConfig `json:"gcs,omitempty"`
}

// UnifAIFileUploadResponse represents the response from uploading a file.
type UnifAIFileUploadResponse struct {
	ID            string      `json:"id"`
	Object        string      `json:"object,omitempty"` // "file"
	Bytes         int64       `json:"bytes"`
	CreatedAt     int64       `json:"created_at"`
	Filename      string      `json:"filename"`
	Purpose       FilePurpose `json:"purpose"`
	Status        FileStatus  `json:"status,omitempty"`
	StatusDetails *string     `json:"status_details,omitempty"`
	ExpiresAt     *int64      `json:"expires_at,omitempty"`

	// Storage backend info
	StorageBackend FileStorageBackend `json:"storage_backend,omitempty"`
	StorageURI     string             `json:"storage_uri,omitempty"` // S3/GCS URI if applicable

	// GCS resumable upload session URL (Vertex only, set when File bytes are not provided).
	// Client PUTs file bytes directly to this URL; UnifAI stays out of the data path.
	UploadURL *string `json:"upload_url,omitempty"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAIFileListRequest represents a request to list files.
type UnifAIFileListRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	RawRequestBody []byte `json:"-"` // Raw request body (not serialized)

	// Filters
	Purpose FilePurpose `json:"purpose,omitempty"` // Filter by purpose

	// Pagination
	Limit int     `json:"limit,omitempty"` // Max results to return
	After *string `json:"after,omitempty"` // Cursor for pagination
	Order *string `json:"order,omitempty"` // Sort order (asc/desc)

	// Storage configuration (for S3/GCS backends)
	StorageConfig *FileStorageConfig `json:"storage_config,omitempty"`

	// Extra parameters for provider-specific features
	ExtraParams map[string]interface{} `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (request *UnifAIFileListRequest) GetRawRequestBody() []byte {
	return request.RawRequestBody
}

// UnifAIFileListResponse represents the response from listing files.
type UnifAIFileListResponse struct {
	Object  string       `json:"object,omitempty"` // "list"
	Data    []FileObject `json:"data"`
	HasMore bool         `json:"has_more,omitempty"`
	After   *string      `json:"after,omitempty"` // Continuation token for pagination

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAIFileRetrieveRequest represents a request to retrieve file metadata.
type UnifAIFileRetrieveRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	RawRequestBody []byte `json:"-"` // Raw request body (not serialized)

	FileID string `json:"file_id"` // ID of the file to retrieve

	// Storage configuration (for S3/GCS backends)
	StorageConfig *FileStorageConfig `json:"storage_config,omitempty"`

	// Extra parameters for provider-specific features
	ExtraParams map[string]interface{} `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (request *UnifAIFileRetrieveRequest) GetRawRequestBody() []byte {
	return request.RawRequestBody
}

// UnifAIFileRetrieveResponse represents the response from retrieving file metadata.
type UnifAIFileRetrieveResponse struct {
	ID            string      `json:"id"`
	Object        string      `json:"object,omitempty"` // "file"
	Bytes         int64       `json:"bytes"`
	CreatedAt     int64       `json:"created_at"`
	UpdatedAt     int64       `json:"updated_at,omitempty"`
	Filename      string      `json:"filename"`
	Purpose       FilePurpose `json:"purpose"`
	Status        FileStatus  `json:"status,omitempty"`
	StatusDetails *string     `json:"status_details,omitempty"`
	ExpiresAt     *int64      `json:"expires_at,omitempty"`

	// Storage backend info
	StorageBackend FileStorageBackend `json:"storage_backend,omitempty"`
	StorageURI     string             `json:"storage_uri,omitempty"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAIFileDeleteRequest represents a request to delete a file.
type UnifAIFileDeleteRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`
	FileID   string        `json:"file_id"` // ID of the file to delete

	RawRequestBody []byte `json:"-"` // Raw request body (not serialized)

	// Storage configuration (for S3/GCS backends)
	StorageConfig *FileStorageConfig `json:"storage_config,omitempty"`

	// Extra parameters for provider-specific features
	ExtraParams map[string]interface{} `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (request *UnifAIFileDeleteRequest) GetRawRequestBody() []byte {
	return request.RawRequestBody
}

// UnifAIFileDeleteResponse represents the response from deleting a file.
type UnifAIFileDeleteResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"` // "file"
	Deleted bool   `json:"deleted"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAIFileContentRequest represents a request to download file content.
type UnifAIFileContentRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`
	FileID   string        `json:"file_id"` // ID of the file to download

	RawRequestBody []byte `json:"-"` // Raw request body (not serialized)

	// Storage configuration (for S3/GCS backends)
	StorageConfig *FileStorageConfig `json:"storage_config,omitempty"`

	// Extra parameters for provider-specific features
	ExtraParams map[string]interface{} `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (request *UnifAIFileContentRequest) GetRawRequestBody() []byte {
	return request.RawRequestBody
}

// UnifAIFileContentResponse represents the response from downloading file content.
type UnifAIFileContentResponse struct {
	FileID      string `json:"file_id"`
	Content     []byte `json:"-"`                      // Raw file content (not serialized)
	ContentType string `json:"content_type,omitempty"` // MIME type

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}
