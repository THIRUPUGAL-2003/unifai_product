package bedrock

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/unifai/unifai/core/schemas"
)

// bedrockInputTokenCountHeader is the HTTP response header Bedrock uses to report input
// token counts for models — notably Cohere embed and rerank — that omit token usage from
// the response body.
const bedrockInputTokenCountHeader = "X-Amzn-Bedrock-Input-Token-Count"

// inputTokensFromHeaders extracts the X-Amzn-Bedrock-Input-Token-Count value from a provider
// response-headers map (case-insensitive, since header casing depends on the transport).
// It returns (count, true) only when the header is present and parses as a non-negative int.
func inputTokensFromHeaders(headers map[string]string) (int, bool) {
	for k, v := range headers {
		if strings.EqualFold(k, bedrockInputTokenCountHeader) {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

// ToBedrockTitanEmbeddingRequest converts a UnifAI embedding request to Bedrock Titan format
func ToBedrockTitanEmbeddingRequest(unifaiReq *schemas.UnifAIEmbeddingRequest) (*BedrockTitanEmbeddingRequest, error) {
	if unifaiReq == nil {
		return nil, fmt.Errorf("unifai embedding request is nil")
	}

	// Validate that only single text input is provided for Titan models
	if unifaiReq.Input.Text == nil && len(unifaiReq.Input.Texts) == 0 {
		return nil, fmt.Errorf("no input text provided for embedding")
	}

	titanReq := &BedrockTitanEmbeddingRequest{}

	// Set input text
	if unifaiReq.Input.Text != nil {
		titanReq.InputText = *unifaiReq.Input.Text
	} else if len(unifaiReq.Input.Texts) > 0 {
		var embeddingText string
		for _, text := range unifaiReq.Input.Texts {
			embeddingText += text + " \n"
		}
		titanReq.InputText = embeddingText
	}

	if unifaiReq.Params != nil {
		titanReq.Dimensions = unifaiReq.Params.Dimensions
		if normalize, ok := unifaiReq.Params.ExtraParams["normalize"]; ok {
			if b, ok := normalize.(bool); ok {
				titanReq.Normalize = &b
			}
		}
		// Forward remaining extra params (excluding normalize which is now a first-class field)
		if len(unifaiReq.Params.ExtraParams) > 0 {
			extra := make(map[string]interface{})
			for k, v := range unifaiReq.Params.ExtraParams {
				if k != "normalize" {
					extra[k] = v
				}
			}
			if len(extra) > 0 {
				titanReq.ExtraParams = extra
			}
		}
	}

	return titanReq, nil
}

// ToUnifAIEmbeddingResponse converts a Bedrock Titan embedding response to UnifAI format
func (response *BedrockTitanEmbeddingResponse) ToUnifAIEmbeddingResponse() *schemas.UnifAIEmbeddingResponse {
	if response == nil {
		return nil
	}

	unifaiResponse := &schemas.UnifAIEmbeddingResponse{
		Object: "list",
		Data: []schemas.EmbeddingData{
			{
				Index:  0,
				Object: "embedding",
				Embedding: schemas.EmbeddingStruct{
					EmbeddingArray: response.Embedding,
				},
			},
		},
		Usage: &schemas.UnifAILLMUsage{
			PromptTokens: response.InputTextTokenCount,
			TotalTokens:  response.InputTextTokenCount,
		},
	}

	return unifaiResponse
}

// ToBedrockCohereEmbeddingRequest converts a UnifAI embedding request to Bedrock Cohere format.
// Unlike the direct Cohere API, Bedrock does not accept a "model" field in the request body.
func ToBedrockCohereEmbeddingRequest(unifaiReq *schemas.UnifAIEmbeddingRequest) (*BedrockCohereEmbeddingRequest, error) {
	if unifaiReq == nil {
		return nil, fmt.Errorf("unifai embedding request is nil")
	}
	if unifaiReq.Input == nil || (unifaiReq.Input.Text == nil && len(unifaiReq.Input.Texts) == 0) {
		return nil, fmt.Errorf("no input provided for embedding")
	}

	req := &BedrockCohereEmbeddingRequest{}

	// Map texts
	if unifaiReq.Input.Text != nil {
		req.Texts = []string{*unifaiReq.Input.Text}
	} else if len(unifaiReq.Input.Texts) > 0 {
		req.Texts = unifaiReq.Input.Texts
	}

	if unifaiReq.Params != nil {
		extra := make(map[string]interface{}, len(unifaiReq.Params.ExtraParams))
		for k, v := range unifaiReq.Params.ExtraParams {
			extra[k] = v
		}

		if v, ok := extra["input_type"]; ok {
			if s, ok := v.(string); ok {
				req.InputType = s
				delete(extra, "input_type")
			}
		}
		if v, ok := extra["truncate"]; ok {
			if s, ok := v.(string); ok {
				req.Truncate = &s
				delete(extra, "truncate")
			}
		}
		if v, ok := extra["embedding_types"]; ok {
			if ss, ok := v.([]string); ok {
				req.EmbeddingTypes = ss
				delete(extra, "embedding_types")
			}
		}
		if v, ok := extra["images"]; ok {
			if ss, ok := v.([]string); ok {
				req.Images = ss
				delete(extra, "images")
			}
		}
		if v, ok := extra["inputs"]; ok {
			if inputs, ok := v.([]BedrockCohereEmbeddingInput); ok {
				req.Inputs = inputs
				delete(extra, "inputs")
			}
		}
		if v, ok := extra["max_tokens"]; ok {
			switch n := v.(type) {
			case int:
				req.MaxTokens = &n
				delete(extra, "max_tokens")
			case float64:
				i := int(n)
				req.MaxTokens = &i
				delete(extra, "max_tokens")
			}
		}
		if unifaiReq.Params.Dimensions != nil {
			req.OutputDimension = unifaiReq.Params.Dimensions
		}
		if len(extra) > 0 {
			req.ExtraParams = extra
		}
	}

	return req, nil
}

// DetermineEmbeddingModelType determines the embedding model type for the
// current attempt. It consults the resolved alias family first
// (model_family / model_name / model_id / alias key) and falls back to the
// substring detectors against the wire model — so an alias to an opaque
// Bedrock deployment that's tagged with the right family routes correctly.
func DetermineEmbeddingModelType(ctx *schemas.UnifAIContext, model string) (string, error) {
	switch {
	case schemas.IsTitanModelFamily(ctx, model):
		return "titan", nil
	case schemas.IsCohereModelFamily(ctx, model):
		return "cohere", nil
	default:
		return "", fmt.Errorf("unsupported embedding model: %s", model)
	}
}

// ToUnifAIEmbeddingResponse converts a BedrockCohereEmbeddingResponse to UnifAI format.
// Bedrock returns embeddings as a raw [][]float32 when response_type is "embeddings_floats"
// (the default, when no embedding_types are requested), and as a typed object when
// response_type is "embeddings_by_type".
func (r *BedrockCohereEmbeddingResponse) ToUnifAIEmbeddingResponse() (*schemas.UnifAIEmbeddingResponse, error) {
	if r == nil {
		return nil, fmt.Errorf("nil Bedrock Cohere embedding response")
	}

	unifaiResponse := &schemas.UnifAIEmbeddingResponse{Object: "list"}

	switch r.ResponseType {
	case "embeddings_by_type":
		// Object form: {"float": [[...]], "int8": [[...]], "uint8": [[...]], "binary": [[...]], "ubinary": [[...]], "base64": [...]}
		var typed struct {
			Float   [][]float32 `json:"float"`
			Base64  []string    `json:"base64"`
			Int8    [][]int8    `json:"int8"`
			Uint8   [][]int32   `json:"uint8"` // int32 avoids []byte→base64 JSON issue
			Binary  [][]int8    `json:"binary"`
			Ubinary [][]int32   `json:"ubinary"` // int32 avoids []byte→base64 JSON issue
		}
		if err := json.Unmarshal(r.Embeddings, &typed); err != nil {
			return nil, fmt.Errorf("error parsing embeddings_by_type: %w", err)
		}
		if typed.Float != nil {
			for i, emb := range typed.Float {
				float64Emb := make([]float64, len(emb))
				for j, v := range emb {
					float64Emb[j] = float64(v)
				}
				unifaiResponse.Data = append(unifaiResponse.Data, schemas.EmbeddingData{
					Object:    "embedding",
					Index:     i,
					Embedding: schemas.EmbeddingStruct{EmbeddingArray: float64Emb},
				})
			}
		}
		if typed.Base64 != nil {
			for i, emb := range typed.Base64 {
				e := emb
				unifaiResponse.Data = append(unifaiResponse.Data, schemas.EmbeddingData{
					Object:    "embedding",
					Index:     i,
					Embedding: schemas.EmbeddingStruct{EmbeddingStr: &e},
				})
			}
		}
		for i, emb := range typed.Int8 {
			unifaiResponse.Data = append(unifaiResponse.Data, schemas.EmbeddingData{
				Object:    "embedding",
				Index:     i,
				Embedding: schemas.EmbeddingStruct{EmbeddingInt8Array: emb},
			})
		}
		for i, emb := range typed.Binary {
			unifaiResponse.Data = append(unifaiResponse.Data, schemas.EmbeddingData{
				Object:    "embedding",
				Index:     i,
				Embedding: schemas.EmbeddingStruct{EmbeddingInt8Array: emb},
			})
		}
		for i, emb := range typed.Uint8 {
			unifaiResponse.Data = append(unifaiResponse.Data, schemas.EmbeddingData{
				Object:    "embedding",
				Index:     i,
				Embedding: schemas.EmbeddingStruct{EmbeddingInt32Array: emb},
			})
		}
		for i, emb := range typed.Ubinary {
			unifaiResponse.Data = append(unifaiResponse.Data, schemas.EmbeddingData{
				Object:    "embedding",
				Index:     i,
				Embedding: schemas.EmbeddingStruct{EmbeddingInt32Array: emb},
			})
		}

	default:
		// Default / "embeddings_floats": raw array form [[...], [...]]
		var floats [][]float32
		if err := json.Unmarshal(r.Embeddings, &floats); err != nil {
			return nil, fmt.Errorf("error parsing embeddings_floats: %w", err)
		}
		for i, emb := range floats {
			float64Emb := make([]float64, len(emb))
			for j, v := range emb {
				float64Emb[j] = float64(v)
			}
			unifaiResponse.Data = append(unifaiResponse.Data, schemas.EmbeddingData{
				Object:    "embedding",
				Index:     i,
				Embedding: schemas.EmbeddingStruct{EmbeddingArray: float64Emb},
			})
		}
	}

	return unifaiResponse, nil
}
