package integrations

import (
	"context"
	"errors"

	gateway "github.com/gateway/gateway/core"
	"github.com/gateway/gateway/core/providers/cohere"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/valyala/fasthttp"
)

// hydrateCohereRequestFromLargePayloadMetadata populates model + stream from
// LargePayloadMetadata when body parsing is skipped under large payload mode.
func hydrateCohereRequestFromLargePayloadMetadata(gatewayCtx *schemas.GatewayContext, req interface{}) {
	if gatewayCtx == nil {
		return
	}
	isLargePayload, _ := gatewayCtx.Value(schemas.GatewayContextKeyLargePayloadMode).(bool)
	if !isLargePayload {
		return
	}
	metadata := resolveLargePayloadMetadata(gatewayCtx)
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
func cohereLargePayloadPreHook(_ *fasthttp.RequestCtx, gatewayCtx *schemas.GatewayContext, req interface{}) error {
	hydrateCohereRequestFromLargePayloadMetadata(gatewayCtx, req)
	return nil
}

// CohereRouter holds route registrations for Cohere endpoints.
// It supports Cohere's v2 chat, embeddings, and rerank APIs.
type CohereRouter struct {
	*GenericRouter
}

// NewCohereRouter creates a new CohereRouter with the given gateway client.
func NewCohereRouter(client *gateway.Gateway, handlerStore lib.HandlerStore, logger schemas.Logger) *CohereRouter {
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
		RequestConverter: func(ctx *schemas.GatewayContext, req interface{}) (*schemas.GatewayRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereChatRequest); ok {
				return &schemas.GatewayRequest{
					ChatRequest: cohereReq.ToGatewayChatRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid request type")
		},
		ChatResponseConverter: func(ctx *schemas.GatewayContext, resp *schemas.GatewayChatResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		},
		StreamConfig: &StreamConfig{
			ChatStreamResponseConverter: func(ctx *schemas.GatewayContext, resp *schemas.GatewayChatResponse) (string, interface{}, error) {
				if resp.ExtraFields.Provider == schemas.Cohere {
					if resp.ExtraFields.RawResponse != nil {
						return "", resp.ExtraFields.RawResponse, nil
					}
				}
				return "", resp, nil
			},
			ErrorConverter: func(ctx *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
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
		RequestConverter: func(ctx *schemas.GatewayContext, req interface{}) (*schemas.GatewayRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereEmbeddingRequest); ok {
				return &schemas.GatewayRequest{
					EmbeddingRequest: cohereReq.ToGatewayEmbeddingRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid embedding request type")
		},
		EmbeddingResponseConverter: func(ctx *schemas.GatewayContext, resp *schemas.GatewayEmbeddingResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
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
		RequestConverter: func(ctx *schemas.GatewayContext, req interface{}) (*schemas.GatewayRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereRerankRequest); ok {
				return &schemas.GatewayRequest{
					RerankRequest: cohereReq.ToGatewayRerankRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid rerank request type")
		},
		RerankResponseConverter: func(ctx *schemas.GatewayContext, resp *schemas.GatewayRerankResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
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
		RequestConverter: func(ctx *schemas.GatewayContext, req interface{}) (*schemas.GatewayRequest, error) {
			if cohereReq, ok := req.(*cohere.CohereCountTokensRequest); ok {
				return &schemas.GatewayRequest{
					CountTokensRequest: cohereReq.ToGatewayResponsesRequest(ctx),
				}, nil
			}
			return nil, errors.New("invalid count tokens request type")
		},
		CountTokensResponseConverter: func(ctx *schemas.GatewayContext, resp *schemas.GatewayCountTokensResponse) (interface{}, error) {
			if resp.ExtraFields.Provider == schemas.Cohere {
				if resp.ExtraFields.RawResponse != nil {
					return resp.ExtraFields.RawResponse, nil
				}
			}
			return resp, nil
		},
		ErrorConverter: func(ctx *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		},
	})

	return routes
}
