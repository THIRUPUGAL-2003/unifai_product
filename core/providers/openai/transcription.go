package openai

import (
	"fmt"
	"mime/multipart"
	"sort"

	"github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// ToUnifAITranscriptionRequest converts an OpenAI transcription request to UnifAI format
func (request *OpenAITranscriptionRequest) ToUnifAITranscriptionRequest(ctx *schemas.UnifAIContext) *schemas.UnifAITranscriptionRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	return &schemas.UnifAITranscriptionRequest{
		Provider: provider,
		Model:    model,
		Input: &schemas.TranscriptionInput{
			File: request.File,
		},
		Params:    &request.TranscriptionParameters,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}
}

// ToOpenAITranscriptionRequest converts a UnifAI transcription request to OpenAI format
func ToOpenAITranscriptionRequest(unifaiReq *schemas.UnifAITranscriptionRequest) *OpenAITranscriptionRequest {
	if unifaiReq == nil || unifaiReq.Input.File == nil {
		return nil
	}

	transcriptionInput := unifaiReq.Input
	params := unifaiReq.Params

	openaiReq := &OpenAITranscriptionRequest{
		Model:    unifaiReq.Model,
		File:     transcriptionInput.File,
		Filename: transcriptionInput.Filename,
	}

	if params != nil {
		openaiReq.TranscriptionParameters = *params
		openaiReq.ExtraParams = params.ExtraParams
	}

	return openaiReq
}

// ParseTranscriptionFormDataBodyFromRequest parses the transcription request and writes it to the multipart form.
func ParseTranscriptionFormDataBodyFromRequest(writer *multipart.Writer, openaiReq *OpenAITranscriptionRequest, providerName schemas.ModelProvider) *schemas.UnifAIError {
	// Add model field before the file so upstreams can route without buffering the audio payload.
	if err := writer.WriteField("model", openaiReq.Model); err != nil {
		return utils.NewUnifAIOperationError("failed to write model field", err)
	}

	// Add optional fields
	if openaiReq.Language != nil {
		if err := writer.WriteField("language", *openaiReq.Language); err != nil {
			return utils.NewUnifAIOperationError("failed to write language field", err)
		}
	}

	if openaiReq.Prompt != nil {
		if err := writer.WriteField("prompt", *openaiReq.Prompt); err != nil {
			return utils.NewUnifAIOperationError("failed to write prompt field", err)
		}
	}

	if openaiReq.ResponseFormat != nil {
		if err := writer.WriteField("response_format", *openaiReq.ResponseFormat); err != nil {
			return utils.NewUnifAIOperationError("failed to write response_format field", err)
		}
	}

	if openaiReq.Temperature != nil {
		if err := writer.WriteField("temperature", fmt.Sprintf("%g", *openaiReq.Temperature)); err != nil {
			return utils.NewUnifAIOperationError("failed to write temperature field", err)
		}
	}

	for _, granularity := range openaiReq.TimestampGranularities {
		if err := writer.WriteField("timestamp_granularities[]", granularity); err != nil {
			return utils.NewUnifAIOperationError("failed to write timestamp_granularities field", err)
		}
	}

	for _, include := range openaiReq.Include {
		if err := writer.WriteField("include[]", include); err != nil {
			return utils.NewUnifAIOperationError("failed to write include field", err)
		}
	}

	if openaiReq.Stream != nil && *openaiReq.Stream {
		if err := writer.WriteField("stream", "true"); err != nil {
			return utils.NewUnifAIOperationError("failed to write stream field", err)
		}
	}

	// Forward provider-specific passthrough params (e.g. chunking_strategy, required by
	// OpenAI diarization models). String values are written verbatim; object values are
	// encoded as JSON since multipart form fields are strings. Keys are sorted so the
	// emitted form is deterministic.
	if len(openaiReq.ExtraParams) > 0 {
		extraKeys := make([]string, 0, len(openaiReq.ExtraParams))
		for key := range openaiReq.ExtraParams {
			extraKeys = append(extraKeys, key)
		}
		sort.Strings(extraKeys)
		for _, key := range extraKeys {
			value := openaiReq.ExtraParams[key]
			var fieldValue string
			switch v := value.(type) {
			case string:
				fieldValue = v
			default:
				encoded, err := schemas.MarshalSorted(v)
				if err != nil {
					return utils.NewUnifAIOperationError(fmt.Sprintf("failed to encode %s field", key), err)
				}
				fieldValue = string(encoded)
			}
			if err := writer.WriteField(key, fieldValue); err != nil {
				return utils.NewUnifAIOperationError(fmt.Sprintf("failed to write %s field", key), err)
			}
		}
	}

	// Add file field last so large multipart uploads don't block model discovery upstream.
	filename := openaiReq.Filename
	if filename == "" {
		filename = utils.AudioFilenameFromBytes(openaiReq.File)
	}
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return utils.NewUnifAIOperationError("failed to create form file", err)
	}
	if _, err := fileWriter.Write(openaiReq.File); err != nil {
		return utils.NewUnifAIOperationError("failed to write file data", err)
	}

	// Close the multipart writer
	if err := writer.Close(); err != nil {
		return utils.NewUnifAIOperationError("failed to close multipart writer", err)
	}

	return nil
}
