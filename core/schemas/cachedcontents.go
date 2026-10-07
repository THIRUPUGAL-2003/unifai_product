// Package schemas defines the core schemas and types used by the Gateway system.
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

// GatewayCachedContentCreateRequest creates a new cached content. TTL and
// ExpireTime are mutually exclusive — providers must error if both are set.
type GatewayCachedContentCreateRequest struct {
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
func (r *GatewayCachedContentCreateRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// GatewayCachedContentCreateResponse is the response from creating a cached content.
type GatewayCachedContentCreateResponse struct {
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

	ExtraFields GatewayResponseExtraFields `json:"extra_fields"`
}

// GatewayCachedContentListRequest lists cached contents in the project.
type GatewayCachedContentListRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	// Pagination
	PageSize  int     `json:"page_size,omitempty"`
	PageToken *string `json:"page_token,omitempty"`

	RawRequestBody []byte         `json:"-"`
	ExtraParams    map[string]any `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (r *GatewayCachedContentListRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// GatewayCachedContentListResponse is the response from listing cached contents.
type GatewayCachedContentListResponse struct {
	CachedContents []CachedContentObject `json:"cached_contents"`
	NextPageToken  string                `json:"next_page_token,omitempty"`

	ExtraFields GatewayResponseExtraFields `json:"extra_fields"`
}

// GatewayCachedContentRetrieveRequest retrieves a single cached content by name.
type GatewayCachedContentRetrieveRequest struct {
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
func (r *GatewayCachedContentRetrieveRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// GatewayCachedContentRetrieveResponse is the response from retrieving one cached content.
type GatewayCachedContentRetrieveResponse struct {
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

	ExtraFields GatewayResponseExtraFields `json:"extra_fields"`
}

// GatewayCachedContentUpdateRequest updates a cached content's expiration.
// Only TTL or ExpireTime may be set — they are mutually exclusive.
type GatewayCachedContentUpdateRequest struct {
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
func (r *GatewayCachedContentUpdateRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// GatewayCachedContentUpdateResponse is the response from updating a cached content.
type GatewayCachedContentUpdateResponse struct {
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

	ExtraFields GatewayResponseExtraFields `json:"extra_fields"`
}

// GatewayCachedContentDeleteRequest deletes a cached content by name.
type GatewayCachedContentDeleteRequest struct {
	Provider ModelProvider `json:"provider"`
	Model    *string       `json:"model"`

	// Name is the identifier of the cached content to delete (see Retrieve.Name).
	Name string `json:"name"`

	RawRequestBody []byte         `json:"-"`
	ExtraParams    map[string]any `json:"-"`
}

// GetRawRequestBody returns the raw request body.
func (r *GatewayCachedContentDeleteRequest) GetRawRequestBody() []byte { return r.RawRequestBody }

// GatewayCachedContentDeleteResponse is the response from deleting a cached
// content. Providers typically return an empty body on success; this struct
// carries a Deleted flag set by gateway plus ExtraFields for diagnostics.
type GatewayCachedContentDeleteResponse struct {
	Name    string `json:"name,omitempty"`
	Deleted bool   `json:"deleted"`

	ExtraFields GatewayResponseExtraFields `json:"extra_fields"`
}
