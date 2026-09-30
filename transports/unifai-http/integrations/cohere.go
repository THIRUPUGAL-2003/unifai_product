package integrations

import (
	"context"
	"errors"

	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/providers/cohere"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

// hydrateCohereRequestFromLargePayloadMetadata populates model + stream from
// LargePayloadMetadata when body parsing is skipped under large payload mode.
func hydrateCohereRequestFromLargePayloadMetadata(unifaiCtx *schemas.UnifAIContext, req interface{}) {
	if unifaiCtx == nil {
		return
	}
	isLargePayload, _ := unifaiCtx.Value(schemas.UnifAIContextKeyLargePayloadMode).(bool)
	if !isLargePayload {
		return
	}
	metadata := resolveLargePayloadMetadata(unifaiCtx)
	if metadata == nil {
		return
	}

	switch r := req.(type) {
	case *cohere.CohereChatRequest:
		if r.Model == "" {
			r.Model = metadata.Model
		}
		if metadata.StreamRequested != nil && r.Stream == nil {
			r.Stream = schemas.Ptr(*metadata.StreamRequested)
		}
	case *cohere.CohereEmbeddingRequest:
		if r.Model == "" {
			r.Model = metadata.Model
		}
	case *cohere.CohereRerankRequest:
		if r.Model == "" {
			r.Model = metadata.Model
		}
	case *cohere.CohereCountTokensRequest:
		if r.Model == "" {
			r.Model = metadata.Model
		}
	}
}

// cohereLargePayloadPreHook populates model + stream from LargePayloadMetadata
// when body parsing is skipped under large payload mode.
func cohereLargePayloadPreHook(_ *fasthttp.RequestCtx, unifaiCtx *schemas.UnifAIContext, req interface{}) error {
	hydrateCohereRequestFromLargePayloadMetadata(unifaiCtx, req)
	return nil
}

// CohereRouter holds route registrations for Cohere endpoints.
// It supports Cohere's v2 chat, embeddings, and rerank APIs.
type CohereRouter struct {
	*GenericRouter
}

// NewCohereRouter creates a new CohereRouter with the given unifai client.
func NewCohereRouter(client *unifai.UnifAI, handlerStore lib.HandlerStore, logger schemas.Logger) *CohereRouter {
	return &CohereRouter{
		GenericRouter: NewGenericRouter(client, handlerStore, CreateCohereRouteConfigs("/cohere"), nil, logger),
	}
}

// CreateCohereRouteConfigs creates route configurations for Cohere API endpoints.
func CreateCohereRouteConfigs(pathPrefix string) []RouteConfig {
	var routes []RouteConfig

	// Chat completions endpoint (v2/chat)
	routes = append(routes, RouteConfig{
		Type:        RouteConfigTypeCohere,
		Path:        pathPrefix + "/v2/chat",
		Method:      "POST",
		PreCallback: cohereLargePayloadPreHook,
		GetHTTPRequestType: func(ctx *fasthttp.RequestCtx) schemas.RequestType {
			return schemas.ChatCompletionRequest
		},
		GetRequestTypeInstance: func(ctx context.Context) interface{} {
			return &cohere.CohereChatRequest{}
		},
		RequestConverter: func(ctx *schemas.UnifAIContext, req interface{}) (*schemas.UnifAIRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereChatRequest); ok {
				return &schemas.UnifAIRequest{
					ChatRequest: cohereReq.ToUnifAIChatRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid request type")
		},
		ChatResponseConverter: func(ctx *schemas.UnifAIContext, resp *schemas.UnifAIChatResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.UnifAIContext, err *schemas.UnifAIError) interface{} {
			return err
		},
		StreamConfig: &StreamConfig{
			ChatStreamResponseConverter: func(ctx *schemas.UnifAIContext, resp *schemas.UnifAIChatResponse) (string, interface{}, error) {
				if resp.ExtraFields.Provider == schemas.Cohere {
					if resp.ExtraFields.RawResponse != nil {
						return "", resp.ExtraFields.RawResponse, nil
					}
				}
				return "", resp, nil
			},
			ErrorConverter: func(ctx *schemas.UnifAIContext, err *schemas.UnifAIError) interface{} {
				return err
			},
		},
	})

	// Embeddings endpoint (v2/embed)
	routes = append(routes, RouteConfig{
		Type:        RouteConfigTypeCohere,
		Path:        pathPrefix + "/v2/embed",
		Method:      "POST",
		PreCallback: cohereLargePayloadPreHook,
		GetHTTPRequestType: func(ctx *fasthttp.RequestCtx) schemas.RequestType {
			return schemas.EmbeddingRequest
		},
		GetRequestTypeInstance: func(ctx context.Context) interface{} {
			return &cohere.CohereEmbeddingRequest{}
		},
		RequestConverter: func(ctx *schemas.UnifAIContext, req interface{}) (*schemas.UnifAIRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereEmbeddingRequest); ok {
				return &schemas.UnifAIRequest{
					EmbeddingRequest: cohereReq.ToUnifAIEmbeddingRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid embedding request type")
		},
		EmbeddingResponseConverter: func(ctx *schemas.UnifAIContext, resp *schemas.UnifAIEmbeddingResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.UnifAIContext, err *schemas.UnifAIError) interface{} {
			return err
		},
	})

	// Rerank endpoint (v2/rerank)
	routes = append(routes, RouteConfig{
		Type:        RouteConfigTypeCohere,
		Path:        pathPrefix + "/v2/rerank",
		Method:      "POST",
		PreCallback: cohereLargePayloadPreHook,
		GetHTTPRequestType: func(ctx *fasthttp.RequestCtx) schemas.RequestType {
			return schemas.RerankRequest
		},
		GetRequestTypeInstance: func(ctx context.Context) interface{} {
			return &cohere.CohereRerankRequest{}
		},
		RequestConverter: func(ctx *schemas.UnifAIContext, req interface{}) (*schemas.UnifAIRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereRerankRequest); ok {
				return &schemas.UnifAIRequest{
					RerankRequest: cohereReq.ToUnifAIRerankRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid rerank request type")
		},
		RerankResponseConverter: func(ctx *schemas.UnifAIContext, resp *schemas.UnifAIRerankResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.UnifAIContext, err *schemas.UnifAIError) interface{} {
			return err
		},
	})

	// Tokenize endpoint (v1/tokenize)
	routes = append(routes, RouteConfig{
		Type:        RouteConfigTypeCohere,
		Path:        pathPrefix + "/v1/tokenize",
		Method:      "POST",
		PreCallback: cohereLargePayloadPreHook,
		GetHTTPRequestType: func(ctx *fasthttp.RequestCtx) schemas.RequestType {
			return schemas.CountTokensRequest
		},
		GetRequestTypeInstance: func(ctx context.Context) interface{} {
			return &cohere.CohereCountTokensRequest{}
		},
		RequestConverter: func(ctx *schemas.UnifAIContext, req interface{}) (*schemas.UnifAIRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereCountTokensRequest); ok {
				return &schemas.UnifAIRequest{
					CountTokensRequest: cohereReq.ToUnifAIResponsesRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid count tokens request type")
		},
		CountTokensResponseConverter: func(ctx *schemas.UnifAIContext, resp *schemas.UnifAICountTokensResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.UnifAIContext, err *schemas.UnifAIError) interface{} {
			return err
		},
	})

	return routes
}
