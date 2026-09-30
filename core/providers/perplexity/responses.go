package perplexity

import (
	"strings"

	"github.com/unifai/unifai/core/schemas"
)

// isPerplexityResponsesSupported reports whether the model should use /v1/responses vs /chat/completions.
// Denylist by design: /chat/completions serves only the Sonar family, so sonar-* variants go to chat and
// every other model (base sonar + non-Sonar) goes to responses. This keeps newly-shipped non-Sonar models
// working without a code change; they'd fail on chat, which doesn't serve them.
func isPerplexityResponsesSupported(model string) bool {
	return !strings.HasPrefix(strings.TrimPrefix(model, "perplexity/"), "sonar-")
}

// ToPerplexityResponsesRequest converts a UnifAIResponsesRequest to PerplexityChatRequest
func ToPerplexityResponsesRequest(unifaiReq *schemas.UnifAIResponsesRequest) *PerplexityChatRequest {
	if unifaiReq == nil {
		return nil
	}

	perplexityReq := &PerplexityChatRequest{
		Model: unifaiReq.Model,
	}

	// Map basic parameters
	if unifaiReq.Params != nil {
		// Core parameters
		perplexityReq.MaxTokens = unifaiReq.Params.MaxOutputTokens
		perplexityReq.Temperature = unifaiReq.Params.Temperature
		perplexityReq.TopP = unifaiReq.Params.TopP

		// Handle reasoning effort mapping
		if unifaiReq.Params.Reasoning != nil && unifaiReq.Params.Reasoning.Effort != nil {
			if *unifaiReq.Params.Reasoning.Effort == "minimal" {
				perplexityReq.ReasoningEffort = schemas.Ptr("low")
			} else {
				perplexityReq.ReasoningEffort = schemas.Ptr(*unifaiReq.Params.Reasoning.Effort)
			}
		}

		// Handle extra parameters for Perplexity-specific fields
		if unifaiReq.Params.ExtraParams != nil {
			// Search-related parameters
			if searchMode, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["search_mode"]); ok {
				perplexityReq.SearchMode = searchMode
			}

			if languagePreference, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["language_preference"]); ok {
				perplexityReq.LanguagePreference = languagePreference
			}

			if searchDomainFilter, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["search_domain_filter"]); ok {
				perplexityReq.SearchDomainFilter = searchDomainFilter
			}

			if returnImages, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["return_images"]); ok {
				perplexityReq.ReturnImages = returnImages
			}

			if returnRelatedQuestions, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["return_related_questions"]); ok {
				perplexityReq.ReturnRelatedQuestions = returnRelatedQuestions
			}

			if searchRecencyFilter, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["search_recency_filter"]); ok {
				perplexityReq.SearchRecencyFilter = searchRecencyFilter
			}

			if searchAfterDateFilter, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["search_after_date_filter"]); ok {
				perplexityReq.SearchAfterDateFilter = searchAfterDateFilter
			}

			if searchBeforeDateFilter, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["search_before_date_filter"]); ok {
				perplexityReq.SearchBeforeDateFilter = searchBeforeDateFilter
			}

			if lastUpdatedAfterFilter, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["last_updated_after_filter"]); ok {
				perplexityReq.LastUpdatedAfterFilter = lastUpdatedAfterFilter
			}

			if lastUpdatedBeforeFilter, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["last_updated_before_filter"]); ok {
				perplexityReq.LastUpdatedBeforeFilter = lastUpdatedBeforeFilter
			}

			if topK, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["top_k"]); ok {
				perplexityReq.TopK = topK
			}

			if stream, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["stream"]); ok {
				perplexityReq.Stream = stream
			}

			if disableSearch, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["disable_search"]); ok {
				perplexityReq.DisableSearch = disableSearch
			}

			if enableSearchClassifier, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["enable_search_classifier"]); ok {
				perplexityReq.EnableSearchClassifier = enableSearchClassifier
			}

			if presencePenalty, ok := schemas.SafeExtractFloat64Pointer(unifaiReq.Params.ExtraParams["presence_penalty"]); ok {
				perplexityReq.PresencePenalty = presencePenalty
			}

			if frequencyPenalty, ok := schemas.SafeExtractFloat64Pointer(unifaiReq.Params.ExtraParams["frequency_penalty"]); ok {
				perplexityReq.FrequencyPenalty = frequencyPenalty
			}

			if responseFormat, ok := schemas.SafeExtractFromMap(unifaiReq.Params.ExtraParams, "response_format"); ok {
				perplexityReq.ResponseFormat = &responseFormat
			}

			// Perplexity-specific request fields
			if numSearchResults, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["num_search_results"]); ok {
				perplexityReq.NumSearchResults = numSearchResults
			}

			if numImages, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["num_images"]); ok {
				perplexityReq.NumImages = numImages
			}

			if searchLanguageFilter, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["search_language_filter"]); ok {
				perplexityReq.SearchLanguageFilter = searchLanguageFilter
			}

			if imageFormatFilter, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["image_format_filter"]); ok {
				perplexityReq.ImageFormatFilter = imageFormatFilter
			}

			if imageDomainFilter, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["image_domain_filter"]); ok {
				perplexityReq.ImageDomainFilter = imageDomainFilter
			}

			if safeSearch, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["safe_search"]); ok {
				perplexityReq.SafeSearch = safeSearch
			}

			if streamMode, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["stream_mode"]); ok {
				perplexityReq.StreamMode = streamMode
			}

			// Handle web_search_options
			if webSearchOptionsParam, ok := schemas.SafeExtractFromMap(unifaiReq.Params.ExtraParams, "web_search_options"); ok {
				if webSearchOptionsSlice, ok := webSearchOptionsParam.([]interface{}); ok {
					var webSearchOptions []WebSearchOption
					for _, optionInterface := range webSearchOptionsSlice {
						if optionMap, ok := optionInterface.(map[string]interface{}); ok {
							option := WebSearchOption{}

							if searchContextSize, ok := schemas.SafeExtractStringPointer(optionMap["search_context_size"]); ok {
								option.SearchContextSize = searchContextSize
							}

							if imageResultsEnhancedRelevance, ok := schemas.SafeExtractBoolPointer(optionMap["image_results_enhanced_relevance"]); ok {
								option.ImageResultsEnhancedRelevance = imageResultsEnhancedRelevance
							}

							if searchType, ok := schemas.SafeExtractStringPointer(optionMap["search_type"]); ok {
								option.SearchType = searchType
							}

							// Handle user_location
							if userLocationParam, ok := schemas.SafeExtractFromMap(optionMap, "user_location"); ok {
								if userLocationMap, ok := userLocationParam.(map[string]interface{}); ok {
									userLocation := &WebSearchOptionUserLocation{}

									if latitude, ok := schemas.SafeExtractFloat64Pointer(userLocationMap["latitude"]); ok {
										userLocation.Latitude = latitude
									}
									if longitude, ok := schemas.SafeExtractFloat64Pointer(userLocationMap["longitude"]); ok {
										userLocation.Longitude = longitude
									}
									if city, ok := schemas.SafeExtractStringPointer(userLocationMap["city"]); ok {
										userLocation.City = city
									}
									if country, ok := schemas.SafeExtractStringPointer(userLocationMap["country"]); ok {
										userLocation.Country = country
									}
									if region, ok := schemas.SafeExtractStringPointer(userLocationMap["region"]); ok {
										userLocation.Region = region
									}

									option.UserLocation = userLocation
								}
							}

							webSearchOptions = append(webSearchOptions, option)
						}
					}
					perplexityReq.WebSearchOptions = webSearchOptions
				}
			}

			// Handle media_response
			if mediaResponseParam, ok := schemas.SafeExtractFromMap(unifaiReq.Params.ExtraParams, "media_response"); ok {
				if mediaResponseMap, ok := mediaResponseParam.(map[string]interface{}); ok {
					mediaResponse := &MediaResponse{}

					if overridesParam, ok := schemas.SafeExtractFromMap(mediaResponseMap, "overrides"); ok {
						if overridesMap, ok := overridesParam.(map[string]interface{}); ok {
							overrides := MediaResponseOverrides{}

							if returnVideos, ok := schemas.SafeExtractBoolPointer(overridesMap["return_videos"]); ok {
								overrides.ReturnVideos = returnVideos
							}
							if returnImages, ok := schemas.SafeExtractBoolPointer(overridesMap["return_images"]); ok {
								overrides.ReturnImages = returnImages
							}

							mediaResponse.Overrides = overrides
						}
					}

					perplexityReq.MediaResponse = mediaResponse
				}
			}
		}
	}

	// Process ResponsesInput (which contains the Responses messages)
	if unifaiReq.Input != nil {
		perplexityReq.Messages = schemas.ToChatMessages(unifaiReq.Input)
	}

	return perplexityReq
}
