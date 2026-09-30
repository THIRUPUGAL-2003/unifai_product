// Package schemas defines the core schemas and types used by the UnifAI system.
package schemas

// CachedContentObject represents a cached content resource as returned by the
// provider API (Gemini / Vertex AI). The `name` field is the canonical identifier:
//   - Google AI Studio: "cachedContents/{id}"
//   - Vertex AI:        "projects/{p}/locations/{l}/cachedContents/{id}"
type CachedContentObject struct {
	Name              string         `json:"name"`
	DisplayName       string         `json:"display_name,omitempty"`
	Model             string         `json:"model"`
	SystemInstruction any            `json:"system_instruction,omitempty"`
	Contents          []any          `json:"contents,omitempty"`
	Tools             []any          `json:"tools,omitempty"`
	ToolConfig        any            `json:"tool_config,omitempty"`
	CreateTime        string         `json:"create_time,omitempty"`
	UpdateTime        string         `json:"update_time,omitempty"`
	ExpireTime        string         `json:"expire_time,omitempty"`
	UsageMetadata     map[string]any `json:"usage_metadata,omitempty"`
}

// UnifAICachedContentCreateRequest creates a new cached content. TTL and
// ExpireTime are mutually exclusive — providers must error if both are set.
type UnifAICachedContentCreateRequest struct {
	Provider          ModelProvider `json:"provider"`
	Model             string        `json:"model"`
	DisplayName       *string       `json:"display_name,omitempty"`
	SystemInstruction any           `json:"system_instruction,omitempty"`
	Contents          []any         `json:"contents,omitempty"`
	Tools             []any         `json:"tools,omitempty"`
	ToolConfig        any           `json:"tool_config,omitempty"`
	TTL               *string       `json:"ttl,omitempty"`         // duration like "3600s"
	ExpireTime        *string       `json:"expire_time,omitempty"` // RFC3339 timestamp

	RawRequestBody []byte         `json:"-"`
	ExtraParams    map[string]any `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (r *UnifAICachedContentCreateRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// UnifAICachedContentCreateResponse is the response from creating a cached content.
type UnifAICachedContentCreateResponse struct {
	Name              string         `json:"name"`
	DisplayName       string         `json:"display_name,omitempty"`
	Model             string         `json:"model"`
	SystemInstruction any            `json:"system_instruction,omitempty"`
	Contents          []any          `json:"contents,omitempty"`
	Tools             []any          `json:"tools,omitempty"`
	ToolConfig        any            `json:"tool_config,omitempty"`
	CreateTime        string         `json:"create_time,omitempty"`
	UpdateTime        string         `json:"update_time,omitempty"`
	ExpireTime        string         `json:"expire_time,omitempty"`
	UsageMetadata     map[string]any `json:"usage_metadata,omitempty"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAICachedContentListRequest lists cached contents in the project.
type UnifAICachedContentListRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	// Pagination
	PageSize  int     `json:"page_size,omitempty"`
	PageToken *string `json:"page_token,omitempty"`

	RawRequestBody []byte         `json:"-"`
	ExtraParams    map[string]any `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (r *UnifAICachedContentListRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// UnifAICachedContentListResponse is the response from listing cached contents.
type UnifAICachedContentListResponse struct {
	CachedContents []CachedContentObject `json:"cached_contents"`
	NextPageToken  string                `json:"next_page_token,omitempty"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAICachedContentRetrieveRequest retrieves a single cached content by name.
type UnifAICachedContentRetrieveRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	// Name is the identifier of the cached content.
	//   - Google AI Studio: "cachedContents/{id}" or just "{id}"
	//   - Vertex AI:        "projects/{p}/locations/{l}/cachedContents/{id}" or just "{id}"
	Name string `json:"name"`

	RawRequestBody []byte         `json:"-"`
	ExtraParams    map[string]any `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (r *UnifAICachedContentRetrieveRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// UnifAICachedContentRetrieveResponse is the response from retrieving one cached content.
type UnifAICachedContentRetrieveResponse struct {
	Name              string         `json:"name"`
	DisplayName       string         `json:"display_name,omitempty"`
	Model             string         `json:"model"`
	SystemInstruction any            `json:"system_instruction,omitempty"`
	Contents          []any          `json:"contents,omitempty"`
	Tools             []any          `json:"tools,omitempty"`
	ToolConfig        any            `json:"tool_config,omitempty"`
	CreateTime        string         `json:"create_time,omitempty"`
	UpdateTime        string         `json:"update_time,omitempty"`
	ExpireTime        string         `json:"expire_time,omitempty"`
	UsageMetadata     map[string]any `json:"usage_metadata,omitempty"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAICachedContentUpdateRequest updates a cached content's expiration.
// Only TTL or ExpireTime may be set — they are mutually exclusive.
type UnifAICachedContentUpdateRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	// Name is the identifier of the cached content to update (see Retrieve.Name).
	Name string `json:"name"`

	TTL        *string `json:"ttl,omitempty"`
	ExpireTime *string `json:"expire_time,omitempty"`

	RawRequestBody []byte         `json:"-"`
	ExtraParams    map[string]any `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (r *UnifAICachedContentUpdateRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// UnifAICachedContentUpdateResponse is the response from updating a cached content.
type UnifAICachedContentUpdateResponse struct {
	Name              string         `json:"name"`
	DisplayName       string         `json:"display_name,omitempty"`
	Model             string         `json:"model"`
	SystemInstruction any            `json:"system_instruction,omitempty"`
	Contents          []any          `json:"contents,omitempty"`
	Tools             []any          `json:"tools,omitempty"`
	ToolConfig        any            `json:"tool_config,omitempty"`
	CreateTime        string         `json:"create_time,omitempty"`
	UpdateTime        string         `json:"update_time,omitempty"`
	ExpireTime        string         `json:"expire_time,omitempty"`
	UsageMetadata     map[string]any `json:"usage_metadata,omitempty"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}

// UnifAICachedContentDeleteRequest deletes a cached content by name.
type UnifAICachedContentDeleteRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	// Name is the identifier of the cached content to delete (see Retrieve.Name).
	Name string `json:"name"`

	RawRequestBody []byte         `json:"-"`
	ExtraParams    map[string]any `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (r *UnifAICachedContentDeleteRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// UnifAICachedContentDeleteResponse is the response from deleting a cached
// content. Providers typically return an empty body on success; this struct
// carries a Deleted flag set by unifai plus ExtraFields for diagnostics.
type UnifAICachedContentDeleteResponse struct {
	Name    string `json:"name,omitempty"`
	Deleted bool   `json:"deleted"`

	ExtraFields UnifAIResponseExtraFields `json:"extra_fields"`
}
