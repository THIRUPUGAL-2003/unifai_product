package bedrock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/raksha/raksha/core/providers/anthropic"
	openai "github.com/raksha/raksha/core/providers/openai"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// BedrockProvider implements the Provider interface for AWS Bedrock.
type BedrockProvider struct {
	logger                schemas.Logger                // Logger for provider operations
	client                *http.Client                  // HTTP client for unary API requests (Client.Timeout bounds overall response)
	streamingClient       *http.Client                  // HTTP client for streaming API requests (no Timeout; idle governed by NewIdleTimeoutReader)
	mantleClient          *fasthttp.Client              // fasthttp client for Bedrock Mantle unary requests (OpenAI-compatible and native-Anthropic paths)
	mantleStreamingClient *fasthttp.Client              // fasthttp streaming client for Bedrock Mantle streaming requests
	networkConfig         schemas.NetworkConfig         // Network configuration including extra headers
	customProviderConfig  *schemas.CustomProviderConfig // Custom provider config
	sendBackRawRequest    bool                          // Whether to include raw request in RakshaResponse
	sendBackRawResponse   bool                          // Whether to include raw response in RakshaResponse
}

// assumeRoleCredsCache caches *aws.CredentialsCache instances keyed by the
// unique combination of role parameters so that STS AssumeRole is not called
// on every request.
var assumeRoleCredsCache sync.Map

// bedrockChatResponsePool provides a pool for Bedrock response objects.
var bedrockChatResponsePool = sync.Pool{
	New: func() interface{} {
		return &BedrockConverseResponse{}
	},
}

// acquireBedrockChatResponse gets a Bedrock response from the pool and resets it.
func acquireBedrockChatResponse() *BedrockConverseResponse {
	resp := bedrockChatResponsePool.Get().(*BedrockConverseResponse)
	*resp = BedrockConverseResponse{} // Reset the struct
	return resp
}

// releaseBedrockChatResponse returns a Bedrock response to the pool.
func releaseBedrockChatResponse(resp *BedrockConverseResponse) {
	if resp != nil {
		bedrockChatResponsePool.Put(resp)
	}
}

// NewBedrockProvider creates a new Bedrock provider instance.
// It initializes the HTTP client with the provided configuration and sets up response pools.
// The client is configured with timeouts and AWS-specific settings.
func NewBedrockProvider(config *schemas.ProviderConfig, logger schemas.Logger) (*BedrockProvider, error) {
	config.CheckAndSetDefaults()

	requestTimeout := time.Second * time.Duration(config.NetworkConfig.DefaultRequestTimeoutInSeconds)

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxConnsPerHost:       config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConns:          schemas.DefaultMaxIdleConnsPerHost,
		MaxIdleConnsPerHost:   schemas.DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: requestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     config.NetworkConfig.EnforceHTTP2,
	}

	// Disable HTTP/2 auto-negotiation when not explicitly enforced.
	// ForceAttemptHTTP2=false alone does NOT prevent HTTP/2 — Go's http2 package
	// auto-registers h2 via TLSNextProto in init(). Setting TLSNextProto to an
	// empty map prevents ALPN negotiation from upgrading connections to h2.
	if !config.NetworkConfig.EnforceHTTP2 {
		transport.TLSNextProto = make(map[string]func(authority string, c *tls.Conn) http.RoundTripper)
	}

	// Apply TLS settings from NetworkConfig
	caCertPEM := ""
	if config.NetworkConfig.CACertPEM != nil {
		caCertPEM = config.NetworkConfig.CACertPEM.GetValue()
	}
	if config.NetworkConfig.InsecureSkipVerify || caCertPEM != "" {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if config.NetworkConfig.InsecureSkipVerify {
			tlsConfig.InsecureSkipVerify = true
		}
		if caCertPEM != "" {
			certPool, err := x509.SystemCertPool()
			if err != nil {
				certPool = x509.NewCertPool()
			}
			if !certPool.AppendCertsFromPEM([]byte(caCertPEM)) {
				return nil, fmt.Errorf("failed to parse CA certificate PEM")
			}
			tlsConfig.RootCAs = certPool
		}
		transport.TLSClientConfig = tlsConfig
	}

	client := &http.Client{Transport: transport, Timeout: requestTimeout}
	streamingClient := providerUtils.BuildStreamingHTTPClient(client)

	// fasthttp clients for Bedrock Mantle (shared by OpenAI-compatible and native-Anthropic paths).
	// ReadTimeout is the shared provider request timeout, not an OpenAI-specific value; oversized
	// Anthropic responses are handled by PrepareResponseStreaming, not by these static settings.
	mantleFasthttpClient := &fasthttp.Client{
		ReadTimeout:         requestTimeout,
		WriteTimeout:        requestTimeout,
		MaxConnsPerHost:     config.NetworkConfig.MaxConnsPerHost,
		MaxIdleConnDuration: 30 * time.Second,
		MaxConnWaitTimeout:  requestTimeout,
		MaxConnDuration:     time.Second * time.Duration(schemas.DefaultMaxConnDurationInSeconds),
		ConnPoolStrategy:    fasthttp.FIFO,
	}
	mantleFasthttpClient = providerUtils.ConfigureProxy(mantleFasthttpClient, config.ProxyConfig, logger)
	mantleFasthttpClient = providerUtils.ConfigureDialer(mantleFasthttpClient, config.NetworkConfig.AllowPrivateNetwork)
	mantleFasthttpClient = providerUtils.ConfigureTLS(mantleFasthttpClient, config.NetworkConfig, logger)
	mantleStreamingFasthttpClient := providerUtils.BuildStreamingClient(mantleFasthttpClient)

	// Pre-warm response pools
	for i := 0; i < config.ConcurrencyAndBufferSize.Concurrency; i++ {
		bedrockChatResponsePool.Put(&BedrockConverseResponse{})
	}

	return &BedrockProvider{
		logger:                logger,
		client:                client,
		streamingClient:       streamingClient,
		mantleClient:          mantleFasthttpClient,
		mantleStreamingClient: mantleStreamingFasthttpClient,
		networkConfig:         config.NetworkConfig,
		customProviderConfig:  config.CustomProviderConfig,
		sendBackRawRequest:    config.SendBackRawRequest,
		sendBackRawResponse:   config.SendBackRawResponse,
	}, nil
}

// GetProviderKey returns the provider identifier for Bedrock.
func (provider *BedrockProvider) GetProviderKey() schemas.ModelProvider {
	return providerUtils.GetProviderName(schemas.Bedrock, provider.customProviderConfig)
}

// isStreamTransportError reports whether err is a transport-level connection
// failure that occurred while reading the EventStream body — as opposed to a
// semantic error (JSON parse failure, AWS exception event, etc.).
//
// Transport errors are caused by the underlying TCP/HTTP/2 connection being
// closed or reset (e.g. AWS Bedrock closing idle connections after ~60 s).
// They are retryable: the request has not yet been partially processed by the
// provider, so a fresh connection can be used to retry transparently.
//
// Detected cases:
//   - *net.OpError  — "use of closed network connection", connection reset, etc.
//   - *net.DNSError — transient DNS failure
//   - io.ErrUnexpectedEOF — HTTP/2 stream closed mid-frame (body abruptly ended)
func isStreamTransportError(err error) bool {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var checksumErr eventstream.ChecksumError
	if errors.As(err, &checksumErr) {
		return true
	}
	var opErr *net.OpError
	var dnsErr *net.DNSError
	return errors.As(err, &opErr) || errors.As(err, &dnsErr)
}

// retryableBedrockExceptions maps AWS Bedrock EventStream exception types (the
// camelCase shape names AWS uses for ConverseStream / InvokeModelWithResponseStream
// in-stream exception members) to a retryable HTTP status code. These exceptions
// are transient and should be retried — the retry gate in executeRequestWithRetries
// checks StatusCode against transientServerStatusCodes (500, 502, 503, 504) for
// same-key retries and perKeyFailureStatusCodes (429) for rotation-triggered retries.
//
// Some AWS exceptions have a native status code the gate does not recognize
// (modelStreamErrorException=424, modelTimeoutException=408); they are mapped to
// the nearest gate-recognized transient code so the retry still fires. The client
// still sees the original exception type — only the internal retry hint is mapped.
//
// modelNotReadyException is an HTTP-level error (not an in-stream member) but is
// kept here because AWS auto-retries it; it is harmless when it never matches a
// stream event. validationException / accessDeniedException / resourceNotFoundException
// are intentionally absent — they are terminal request-bound errors.
var retryableBedrockExceptions = map[string]int{
	"throttlingException":         429,
	"serviceUnavailableException": 503,
	"modelNotReadyException":      503,
	"internalServerException":     500,
	"modelStreamErrorException":   503, // native 424; AWS guidance: "Retry your request"
	"modelTimeoutException":       504, // native 408; processing timeout, transient
}

// newBedrockStreamException builds a RakshaError from an AWS EventStream
// exception message (any :message-type other than "event"). It preserves the
// upstream exception type — the payload's "__type" when present, else the
// :exception-type header value (excType) — so downstream conversion
// (ToBedrockError) forwards it instead of falling back to "InternalServerError".
//
// Retryable exceptions are emitted with IsRakshaError:false and the equivalent
// HTTP status so the retry gate in executeRequestWithRetries handles them;
// non-retryable ones are terminal (IsRakshaError:true). providerName is an
// optional label prefix for the message.
func newBedrockStreamException(providerName, excType string, payload []byte) *schemas.RakshaError {
	errMsg := string(payload)
	var bedrockErr BedrockError
	if err := sonic.Unmarshal(payload, &bedrockErr); err == nil && bedrockErr.Message != "" {
		errMsg = bedrockErr.Message
	}

	fwdType := bedrockErr.Type
	if fwdType == "" {
		fwdType = excType
	}

	prefix := "stream"
	if providerName != "" {
		prefix = providerName + " stream"
	}

	streamErr := &schemas.RakshaError{
		IsRakshaError: false,
		Error: &schemas.ErrorField{
			Message: fmt.Sprintf("%s %s: %s", prefix, excType, errMsg),
		},
	}
	if fwdType != "" {
		streamErr.Type = &fwdType
	}
	if statusCode, ok := retryableBedrockExceptions[excType]; ok {
		sc := statusCode
		streamErr.StatusCode = &sc
	} else {
		streamErr.IsRakshaError = true
	}
	return streamErr
}

// completeRequest sends a request to Bedrock's API and handles the response.
// It constructs the API URL, sets up AWS authentication, and processes the response.
// Returns the response body, request latency, or an error if the request fails.
func (provider *BedrockProvider) completeRequest(ctx *schemas.RakshaContext, jsonData []byte, path string, key schemas.Key, model string) ([]byte, time.Duration, map[string]string, *schemas.RakshaError) {
	config := key.BedrockKeyConfig
	region := resolveBedrockRegion(ctx, key, model)

	// Create the request with the JSON body
	requestURL := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s", region, path)
	req, err := http.NewRequestWithContext(ctx, "POST", requestURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, nil, &schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: "error creating request",
				Error:   err,
			},
		}
	}

	// Set any extra headers from network config
	providerUtils.SetExtraHeadersHTTP(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	if filtered := anthropic.FilterBetaHeadersForProvider(anthropic.MergeBetaHeaders(ctx, provider.networkConfig.ExtraHeaders), schemas.Bedrock, provider.networkConfig.BetaHeaderOverrides); len(filtered) > 0 {
		req.Header.Set(anthropic.AnthropicBetaHeader, strings.Join(filtered, ","))
	} else {
		req.Header.Del(anthropic.AnthropicBetaHeader)
	}

	// If Value is set, use API Key authentication - else use IAM role authentication
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key.Value.GetValue()))
	} else {
		// Sign the request using either explicit credentials or IAM role authentication
		if err := signAWSRequest(ctx, req, config, region, bedrockSigningService); err != nil {
			return nil, 0, nil, err
		}
	}

	body, latency, providerResponseHeaders, rakshaErr := provider.executeBedrockRequest(req)
	return body, latency, providerResponseHeaders, rakshaErr
}

// executeBedrockRequest sends an already-built (and authenticated) request via the
// unary HTTP client, measures latency, and parses a Bedrock error envelope on non-200
// responses. Used by completeRequest for the bedrock-runtime (Converse) path.
func (provider *BedrockProvider) executeBedrockRequest(req *http.Request) ([]byte, time.Duration, map[string]string, *schemas.RakshaError) {
	// Execute the request and measure latency
	startTime := time.Now()
	resp, err := provider.client.Do(req)
	latency := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, latency, nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		// Check for timeout first using net.Error before checking net.OpError
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, latency, nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		if errors.Is(err, http.ErrHandlerTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, latency, nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		// Check for DNS lookup and network errors after timeout checks
		var opErr *net.OpError
		var dnsErr *net.DNSError
		if errors.As(err, &opErr) || errors.As(err, &dnsErr) {
			return nil, latency, nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderNetworkError,
					Error:   err,
				},
			}, latency)
		}
		return nil, latency, nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: false,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderDoRequest,
				Error:   err,
			},
		}, latency)
	}

	// Extract provider response headers before closing the body
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeadersFromHTTP(resp)
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, providerResponseHeaders, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: "error reading request",
				Error:   err,
			},
		}, latency)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp BedrockError

		var rawErrorResponse interface{}
		if err := sonic.Unmarshal(body, &rawErrorResponse); err != nil {
			rawErrorResponse = string(body)
		}

		if err := sonic.Unmarshal(body, &errorResp); err != nil {
			return nil, latency, providerResponseHeaders, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: true,
				StatusCode:     &resp.StatusCode,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderResponseUnmarshal,
					Error:   err,
				},
				ExtraFields: schemas.RakshaErrorExtraFields{
					RawResponse: rawErrorResponse,
				},
			}, latency)
		}

		return nil, latency, providerResponseHeaders, providerUtils.SetErrorLatency(&schemas.RakshaError{
			StatusCode: &resp.StatusCode,
			Error: &schemas.ErrorField{
				Message: errorResp.Message,
			},
			ExtraFields: schemas.RakshaErrorExtraFields{
				RawResponse: rawErrorResponse,
			},
		}, latency)
	}

	return body, latency, providerResponseHeaders, nil
}

// completeAgentRuntimeRequest sends a request to Bedrock Agent Runtime API and handles the response.
// This is used for operations (like rerank) that are served by bedrock-agent-runtime.
func (provider *BedrockProvider) completeAgentRuntimeRequest(ctx *schemas.RakshaContext, jsonData []byte, path string, key schemas.Key) ([]byte, time.Duration, map[string]string, *schemas.RakshaError) {
	config := key.BedrockKeyConfig

	region := DefaultBedrockRegion
	if config.Region != nil && config.Region.GetValue() != "" {
		region = config.Region.GetValue()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://bedrock-agent-runtime.%s.amazonaws.com%s", region, path), bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, nil, &schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: "error creating request",
				Error:   err,
			},
		}
	}

	providerUtils.SetExtraHeadersHTTP(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key.Value.GetValue()))
	} else {
		if err := signAWSRequest(ctx, req, config, region, bedrockSigningService); err != nil {
			return nil, 0, nil, err
		}
	}

	startTime := time.Now()
	resp, err := provider.client.Do(req)
	latency := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, latency, nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, latency, nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		if errors.Is(err, http.ErrHandlerTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, latency, nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		var opErr *net.OpError
		var dnsErr *net.DNSError
		if errors.As(err, &opErr) || errors.As(err, &dnsErr) {
			return nil, latency, nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderNetworkError,
					Error:   err,
				},
			}, latency)
		}
		return nil, latency, nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: false,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderDoRequest,
				Error:   err,
			},
		}, latency)
	}

	// Extract provider response headers before closing the body
	providerResponseHeaders := providerUtils.ExtractProviderResponseHeadersFromHTTP(resp)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, providerResponseHeaders, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: "error reading request",
				Error:   err,
			},
		}, latency)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, latency, providerResponseHeaders, providerUtils.SetErrorLatency(parseBedrockHTTPError(resp.StatusCode, resp.Header, body), latency)
	}

	return body, latency, providerResponseHeaders, nil
}

// makeStreamingRequest creates a streaming request to Bedrock's API.
// It formats the request, sends it to Bedrock, and returns the response.
// Returns the response body and an error if the request fails.
func (provider *BedrockProvider) makeStreamingRequest(ctx *schemas.RakshaContext, jsonData []byte, key schemas.Key, model string, action string) (*http.Response, *schemas.RakshaError) {
	// Parse region and path in one pass to avoid running the regex twice.
	path, region := provider.getModelPathAndRegion(ctx, action, model, key)

	// Create HTTP request for streaming
	requestURL := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s", region, path)
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(jsonData))
	if reqErr != nil {
		return nil, providerUtils.NewRakshaOperationError("error creating request", reqErr)
	}

	// Set any extra headers from network config
	providerUtils.SetExtraHeadersHTTP(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	if filtered := anthropic.FilterBetaHeadersForProvider(anthropic.MergeBetaHeaders(ctx, provider.networkConfig.ExtraHeaders), schemas.Bedrock, provider.networkConfig.BetaHeaderOverrides); len(filtered) > 0 {
		req.Header.Set(anthropic.AnthropicBetaHeader, strings.Join(filtered, ","))
	} else {
		req.Header.Del(anthropic.AnthropicBetaHeader)
	}

	// If Value is set, use API Key authentication - else use IAM role authentication
	req.Header.Set("Accept", "application/vnd.amazon.eventstream")
	// Force identity encoding so Go's net/http transport does NOT auto-negotiate
	// gzip. A gzip-compressed eventstream buffers upstream until the stream
	// completes, collapsing TTFB to the total generation time (issue #4542).
	req.Header.Set("Accept-Encoding", "identity")
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key.Value.GetValue()))
	} else {
		// Sign the request using either explicit credentials or IAM role authentication
		if err := signAWSRequest(ctx, req, key.BedrockKeyConfig, region, bedrockSigningService); err != nil {
			return nil, err
		}
	}

	// Make the request
	startTime := time.Now()
	resp, respErr := provider.streamingClient.Do(req)
	latency := time.Since(startTime)
	if respErr != nil {
		if errors.Is(respErr, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   respErr,
				},
			}, latency)
		}
		// Check for timeout first using net.Error before checking net.OpError
		var netErr net.Error
		if errors.As(respErr, &netErr) && netErr.Timeout() {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, respErr), latency)
		}
		if errors.Is(respErr, http.ErrHandlerTimeout) || errors.Is(respErr, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, respErr), latency)
		}
		// Check for DNS lookup and network errors after timeout checks
		var opErr *net.OpError
		var dnsErr *net.DNSError
		if errors.As(respErr, &opErr) || errors.As(respErr, &dnsErr) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderNetworkError,
					Error:   respErr,
				},
			}, latency)
		}
		return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: false,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderDoRequest,
				Error:   respErr,
			},
		}, latency)
	}

	// Extract provider response headers before status check so error responses also forward them
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeadersFromHTTP(resp))

	// Check for HTTP errors — use parseBedrockHTTPError to preserve upstream error details
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, providerUtils.SetErrorLatency(parseBedrockHTTPError(resp.StatusCode, resp.Header, body), latency)
	}

	return resp, nil
}

// Returns a RakshaError if signing fails.
func signAWSRequest(
	ctx *schemas.RakshaContext,
	req *http.Request,
	keyCfg *schemas.BedrockKeyConfig,
	region, service string,
) *schemas.RakshaError {
	var accessKey, secretKey schemas.SecretVar
	var sessionToken, roleARN, externalID, sessionName *schemas.SecretVar

	if keyCfg != nil {
		accessKey = keyCfg.AccessKey
		secretKey = keyCfg.SecretKey
		sessionToken = keyCfg.SessionToken
		roleARN = keyCfg.RoleARN
		externalID = keyCfg.ExternalID
		sessionName = keyCfg.RoleSessionName
	}

	// Set required headers before signing (only if not already set)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	// Calculate SHA256 hash of the request body
	var bodyHash string
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return providerUtils.NewRakshaOperationError("error reading request body", err)
		}
		// Restore the body for subsequent reads
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		hash := sha256.Sum256(bodyBytes)
		bodyHash = hex.EncodeToString(hash[:])
	} else {
		// For empty body, use the hash of an empty string
		hash := sha256.Sum256([]byte{})
		bodyHash = hex.EncodeToString(hash[:])
	}

	// Set x-amz-content-sha256 header (required for S3, harmless for other AWS services)
	req.Header.Set("x-amz-content-sha256", bodyHash)

	var cfg aws.Config
	var err error

	// If both accessKey and secretKey are empty, use the default credential provider chain
	// This will automatically use IAM roles, environment variables, shared credentials, etc.
	if accessKey.GetValue() == "" && secretKey.GetValue() == "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	} else {
		// Use explicit credentials when provided
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				creds := aws.Credentials{
					AccessKeyID:     accessKey.GetValue(),
					SecretAccessKey: secretKey.GetValue(),
				}
				if sessionToken != nil && sessionToken.GetValue() != "" {
					creds.SessionToken = sessionToken.GetValue()
				}
				return creds, nil
			})),
		)
	}
	if err != nil {
		return providerUtils.NewRakshaOperationError("failed to load aws config", err)
	}

	if roleARN != nil && roleARN.GetValue() != "" {
		extID := ""
		if externalID != nil {
			extID = externalID.GetValue()
		}
		sessName := "raksha-session"
		if sessionName != nil && sessionName.GetValue() != "" {
			sessName = sessionName.GetValue()
		}
		sourceIdentity := "default_chain"
		if accessKey.GetValue() != "" || secretKey.GetValue() != "" {
			sourceIdentity = accessKey.GetValue()
			if sessionToken != nil && sessionToken.GetValue() != "" {
				tokenHash := sha256.Sum256([]byte(sessionToken.GetValue()))
				sourceIdentity = sourceIdentity + "|" + hex.EncodeToString(tokenHash[:8])
			}
		}
		cacheKey := strings.Join([]string{
			region,
			roleARN.GetValue(),
			extID,
			sessName,
			sourceIdentity,
		}, "|")

		if cached, ok := assumeRoleCredsCache.Load(cacheKey); ok {
			cfg.Credentials = cached.(*aws.CredentialsCache)
		} else {
			stsClient := sts.NewFromConfig(cfg)

			opts := func(o *stscreds.AssumeRoleOptions) {
				if extID != "" {
					o.ExternalID = aws.String(extID)
				}
				o.RoleSessionName = sessName
			}

			credsCache := aws.NewCredentialsCache(
				stscreds.NewAssumeRoleProvider(
					stsClient,
					roleARN.GetValue(),
					opts,
				),
			)
			actual, _ := assumeRoleCredsCache.LoadOrStore(cacheKey, credsCache)
			cfg.Credentials = actual.(*aws.CredentialsCache)
		}
	}

	// Create the AWS signer
	signer := v4.NewSigner()

	// Get credentials
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return providerUtils.NewRakshaOperationError("failed to retrieve aws credentials", err)
	}

	// Sign the request with AWS Signature V4
	if err := signer.SignHTTP(ctx, creds, req, bodyHash, service, region, time.Now()); err != nil {
		return providerUtils.NewRakshaOperationError("failed to sign request", err)
	}

	return nil
}

// listModelsByKey performs a list models request to Bedrock's API for a single key.
// It retrieves all foundation models available in Amazon Bedrock for a specific key.
// listMantleModels lists models from the Bedrock Mantle (OpenAI-compatible) /v1/models
// endpoint, converted to a Raksha response with the same allow/blacklist/alias gating as
// the foundation-model path. The bare /v1/models path returns the full mantle catalog
// (including the mantle-only gpt-5.x / gemma-4 models that ListFoundationModels omits).
// The request is signed as it is sent (mantleSigV4Headers signs POST and can't be reused
// for this GET). Best-effort: returns nil on any failure so the foundation-model list is
// still returned.
func (provider *BedrockProvider) listMantleModels(ctx *schemas.RakshaContext, key schemas.Key, region string, unfiltered bool) *schemas.RakshaListModelsResponse {
	mURL := mantleOpenAIURL(region, "", "models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mURL, nil)
	if err != nil {
		provider.logger.Warn("failed to build mantle list-models request: %v", err)
		return nil
	}
	providerUtils.SetExtraHeadersHTTP(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key.Value.GetValue()))
	} else if rakshaErr := signAWSRequest(ctx, req, key.BedrockKeyConfig, region, bedrockMantleSigningService); rakshaErr != nil {
		provider.logger.Warn("failed to sign mantle list-models request: %v", rakshaErr.Error.Message)
		return nil
	}

	resp, err := provider.client.Do(req)
	if err != nil {
		provider.logger.Warn("mantle list-models request failed: %v", err)
		return nil
	}
	responseBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		provider.logger.Warn("failed to read mantle list-models response: %v", err)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		provider.logger.Warn("mantle list-models returned status %d: %s", resp.StatusCode, string(responseBody))
		return nil
	}

	mantleResponse := &openai.OpenAIListModelsResponse{}
	if err := sonic.Unmarshal(responseBody, mantleResponse); err != nil {
		provider.logger.Warn("failed to parse mantle list-models response: %v", err)
		return nil
	}
	return mantleResponse.ToRakshaListModelsResponse(provider.GetProviderKey(), key.Models, key.BlacklistedModels, key.Aliases, unfiltered)
}

func (provider *BedrockProvider) listModelsByKey(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	providerName := provider.GetProviderKey()
	config := key.BedrockKeyConfig
	region := DefaultBedrockRegion
	if config.Region != nil && config.Region.GetValue() != "" {
		region = config.Region.GetValue()
	}

	// Build query parameters
	params := url.Values{}
	if request.ExtraParams != nil {
		if byCustomizationType, ok := request.ExtraParams["byCustomizationType"].(string); ok && byCustomizationType != "" {
			params.Set("byCustomizationType", byCustomizationType)
		}
		if byInferenceType, ok := request.ExtraParams["byInferenceType"].(string); ok && byInferenceType != "" {
			params.Set("byInferenceType", byInferenceType)
		}
		if byOutputModality, ok := request.ExtraParams["byOutputModality"].(string); ok && byOutputModality != "" {
			params.Set("byOutputModality", byOutputModality)
		}
		if byProvider, ok := request.ExtraParams["byProvider"].(string); ok && byProvider != "" {
			params.Set("byProvider", byProvider)
		}
	}

	// List models endpoint uses the bedrock service (not bedrock-runtime)
	url := fmt.Sprintf("https://bedrock.%s.amazonaws.com/foundation-models?%s", region, params.Encode())

	// Create the GET request without a body
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: "error creating request",
				Error:   err,
			},
		}
	}

	// Set any extra headers from network config
	providerUtils.SetExtraHeadersHTTP(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	// If Value is set, use API Key authentication - else use IAM role authentication
	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key.Value.GetValue()))
	} else {
		// Sign the request using either explicit credentials or IAM role authentication

		if err := signAWSRequest(ctx, req, config, region, bedrockSigningService); err != nil {
			return nil, err
		}
	}

	startTime := time.Now()

	// Execute the request
	resp, err := provider.client.Do(req)
	latency := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, latency)
		}
		// Check for timeout first using net.Error before checking net.OpError
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		if errors.Is(err, http.ErrHandlerTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, providerUtils.SetErrorLatency(providerUtils.NewRakshaTimeoutError(schemas.ErrProviderRequestTimedOut, err), latency)
		}
		// Check for DNS lookup and network errors after timeout checks
		var opErr *net.OpError
		var dnsErr *net.DNSError
		if errors.As(err, &opErr) || errors.As(err, &dnsErr) {
			return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Message: schemas.ErrProviderNetworkError,
					Error:   err,
				},
			}, latency)
		}
		return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: false,
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderDoRequest,
				Error:   err,
			},
		}, latency)
	}

	// Read response body and close
	responseBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, providerUtils.SetErrorLatency(&schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: "error reading request",
				Error:   err,
			},
		}, latency)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseBedrockHTTPError(resp.StatusCode, resp.Header, responseBody), latency)
	}

	// Parse Bedrock-specific response
	bedrockResponse := &BedrockListModelsResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(responseBody, bedrockResponse, nil, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	// Convert to Raksha response
	response := bedrockResponse.ToRakshaListModelsResponse(providerName, key.Models, key.BlacklistedModels, key.Aliases, request.Unfiltered)
	if response == nil {
		return nil, providerUtils.NewRakshaOperationError("failed to convert Bedrock model list response", nil)
	}

	// Merge in the mantle catalog: ListFoundationModels omits the mantle-only models
	// (gpt-5.x, gemma-4, ...) served on the OpenAI-compatible endpoint. Same gating, dedup by id.
	if mantleResponse := provider.listMantleModels(ctx, key, region, request.Unfiltered); mantleResponse != nil {
		seen := make(map[string]struct{}, len(response.Data))
		for _, m := range response.Data {
			seen[m.ID] = struct{}{}
		}
		for _, m := range mantleResponse.Data {
			if _, ok := seen[m.ID]; ok {
				continue
			}
			seen[m.ID] = struct{}{}
			response.Data = append(response.Data, m)
		}
	}

	response.ExtraFields.Latency = time.Since(startTime).Milliseconds()

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// ListModels performs a list models request to Bedrock's API.
// It retrieves all foundation models available in Amazon Bedrock.
// Requests are made concurrently for improved performance.
func (provider *BedrockProvider) ListModels(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaListModelsRequest) (*schemas.RakshaListModelsResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ListModelsRequest); err != nil {
		return nil, err
	}
	return providerUtils.HandleMultipleListModelsRequests(
		ctx,
		keys,
		request,
		provider.listModelsByKey,
	)
}

// TextCompletion performs a text completion request to Bedrock's API.
// It formats the request, sends it to Bedrock, and processes the response.
// Returns a RakshaResponse containing the completion results or an error if the request fails.
func (provider *BedrockProvider) TextCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTextCompletionRequest) (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.TextCompletionRequest); err != nil {
		return nil, err
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockTextCompletionRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	path, _ := provider.getModelPathAndRegion(ctx, "invoke", request.Model, key)
	body, latency, providerResponseHeaders, err := provider.completeRequest(ctx, jsonData, path, key, request.Model)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Handle model-specific response conversion
	var rakshaResponse *schemas.RakshaTextCompletionResponse
	switch {
	case schemas.IsAnthropicModelFamily(ctx, request.Model):
		var response BedrockAnthropicTextResponse
		if err := sonic.Unmarshal(body, &response); err != nil {
			return nil, providerUtils.NewRakshaOperationError("error parsing anthropic response", err)
		}
		rakshaResponse = response.ToRakshaTextCompletionResponse()

	case schemas.IsMistralModelFamily(ctx, request.Model):
		var response BedrockMistralTextResponse
		if err := sonic.Unmarshal(body, &response); err != nil {
			return nil, providerUtils.NewRakshaOperationError("error parsing mistral response", err)
		}
		rakshaResponse = response.ToRakshaTextCompletionResponse()

	default:
		return nil, providerUtils.NewConfigurationError(fmt.Sprintf("unsupported model type for text completion: %s", request.Model))
	}

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&rakshaResponse.ExtraFields, jsonData)
	}

	// Parse raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponse interface{}
		if err := sonic.Unmarshal(body, &rawResponse); err != nil {
			return nil, providerUtils.NewRakshaOperationError("error parsing raw response", err)
		}
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// TextCompletionStream performs a streaming text completion request to Bedrock's API.
// It formats the request, sends it to Bedrock, and processes the response.
// Returns a channel of RakshaStreamChunk objects or an error if the request fails.
func (provider *BedrockProvider) TextCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTextCompletionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.TextCompletionStreamRequest); err != nil {
		return nil, err
	}

	providerName := provider.GetProviderKey()

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockTextCompletionRequest(request), nil
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	startTime := time.Now()
	resp, rakshaErr := provider.makeStreamingRequest(ctx, jsonData, key, request.Model, "invoke-with-response-stream")
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse)
	}

	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeadersFromHTTP(resp))

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonData)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonData)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer resp.Body.Close()

		// Wrap body with idle timeout to detect stalled streams.
		idleReader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(resp.Body, resp.Body, providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close body stream on ctx cancellation
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.Body, provider.logger)
		defer stopCancellation()

		// Process AWS Event Stream format
		decoder := eventstream.NewDecoder()
		payloadBuf := make([]byte, 0, 1024*1024) // 1MB payload buffer

		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}
			// Decode a single EventStream message
			message, err := decoder.Decode(idleReader, payloadBuf)
			if err != nil {
				// If context was cancelled/timed out, let defer handle it
				if ctx.Err() != nil {
					return
				}
				if err == io.EOF {
					// End of stream - this is normal
					break
				}
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				provider.logger.Warn("error decoding %s EventStream message: %v", providerName, err)
				// Transport-level errors (stale/closed connection, unexpected EOF) are retryable.
				// Use IsRakshaError:false so the retry gate in executeRequestWithRetries can retry.
				if isStreamTransportError(err) {
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, &schemas.RakshaError{
						IsRakshaError: false,
						Error: &schemas.ErrorField{
							Message: schemas.ErrProviderNetworkError,
							Error:   err,
						},
					}, responseChan, provider.logger, postHookSpanFinalizer)
				} else {
					providerUtils.ProcessAndSendError(ctx, postHookRunner, err, responseChan, provider.logger, postHookSpanFinalizer)
				}
				return
			}

			// Process the decoded message payload (contains JSON for normal events)
			if len(message.Payload) > 0 {
				if msgTypeHeader := message.Headers.Get(":message-type"); msgTypeHeader != nil {
					if msgType := msgTypeHeader.String(); msgType != "event" {
						excType := msgType
						if excHeader := message.Headers.Get(":exception-type"); excHeader != nil {
							if v := excHeader.String(); v != "" {
								excType = v
							}
						}
						streamErr := newBedrockStreamException(string(providerName), excType, message.Payload)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, streamErr, responseChan, provider.logger, postHookSpanFinalizer)
						return
					}
				}

				// Parse the chunk payload
				var chunkPayload struct {
					Bytes []byte `json:"bytes"`
				}
				if err := sonic.Unmarshal(message.Payload, &chunkPayload); err != nil {
					provider.logger.Debug("Failed to parse JSON from event buffer: %v, data: %s", err, string(message.Payload))
					providerUtils.ProcessAndSendError(ctx, postHookRunner, err, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}

				// Create RakshaStreamChunk response containing the raw model-specific JSON chunk
				textResponse := &schemas.RakshaTextCompletionResponse{
					ExtraFields: schemas.RakshaResponseExtraFields{
						Latency: time.Since(startTime).Milliseconds(),
						// Pass the raw JSON string from the chunk bytes
						RawResponse: string(chunkPayload.Bytes),
					},
				}

				providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(textResponse, nil, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
			}
		}
	}()

	return responseChan, nil
}

// ChatCompletion performs a chat completion request to Bedrock's API.
// OpenAI-family and Gemma 4 models route via the Bedrock Mantle OpenAI-compatible endpoint.
// All other models (including Anthropic/Claude) use the Bedrock Converse API.
// Returns a RakshaResponse containing the completion results or an error if the request fails.
func (provider *BedrockProvider) ChatCompletion(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaChatRequest) (*schemas.RakshaChatResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ChatCompletionRequest); err != nil {
		return nil, err
	}

	if isMantleModel(ctx, request.Model) {
		return provider.mantleChatCompletions(ctx, key, request)
	}

	// Use Bedrock Converse API for all other models
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockChatCompletionRequest(ctx, request)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	path, _ := provider.getModelPathAndRegion(ctx, "converse", request.Model, key)

	// Create the signed request
	responseBody, latency, providerResponseHeaders, rakshaErr := provider.completeRequest(ctx, jsonData, path, key, request.Model)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse Bedrock Converse API response
	bedrockResponse := acquireBedrockChatResponse()
	defer releaseBedrockChatResponse(bedrockResponse)

	// Parse the response using the new Bedrock type
	if err := sonic.Unmarshal(responseBody, bedrockResponse); err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("failed to parse bedrock response", err), jsonData, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Convert using the new response converter
	rakshaResponse, err := bedrockResponse.ToRakshaChatResponse(ctx, request.Model)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("failed to convert bedrock response", err), jsonData, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Override finish reason for structured output (Converse API only)
	if _, ok := ctx.Value(schemas.RakshaContextKeyStructuredOutputToolName).(string); ok {
		if len(rakshaResponse.Choices) > 0 && rakshaResponse.Choices[0].FinishReason != nil {
			if *rakshaResponse.Choices[0].FinishReason == string(schemas.RakshaFinishReasonToolCalls) {
				rakshaResponse.Choices[0].FinishReason = schemas.Ptr(string(schemas.RakshaFinishReasonStop))
			}
		}
	}

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&rakshaResponse.ExtraFields, jsonData)
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponse interface{}
		if err := sonic.Unmarshal(responseBody, &rawResponse); err == nil {
			rakshaResponse.ExtraFields.RawResponse = rawResponse
		}
	}

	return rakshaResponse, nil
}

// normalizeCachedUsage folds the accumulated cached read/write token counts into
// PromptTokens. Bedrock reports TotalTokens directly on the stream, so only the
// prompt counter needs the fold. The accumulator must apply it before billing -
// including on a mid-stream cancel/timeout. The += is not idempotent; callers
// guard with a flag to apply it exactly once.
func normalizeCachedUsage(usage *schemas.RakshaLLMUsage) {
	if usage == nil || usage.PromptTokensDetails == nil {
		return
	}
	usage.PromptTokens += usage.PromptTokensDetails.CachedReadTokens + usage.PromptTokensDetails.CachedWriteTokens
}

func accumulateBedrockResponsesUsage(usage *schemas.ResponsesResponseUsage, billedUsage *schemas.RakshaLLMUsage, usageToProcess *BedrockTokenUsage) {
	if usage == nil || usageToProcess == nil {
		return
	}
	if usageToProcess.InputTokens > usage.InputTokens {
		usage.InputTokens = usageToProcess.InputTokens
		if billedUsage != nil {
			billedUsage.PromptTokens = usageToProcess.InputTokens
		}
	}
	if usageToProcess.OutputTokens > usage.OutputTokens {
		usage.OutputTokens = usageToProcess.OutputTokens
		if billedUsage != nil {
			billedUsage.CompletionTokens = usageToProcess.OutputTokens
		}
	}
	if usageToProcess.TotalTokens > usage.TotalTokens {
		usage.TotalTokens = usageToProcess.TotalTokens
		if billedUsage != nil {
			billedUsage.TotalTokens = usageToProcess.TotalTokens
		}
	}
	if usageToProcess.CacheReadInputTokens > 0 {
		if usage.InputTokensDetails == nil {
			usage.InputTokensDetails = &schemas.ResponsesResponseInputTokens{}
		}
		if billedUsage != nil && billedUsage.PromptTokensDetails == nil {
			billedUsage.PromptTokensDetails = &schemas.ChatPromptTokensDetails{}
		}
		if usageToProcess.CacheReadInputTokens > usage.InputTokensDetails.CachedReadTokens {
			usage.InputTokensDetails.CachedReadTokens = usageToProcess.CacheReadInputTokens
			if billedUsage != nil {
				billedUsage.PromptTokensDetails.CachedReadTokens = usageToProcess.CacheReadInputTokens
			}
		}
	}
	if usageToProcess.CacheWriteInputTokens > 0 {
		if usage.InputTokensDetails == nil {
			usage.InputTokensDetails = &schemas.ResponsesResponseInputTokens{}
		}
		if billedUsage != nil && billedUsage.PromptTokensDetails == nil {
			billedUsage.PromptTokensDetails = &schemas.ChatPromptTokensDetails{}
		}
		if usageToProcess.CacheWriteInputTokens > usage.InputTokensDetails.CachedWriteTokens {
			usage.InputTokensDetails.CachedWriteTokens = usageToProcess.CacheWriteInputTokens
			if billedUsage != nil {
				billedUsage.PromptTokensDetails.CachedWriteTokens = usageToProcess.CacheWriteInputTokens
			}
		}
		if usageToProcess.CacheDetails != nil {
			if usage.InputTokensDetails.CachedWriteTokenDetails == nil {
				usage.InputTokensDetails.CachedWriteTokenDetails = &schemas.ChatCachedWriteTokenDetails{}
			}
			if billedUsage != nil && billedUsage.PromptTokensDetails.CachedWriteTokenDetails == nil {
				billedUsage.PromptTokensDetails.CachedWriteTokenDetails = &schemas.ChatCachedWriteTokenDetails{}
			}
			for _, cacheDetail := range *usageToProcess.CacheDetails {
				if cacheDetail.TTL == BedrockCacheWriteTTL5m {
					usage.InputTokensDetails.CachedWriteTokenDetails.CachedWriteTokens5m = cacheDetail.InputTokens
					if billedUsage != nil {
						billedUsage.PromptTokensDetails.CachedWriteTokenDetails.CachedWriteTokens5m = cacheDetail.InputTokens
					}
				}
				if cacheDetail.TTL == BedrockCacheWriteTTL1h {
					usage.InputTokensDetails.CachedWriteTokenDetails.CachedWriteTokens1h = cacheDetail.InputTokens
					if billedUsage != nil {
						billedUsage.PromptTokensDetails.CachedWriteTokenDetails.CachedWriteTokens1h = cacheDetail.InputTokens
					}
				}
			}
		}
	}
}

// ChatCompletionStream performs a streaming chat completion request to Bedrock's API.
// OpenAI-family and Gemma 4 models route via the Bedrock Mantle OpenAI-compatible endpoint.
// All other models (including Anthropic/Claude) use the Bedrock Converse streaming API.
// Returns a channel for streaming RakshaStreamChunk objects or an error if the request fails.
func (provider *BedrockProvider) ChatCompletionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaChatRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ChatCompletionStreamRequest); err != nil {
		return nil, err
	}

	if isMantleModel(ctx, request.Model) {
		return provider.mantleChatCompletionsStream(ctx, postHookRunner, postHookSpanFinalizer, key, request)
	}

	// Use Bedrock Converse streaming API for all other models
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockChatCompletionRequest(ctx, request)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	startTime := time.Now()

	resp, rakshaErr := provider.makeStreamingRequest(ctx, jsonData, key, request.Model, "converse-stream")
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse)
	}

	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeadersFromHTTP(resp))

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)
	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonData)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonData)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		defer resp.Body.Close()

		// Wrap body with idle timeout to detect stalled streams.
		idleReader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(resp.Body, resp.Body, providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close body stream on ctx cancellation
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.Body, provider.logger)
		defer stopCancellation()

		// Process AWS Event Stream format
		usage := &schemas.RakshaLLMUsage{}
		// Register the accumulating usage handle so a mid-stream
		// cancel/timeout can bill for tokens the provider already processed.
		ctx.SetValue(schemas.RakshaContextKeyStreamAccumulatedUsage, usage)

		// Fold cached tokens into PromptTokens exactly once at stream end. The EOF
		// path calls normalizeUsage() after the loop; on a mid-stream cancel/timeout
		// the deferred call below runs first (LIFO, registered after the cancellation
		// handler at the top of the goroutine) so HandleStreamCancellation/Timeout
		// bills the normalized totals.
		usageNormalized := false
		normalizeUsage := func() {
			if usageNormalized {
				return
			}
			usageNormalized = true
			normalizeCachedUsage(usage)
		}
		defer func() {
			if ctx.Err() != nil {
				normalizeUsage()
			}
		}()
		var finishReason *string
		chunkIndex := 0

		// Process AWS Event Stream format using proper decoder
		lastChunkTime := startTime
		decoder := eventstream.NewDecoder()
		payloadBuf := make([]byte, 0, 1024*1024) // 1MB payload buffer

		// Bedrock does not provide a unique identifier for the stream, so we generate one ourselves
		id := uuid.New().String()

		// Check for structured output mode - if set, we need to intercept tool calls
		// and convert them to content instead of forwarding as tool calls
		var structuredOutputToolName string
		if toolName, ok := ctx.Value(schemas.RakshaContextKeyStructuredOutputToolName).(string); ok {
			structuredOutputToolName = toolName
		}

		streamState := NewBedrockStreamStateWithContext(ctx)
		var isAccumulatingStructuredOutput bool
		var structuredOutputBuilder strings.Builder

		for {
			if ctx.Err() != nil {
				return
			}
			// Decode a single EventStream message
			message, err := decoder.Decode(idleReader, payloadBuf)
			if err != nil {
				// If context was cancelled/timed out, let defer handle it
				if ctx.Err() != nil {
					return
				}
				// End of stream - this is normal
				if err == io.EOF {
					break
				}
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				provider.logger.Warn("Error decoding EventStream message: %v", err)
				// Transport-level errors (stale/closed connection, unexpected EOF) are retryable.
				// Use IsRakshaError:false so the retry gate in executeRequestWithRetries can retry.
				if isStreamTransportError(err) {
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, &schemas.RakshaError{
						IsRakshaError: false,
						Error: &schemas.ErrorField{
							Message: schemas.ErrProviderNetworkError,
							Error:   err,
						},
					}, responseChan, provider.logger, postHookSpanFinalizer)
				} else {
					providerUtils.ProcessAndSendError(ctx, postHookRunner, err, responseChan, provider.logger, postHookSpanFinalizer)
				}
				return
			}

			// Process the decoded message payload (contains JSON for normal events)
			if len(message.Payload) > 0 {
				if msgTypeHeader := message.Headers.Get(":message-type"); msgTypeHeader != nil {
					if msgType := msgTypeHeader.String(); msgType != "event" {
						excType := msgType
						if excHeader := message.Headers.Get(":exception-type"); excHeader != nil {
							if v := excHeader.String(); v != "" {
								excType = v
							}
						}
						streamErr := newBedrockStreamException("", excType, message.Payload)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, streamErr, responseChan, provider.logger, postHookSpanFinalizer)
						return
					}
				}

				// Converse API path: parse Bedrock Converse-specific stream events
				var streamEvent BedrockStreamEvent
				if err := sonic.Unmarshal(message.Payload, &streamEvent); err != nil {
					provider.logger.Debug("Failed to parse JSON from event buffer: %v, data: %s", err, string(message.Payload))
					providerUtils.ProcessAndSendError(ctx, postHookRunner, err, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}

				if streamEvent.Usage != nil {
					// Accumulate usage information instead of overwriting
					// In some cases usage comes in multiple events, so we need to take the maximum values
					if streamEvent.Usage.InputTokens > usage.PromptTokens {
						usage.PromptTokens = streamEvent.Usage.InputTokens
					}
					if streamEvent.Usage.OutputTokens > usage.CompletionTokens {
						usage.CompletionTokens = streamEvent.Usage.OutputTokens
					}
					if streamEvent.Usage.TotalTokens > usage.TotalTokens {
						usage.TotalTokens = streamEvent.Usage.TotalTokens
					}
					// Handle cached tokens if present
					if streamEvent.Usage.CacheReadInputTokens > 0 {
						if usage.PromptTokensDetails == nil {
							usage.PromptTokensDetails = &schemas.ChatPromptTokensDetails{}
						}
						if streamEvent.Usage.CacheReadInputTokens > usage.PromptTokensDetails.CachedReadTokens {
							usage.PromptTokensDetails.CachedReadTokens = streamEvent.Usage.CacheReadInputTokens
						}
					}
					if streamEvent.Usage.CacheWriteInputTokens > 0 {
						if usage.PromptTokensDetails == nil {
							usage.PromptTokensDetails = &schemas.ChatPromptTokensDetails{}
						}
						if streamEvent.Usage.CacheWriteInputTokens > usage.PromptTokensDetails.CachedWriteTokens {
							usage.PromptTokensDetails.CachedWriteTokens = streamEvent.Usage.CacheWriteInputTokens
						}
						if streamEvent.Usage.CacheDetails != nil {
							if usage.PromptTokensDetails.CachedWriteTokenDetails == nil {
								usage.PromptTokensDetails.CachedWriteTokenDetails = &schemas.ChatCachedWriteTokenDetails{}
							}
							for _, cacheDetail := range *streamEvent.Usage.CacheDetails {
								if cacheDetail.TTL == BedrockCacheWriteTTL5m {
									usage.PromptTokensDetails.CachedWriteTokenDetails.CachedWriteTokens5m = cacheDetail.InputTokens
								}
								if cacheDetail.TTL == BedrockCacheWriteTTL1h {
									usage.PromptTokensDetails.CachedWriteTokenDetails.CachedWriteTokens1h = cacheDetail.InputTokens
								}
							}
						}
					}
				}

				if streamEvent.StopReason != nil {
					finishReason = schemas.Ptr(anthropic.ConvertAnthropicFinishReasonToRaksha(anthropic.AnthropicStopReason(*streamEvent.StopReason)))

					// Override finish reason for structured output
					// When structured output is used, tool_use stop reason should appear as "stop" to the client
					if structuredOutputToolName != "" && *finishReason == string(schemas.RakshaFinishReasonToolCalls) {
						finishReason = schemas.Ptr(string(schemas.RakshaFinishReasonStop))
					}
				}

				// Handle structured output: intercept tool calls for the structured output tool
				// and convert them to content instead of forwarding as tool calls
				if structuredOutputToolName != "" {
					// Check for tool use start event
					if streamEvent.Start != nil && streamEvent.Start.ToolUse != nil {
						if streamEvent.Start.ToolUse.Name == structuredOutputToolName {
							// This is the structured output tool - start accumulating, don't forward
							isAccumulatingStructuredOutput = true
							continue
						}
					}

					// Check for tool use delta event
					if streamEvent.Delta != nil && streamEvent.Delta.ToolUse != nil && isAccumulatingStructuredOutput {
						// Accumulate the input for tracking purposes
						structuredOutputBuilder.WriteString(streamEvent.Delta.ToolUse.Input)

						// Convert tool use delta to content delta
						content := streamEvent.Delta.ToolUse.Input
						response := &schemas.RakshaChatResponse{
							ID:     id,
							Model:  request.Model,
							Object: "chat.completion.chunk",
							Choices: []schemas.RakshaResponseChoice{
								{
									Index: 0,
									ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{
										Delta: &schemas.ChatStreamResponseChoiceDelta{
											Content: &content,
										},
									},
								},
							},
							ExtraFields: schemas.RakshaResponseExtraFields{
								ChunkIndex: chunkIndex,
								Latency:    time.Since(lastChunkTime).Milliseconds(),
							},
						}
						chunkIndex++
						lastChunkTime = time.Now()

						if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
							response.ExtraFields.RawResponse = string(message.Payload)
						}

						providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
						continue
					}

					// Suppress non-tool content events that would leak into the
					// assembled structured output (mirrors ResponsesStreamRequest).
					if streamEvent.Delta != nil && (streamEvent.Delta.Text != nil || streamEvent.Delta.ReasoningContent != nil) {
						continue
					}
					if streamEvent.Start != nil && streamEvent.Start.ToolUse == nil {
						continue // non-tool content-block start (text block) — drop
					}
				}

				response, rakshaErr, _ := streamEvent.ToRakshaChatCompletionStream(streamState)
				if rakshaErr != nil {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, rakshaErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				if response != nil {
					response.ID = id
					response.Model = request.Model
					response.ExtraFields = schemas.RakshaResponseExtraFields{
						ChunkIndex: chunkIndex,
						Latency:    time.Since(lastChunkTime).Milliseconds(),
					}
					chunkIndex++
					lastChunkTime = time.Now()

					if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
						response.ExtraFields.RawResponse = string(message.Payload)
					}

					providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
				}
			}
		}

		normalizeUsage()

		// Send final chunk with accumulated usage
		response := providerUtils.CreateRakshaChatCompletionChunkResponse(id, usage, finishReason, chunkIndex, request.Model, 0)
		// Set raw request if enabled
		if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
			providerUtils.ParseAndSetRawRequest(&response.ExtraFields, jsonData)
		}
		response.ExtraFields.Latency = time.Since(startTime).Milliseconds()
		ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
		providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, response, nil, nil, nil, nil), responseChan, postHookSpanFinalizer)
	}()

	return responseChan, nil
}

// Responses performs a responses request to Bedrock's API.
// OpenAI-family and Gemma 4 models route via the Bedrock Mantle OpenAI-compatible endpoint.
// All other models (including Anthropic/Claude) use the Bedrock Converse API.
// Returns a RakshaResponse containing the completion results or an error if the request fails.
func (provider *BedrockProvider) Responses(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ResponsesRequest); err != nil {
		return nil, err
	}

	if isMantleModel(ctx, request.Model) {
		return provider.mantleResponses(ctx, key, request)
	}

	// Use Bedrock Converse API for all other models
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockResponsesRequest(ctx, request)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	path, _ := provider.getModelPathAndRegion(ctx, "converse", request.Model, key)

	// Create the signed request
	responseBody, latency, providerResponseHeaders, rakshaErr := provider.completeRequest(ctx, jsonData, path, key, request.Model)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse Bedrock Converse API response
	bedrockResponse := acquireBedrockChatResponse()
	defer releaseBedrockChatResponse(bedrockResponse)

	// Parse the response using the new Bedrock type
	if err := sonic.Unmarshal(responseBody, bedrockResponse); err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("failed to parse bedrock response", err), jsonData, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Convert using the new response converter
	rakshaResponse, err := bedrockResponse.ToRakshaResponsesResponse(ctx)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("failed to convert bedrock response", err), jsonData, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	rakshaResponse.Model = request.Model

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&rakshaResponse.ExtraFields, jsonData)
	}

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponse interface{}
		if err := sonic.Unmarshal(responseBody, &rawResponse); err == nil {
			rakshaResponse.ExtraFields.RawResponse = rawResponse
		}
	}

	return rakshaResponse, nil
}

// ResponsesStream performs a streaming chat completion request to Bedrock's API.
// It formats the request, sends it to Bedrock, and processes the streaming response.
// Returns a channel for streaming RakshaResponse objects or an error if the request fails.
func (provider *BedrockProvider) ResponsesStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaResponsesRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ResponsesStreamRequest); err != nil {
		return nil, err
	}

	if isMantleModel(ctx, request.Model) {
		return provider.mantleResponsesStream(ctx, postHookRunner, postHookSpanFinalizer, key, request)
	}

	// Use Bedrock Converse streaming API for all other models
	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockResponsesRequest(ctx, request)
		})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	startTime := time.Now()

	resp, rakshaErr := provider.makeStreamingRequest(ctx, jsonData, key, request.Model, "converse-stream")
	latency := time.Since(startTime)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerUtils.ExtractProviderResponseHeadersFromHTTP(resp))

	// Create response channel
	responseChan := make(chan *schemas.RakshaStreamChunk, schemas.DefaultStreamBufferSize)

	providerUtils.SetStreamIdleTimeoutIfEmpty(ctx, provider.networkConfig.StreamIdleTimeoutInSeconds)

	// Start streaming in a goroutine
	go func() {
		defer providerUtils.EnsureStreamFinalizerCalled(ctx, postHookSpanFinalizer)
		defer func() {
			if ctx.Err() == context.Canceled {
				providerUtils.HandleStreamCancellation(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonData)
			} else if ctx.Err() == context.DeadlineExceeded {
				providerUtils.HandleStreamTimeout(ctx, postHookRunner, responseChan, provider.logger, postHookSpanFinalizer, jsonData)
			}
			providerUtils.CloseStream(ctx, responseChan)
		}()
		// Always release response on exit; bodyStream close should prevent indefinite blocking.
		defer resp.Body.Close()

		// Wrap body with idle timeout to detect stalled streams.
		idleReader, stopIdleTimeout := providerUtils.NewIdleTimeoutReader(resp.Body, resp.Body, providerUtils.GetStreamIdleTimeout(ctx), ctx)
		defer stopIdleTimeout()

		// Setup cancellation handler to close body stream on ctx cancellation
		stopCancellation := providerUtils.SetupStreamCancellation(ctx, resp.Body, provider.logger)
		defer stopCancellation()

		// Process AWS Event Stream format
		usage := &schemas.ResponsesResponseUsage{}
		billedUsage := &schemas.RakshaLLMUsage{}
		// Register the accumulating usage handle so a mid-stream cancel/timeout
		// can bill for Bedrock Responses usage already reported by stream events
		// before the stream was interrupted.
		ctx.SetValue(schemas.RakshaContextKeyStreamAccumulatedUsage, billedUsage)

		usageNormalized := false
		normalizeUsage := func() {
			if usageNormalized {
				return
			}
			usageNormalized = true
			normalizeCachedUsage(billedUsage)
		}
		defer func() {
			if ctx.Err() != nil {
				normalizeUsage()
			}
		}()

		var streamTrace *BedrockConverseTrace
		chunkIndex := 0

		// Create stream state for stateful conversions (used by Converse API path)
		streamState := acquireBedrockResponsesStreamState()
		streamState.Model = &request.Model
		streamState.Ctx = ctx
		defer releaseBedrockResponsesStreamState(streamState)

		// Check for structured output mode - if set, we need to intercept tool calls
		// and convert them to content instead of forwarding as tool calls
		var structuredOutputToolName string
		if toolName, ok := ctx.Value(schemas.RakshaContextKeyStructuredOutputToolName).(string); ok {
			structuredOutputToolName = toolName
		}
		var isAccumulatingStructuredOutput bool

		// Process AWS Event Stream format using proper decoder
		lastChunkTime := startTime
		decoder := eventstream.NewDecoder()
		payloadBuf := make([]byte, 0, 1024*1024) // 1MB payload buffer
		for {
			// If context was cancelled/timed out, let defer handle it
			if ctx.Err() != nil {
				return
			}
			// Decode a single EventStream message
			message, err := decoder.Decode(idleReader, payloadBuf)
			if err != nil {
				// If context was cancelled/timed out, let defer handle it
				if ctx.Err() != nil {
					return
				}
				if err == io.EOF {
					// Converse API: finalize any open items at end of stream.
					finalResponses := FinalizeBedrockStream(streamState, chunkIndex, usage, streamTrace)
					for i, finalResponse := range finalResponses {
						finalResponse.ExtraFields = schemas.RakshaResponseExtraFields{
							ChunkIndex: chunkIndex,
							Latency:    time.Since(lastChunkTime).Milliseconds(),
						}
						chunkIndex++
						lastChunkTime = time.Now()

						if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
							finalResponse.ExtraFields.RawResponse = "{}" // Final event has no payload
						}

						if i == len(finalResponses)-1 {
							// Set raw request if enabled
							ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
							if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
								providerUtils.ParseAndSetRawRequest(&finalResponse.ExtraFields, jsonData)
							}
							finalResponse.ExtraFields.Latency = time.Since(startTime).Milliseconds()
						}

						providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, finalResponse, nil, nil, nil), responseChan, postHookSpanFinalizer)
					}
					break
				}
				ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
				provider.logger.Warn("Error decoding EventStream message: %v", err)
				// Transport-level errors (stale/closed connection, unexpected EOF) are retryable.
				// Use IsRakshaError:false so the retry gate in executeRequestWithRetries can retry.
				if isStreamTransportError(err) {
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, &schemas.RakshaError{
						IsRakshaError: false,
						Error: &schemas.ErrorField{
							Message: schemas.ErrProviderNetworkError,
							Error:   err,
						},
					}, responseChan, provider.logger, postHookSpanFinalizer)
				} else {
					providerUtils.ProcessAndSendError(ctx, postHookRunner, err, responseChan, provider.logger, postHookSpanFinalizer)
				}
				return
			}

			// Process the decoded message payload (contains JSON for normal events)
			if len(message.Payload) > 0 {
				if msgTypeHeader := message.Headers.Get(":message-type"); msgTypeHeader != nil {
					if msgType := msgTypeHeader.String(); msgType != "event" {
						excType := msgType
						if excHeader := message.Headers.Get(":exception-type"); excHeader != nil {
							if v := excHeader.String(); v != "" {
								excType = v
							}
						}
						streamErr := newBedrockStreamException("", excType, message.Payload)
						providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, streamErr, responseChan, provider.logger, postHookSpanFinalizer)
						return
					}
				}

				// Converse API path: parse Bedrock Converse-specific stream events
				var streamEvent BedrockStreamEvent
				if err := sonic.Unmarshal(message.Payload, &streamEvent); err != nil {
					provider.logger.Debug("Failed to parse JSON from event buffer: %v, data: %s", err, string(message.Payload))
					providerUtils.ProcessAndSendError(ctx, postHookRunner, err, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}

				if streamEvent.Trace != nil {
					streamTrace = streamEvent.Trace
				}

				if streamEvent.Usage != nil {
					// Accumulate usage information instead of overwriting
					// In some cases usage comes in multiple events, so we need to take the maximum values
					accumulateBedrockResponsesUsage(usage, billedUsage, streamEvent.Usage)
				}

				// Handle structured output: intercept tool calls for the structured output tool
				// and convert them to content instead of forwarding as tool calls
				if structuredOutputToolName != "" {
					// Check for tool use start event
					if streamEvent.Start != nil && streamEvent.Start.ToolUse != nil {
						if streamEvent.Start.ToolUse.Name == structuredOutputToolName {
							// This is the structured output tool - start accumulating, don't forward
							isAccumulatingStructuredOutput = true
							streamState.UsedStructuredOutputTool = true
							continue
						}
					}

					// Check for tool use delta event
					if streamEvent.Delta != nil && streamEvent.Delta.ToolUse != nil && isAccumulatingStructuredOutput {
						// Convert tool use delta to text delta
						content := streamEvent.Delta.ToolUse.Input
						response := &schemas.RakshaResponsesStreamResponse{
							Type:           schemas.ResponsesStreamResponseTypeOutputTextDelta,
							SequenceNumber: chunkIndex,
							Delta:          &content,
							ExtraFields: schemas.RakshaResponseExtraFields{
								ChunkIndex: chunkIndex,
								Latency:    time.Since(lastChunkTime).Milliseconds(),
							},
						}
						chunkIndex++
						lastChunkTime = time.Now()

						if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
							response.ExtraFields.RawResponse = string(message.Payload)
						}

						providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
						continue
					}

					// Suppress non-tool content events that would leak into the
					// assembled structured output. Bedrock Claude can emit prose
					// alongside the forced tool call (markdown, preambles, reasoning
					// blocks); forwarding those as text deltas corrupts the JSON
					// the client assembles from the structured-output stream.
					if streamEvent.Delta != nil && (streamEvent.Delta.Text != nil || streamEvent.Delta.ReasoningContent != nil) {
						continue
					}
					if streamEvent.Start != nil && streamEvent.Start.ToolUse == nil {
						continue // non-tool content-block start (text block) — drop
					}
				}

				responses, rakshaErr, _ := streamEvent.ToRakshaResponsesStream(chunkIndex, streamState)
				if rakshaErr != nil {
					ctx.SetValue(schemas.RakshaContextKeyStreamEndIndicator, true)
					providerUtils.ProcessAndSendRakshaError(ctx, postHookRunner, rakshaErr, responseChan, provider.logger, postHookSpanFinalizer)
					return
				}
				for _, response := range responses {
					if response != nil {
						response.ExtraFields = schemas.RakshaResponseExtraFields{
							ChunkIndex: chunkIndex,
							Latency:    time.Since(lastChunkTime).Milliseconds(),
						}
						chunkIndex++
						lastChunkTime = time.Now()

						if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
							response.ExtraFields.RawResponse = string(message.Payload)
						}

						providerUtils.ProcessAndSendResponse(ctx, postHookRunner, providerUtils.GetRakshaResponseForStreamResponse(nil, nil, response, nil, nil, nil), responseChan, postHookSpanFinalizer)
					}
				}
			}
		}
	}()

	return responseChan, nil
}

// Embedding generates embeddings for the given input text(s) using Amazon Bedrock.
// Supports Titan and Cohere embedding models. Returns a RakshaResponse containing the embedding(s) and any error that occurred.
func (provider *BedrockProvider) Embedding(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaEmbeddingRequest) (*schemas.RakshaEmbeddingResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.EmbeddingRequest); err != nil {
		return nil, err
	}

	// Determine model type
	modelType, err := DetermineEmbeddingModelType(ctx, request.Model)
	if err != nil {
		return nil, providerUtils.NewConfigurationError(err.Error())
	}

	// Convert request and execute based on model type
	var rawResponse []byte
	var rakshaError *schemas.RakshaError
	var latency time.Duration
	var providerResponseHeaders map[string]string
	var path string
	var jsonData []byte

	switch modelType {
	case "titan":
		jsonData, rakshaError = providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				return ToBedrockTitanEmbeddingRequest(request)
			})
		if rakshaError != nil {
			return nil, rakshaError
		}
		path, _ = provider.getModelPathAndRegion(ctx, "invoke", request.Model, key)
		rawResponse, latency, providerResponseHeaders, rakshaError = provider.completeRequest(ctx, jsonData, path, key, request.Model)

	case "cohere":
		jsonData, rakshaError = providerUtils.CheckContextAndGetRequestBody(
			ctx,
			request,
			func() (providerUtils.RequestBodyWithExtraParams, error) {
				return ToBedrockCohereEmbeddingRequest(request)
			})
		if rakshaError != nil {
			return nil, rakshaError
		}
		path, _ = provider.getModelPathAndRegion(ctx, "invoke", request.Model, key)
		rawResponse, latency, providerResponseHeaders, rakshaError = provider.completeRequest(ctx, jsonData, path, key, request.Model)

	default:
		return nil, providerUtils.NewConfigurationError("unsupported embedding model type")
	}

	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaError != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaError, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}
	// Parse response based on model type
	var rakshaResponse *schemas.RakshaEmbeddingResponse
	switch modelType {
	case "titan":
		var titanResp BedrockTitanEmbeddingResponse
		if err := sonic.Unmarshal(rawResponse, &titanResp); err != nil {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error parsing Titan embedding response", err), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		rakshaResponse = titanResp.ToRakshaEmbeddingResponse()
		rakshaResponse.Model = request.Model

	case "cohere":
		var cohereResp BedrockCohereEmbeddingResponse
		if err := sonic.Unmarshal(rawResponse, &cohereResp); err != nil {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error parsing Cohere embedding response", err), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		converted, convErr := cohereResp.ToRakshaEmbeddingResponse()
		if convErr != nil {
			return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error parsing Cohere embedding response", convErr), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
		}
		rakshaResponse = converted
		rakshaResponse.Model = request.Model
		// For embeddings_by_type responses preserve the raw Bedrock payload so the
		// invoke-endpoint converter can return all encoding variants verbatim, since
		// the internal RakshaEmbeddingResponse only has float32 and string fields.
		if cohereResp.ResponseType == "embeddings_by_type" {
			var rawResponseData interface{}
			if err := sonic.Unmarshal(rawResponse, &rawResponseData); err == nil {
				rakshaResponse.ExtraFields.RawResponse = rawResponseData
			}
		}
	}

	// Bedrock Cohere embed models omit token usage from the response body and instead
	// return it in the X-Amzn-Bedrock-Input-Token-Count response header. Backfill Usage
	// from that header when the body did not provide it. (#3917)
	if rakshaResponse.Usage == nil {
		if inputTokens, ok := inputTokensFromHeaders(providerResponseHeaders); ok {
			rakshaResponse.Usage = &schemas.RakshaLLMUsage{
				PromptTokens: inputTokens,
				TotalTokens:  inputTokens,
			}
		}
	}

	// Set ExtraFields
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw response if enabled
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponseData interface{}
		if err := sonic.Unmarshal(rawResponse, &rawResponseData); err == nil {
			rakshaResponse.ExtraFields.RawResponse = rawResponseData
		}
	}

	return rakshaResponse, nil
}

// Speech is not supported by the Bedrock provider.
func (provider *BedrockProvider) Speech(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaSpeechRequest) (*schemas.RakshaSpeechResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechRequest, schemas.Bedrock)
}

// Rerank performs a rerank request using the Bedrock Agent Runtime /rerank API.
func (provider *BedrockProvider) Rerank(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaRerankRequest) (*schemas.RakshaRerankResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.RerankRequest); err != nil {
		return nil, err
	}

	if !strings.HasPrefix(request.Model, "arn:") {
		return nil, providerUtils.NewConfigurationError(fmt.Sprintf("bedrock rerank requires an ARN model identifier; got %q", request.Model))
	}

	jsonData, rakshaErr := providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockRerankRequest(request, request.Model)
		},
	)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	rawResponseBody, latency, providerResponseHeaders, rakshaErr := provider.completeAgentRuntimeRequest(ctx, jsonData, "/rerank", key)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	response := &BedrockRerankResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(rawResponseBody, response, jsonData, providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest), providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse))
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, rawResponseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	returnDocuments := request.Params != nil && request.Params.ReturnDocuments != nil && *request.Params.ReturnDocuments
	rakshaResponse := response.ToRakshaRerankResponse(request.Documents, returnDocuments)
	rakshaResponse.Model = request.Model

	// Bedrock returns rerank input token usage only in the X-Amzn-Bedrock-Input-Token-Count
	// response header (it is absent from the body); backfill Usage from it. (#3917)
	if rakshaResponse.Usage == nil {
		if inputTokens, ok := inputTokensFromHeaders(providerResponseHeaders); ok {
			rakshaResponse.Usage = &schemas.RakshaLLMUsage{
				PromptTokens: inputTokens,
				TotalTokens:  inputTokens,
			}
		}
	}

	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		rakshaResponse.ExtraFields.RawRequest = rawRequest
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaResponse.ExtraFields.RawResponse = rawResponse
	}

	return rakshaResponse, nil
}

// OCR is not supported by the Bedrock provider.
func (provider *BedrockProvider) OCR(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaOCRRequest) (*schemas.RakshaOCRResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.OCRRequest, provider.GetProviderKey())
}

// SpeechStream is not supported by the Bedrock provider.
func (provider *BedrockProvider) SpeechStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaSpeechRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.SpeechStreamRequest, schemas.Bedrock)
}

// Transcription is not supported by the Bedrock provider.
func (provider *BedrockProvider) Transcription(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaTranscriptionRequest) (*schemas.RakshaTranscriptionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionRequest, schemas.Bedrock)
}

// TranscriptionStream is not supported by the Bedrock provider.
func (provider *BedrockProvider) TranscriptionStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaTranscriptionRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.TranscriptionStreamRequest, schemas.Bedrock)
}

// ImageGeneration generates images using Amazon Bedrock.
// Supports Titan Image Generator v1, Nova Canvas v1, Titan Image Generator v2, and Stability AI models.
// Returns a RakshaImageGenerationResponse containing the generated images and any error that occurred.
func (provider *BedrockProvider) ImageGeneration(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageGenerationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ImageGenerationRequest); err != nil {
		return nil, err
	}

	var rawResponse []byte
	var jsonData []byte
	var rakshaError *schemas.RakshaError
	var latency time.Duration
	var providerResponseHeaders map[string]string
	var path string

	path, _ = provider.getModelPathAndRegion(ctx, "invoke", request.Model, key)

	jsonData, rakshaError = providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			if isStabilityAIModel(request.Model) {
				return ToStabilityAIImageGenerationRequest(request)
			}
			return ToBedrockImageGenerationRequest(request)
		})
	if rakshaError != nil {
		return nil, rakshaError
	}
	rawResponse, latency, providerResponseHeaders, rakshaError = provider.completeRequest(ctx, jsonData, path, key, request.Model)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaError != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaError, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse response based on model type
	var rakshaResponse *schemas.RakshaImageGenerationResponse
	var imageResp BedrockImageGenerationResponse
	if err := sonic.Unmarshal(rawResponse, &imageResp); err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error parsing image generation response", err), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if imageResp.Error != "" {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(imageResp.Error, nil), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	rakshaResponse = ToRakshaImageGenerationResponse(&imageResp)
	rakshaResponse.Model = request.Model
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&rakshaResponse.ExtraFields, jsonData)
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponseData interface{}
		if err := sonic.Unmarshal(rawResponse, &rawResponseData); err == nil {
			rakshaResponse.ExtraFields.RawResponse = rawResponseData
		}
	}

	return rakshaResponse, nil
}

// ImageGenerationStream is not supported by the Bedrock provider.
func (provider *BedrockProvider) ImageGenerationStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageGenerationRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageGenerationStreamRequest, schemas.Bedrock)
}

// ImageEdit performs image editing using Amazon Bedrock.
// Supports Titan Image Generator v1, Nova Canvas v1, Titan Image Generator v2 (three edit types:
// INPAINTING, OUTPAINTING, BACKGROUND_REMOVAL), and Stability AI edit models (inpaint, outpaint,
// recolor, search-replace, erase-object, remove-bg, control-sketch, control-structure, style-guide,
// style-transfer, upscale-creative, upscale-conservative, upscale-fast).
// Returns a RakshaImageGenerationResponse containing the edited images and any error that occurred.
func (provider *BedrockProvider) ImageEdit(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageEditRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ImageEditRequest); err != nil {
		return nil, err
	}

	var jsonData []byte
	var rakshaError *schemas.RakshaError

	// Stability AI routing and task-type inference use the actual model ID.
	path, _ := provider.getModelPathAndRegion(ctx, "invoke", request.Model, key)

	jsonData, rakshaError = providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			if isStabilityAIModel(request.Model) {
				return ToStabilityAIImageEditRequest(request, request.Model)
			}
			return ToBedrockImageEditRequest(request)
		})
	if rakshaError != nil {
		return nil, rakshaError
	}

	// Make API request (same URL as image generation)
	rawResponse, latency, providerResponseHeaders, rakshaError := provider.completeRequest(ctx, jsonData, path, key, request.Model)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaError != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaError, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse response (reuse BedrockImageGenerationResponse)
	var imageResp BedrockImageGenerationResponse
	if err := sonic.Unmarshal(rawResponse, &imageResp); err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error parsing image edit response", err), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if imageResp.Error != "" {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(imageResp.Error, nil), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Convert response and set metadata
	rakshaResponse := ToRakshaImageGenerationResponse(&imageResp)
	rakshaResponse.Model = request.Model
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request/response if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&rakshaResponse.ExtraFields, jsonData)
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponseData interface{}
		if err := sonic.Unmarshal(rawResponse, &rawResponseData); err == nil {
			rakshaResponse.ExtraFields.RawResponse = rawResponseData
		}
	}

	return rakshaResponse, nil
}

// ImageEditStream is not supported by the Bedrock provider.
func (provider *BedrockProvider) ImageEditStream(ctx *schemas.RakshaContext, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context), key schemas.Key, request *schemas.RakshaImageEditRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ImageEditStreamRequest, provider.GetProviderKey())
}

// ImageVariation generates image variations using Amazon Bedrock.
// Supports Titan Image Generator v1, Nova Canvas v1, and Titan Image Generator v2.
// Returns a RakshaImageGenerationResponse containing the generated image variations and any error that occurred.
func (provider *BedrockProvider) ImageVariation(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaImageVariationRequest) (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.ImageVariationRequest); err != nil {
		return nil, err
	}

	var jsonData []byte
	var rakshaError *schemas.RakshaError

	jsonData, rakshaError = providerUtils.CheckContextAndGetRequestBody(
		ctx,
		request,
		func() (providerUtils.RequestBodyWithExtraParams, error) {
			return ToBedrockImageVariationRequest(request)
		})
	if rakshaError != nil {
		return nil, rakshaError
	}

	// Make API request (same URL as image generation)
	path, _ := provider.getModelPathAndRegion(ctx, "invoke", request.Model, key)
	rawResponse, latency, providerResponseHeaders, rakshaError := provider.completeRequest(ctx, jsonData, path, key, request.Model)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaError != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaError, jsonData, nil, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse response (reuse BedrockImageGenerationResponse and ToRakshaImageGenerationResponse)
	var imageResp BedrockImageGenerationResponse
	if err := sonic.Unmarshal(rawResponse, &imageResp); err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error parsing image variation response", err), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	if imageResp.Error != "" {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(imageResp.Error, nil), jsonData, rawResponse, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Convert response and set metadata
	rakshaResponse := ToRakshaImageGenerationResponse(&imageResp)
	rakshaResponse.Model = request.Model
	rakshaResponse.ExtraFields.Latency = latency.Milliseconds()
	rakshaResponse.ExtraFields.ProviderResponseHeaders = providerResponseHeaders

	// Set raw request/response if enabled
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequest(&rakshaResponse.ExtraFields, jsonData)
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		var rawResponseData interface{}
		if err := sonic.Unmarshal(rawResponse, &rawResponseData); err == nil {
			rakshaResponse.ExtraFields.RawResponse = rawResponseData
		}
	}

	return rakshaResponse, nil
}

// VideoGeneration is not supported by the Bedrock provider.
func (provider *BedrockProvider) VideoGeneration(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoGenerationRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoGenerationRequest, provider.GetProviderKey())
}

// VideoRetrieve is not supported by the Bedrock provider.
func (provider *BedrockProvider) VideoRetrieve(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRetrieveRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRetrieveRequest, provider.GetProviderKey())
}

// VideoDownload is not supported by the Bedrock provider.
func (provider *BedrockProvider) VideoDownload(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDownloadRequest) (*schemas.RakshaVideoDownloadResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDownloadRequest, provider.GetProviderKey())
}

// VideoDelete is not supported by Bedrock provider.
func (provider *BedrockProvider) VideoDelete(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoDeleteRequest) (*schemas.RakshaVideoDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoDeleteRequest, provider.GetProviderKey())
}

// VideoList is not supported by Bedrock provider.
func (provider *BedrockProvider) VideoList(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoListRequest) (*schemas.RakshaVideoListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoListRequest, provider.GetProviderKey())
}

// VideoRemix is not supported by Bedrock provider.
func (provider *BedrockProvider) VideoRemix(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaVideoRemixRequest) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.VideoRemixRequest, provider.GetProviderKey())
}

// FileUpload uploads a file to S3 for Bedrock batch processing.
func (provider *BedrockProvider) FileUpload(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaFileUploadRequest) (*schemas.RakshaFileUploadResponse, *schemas.RakshaError) {

	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.FileUploadRequest); err != nil {
		if err.Error != nil {
			provider.logger.Error("file upload operation not allowed: %s", err.Error.Message)
		}
		return nil, err
	}

	// Get S3 bucket from storage config or extra params
	s3Bucket := ""
	s3Prefix := ""
	if request.StorageConfig != nil && request.StorageConfig.S3 != nil {
		if request.StorageConfig.S3.Bucket != "" {
			s3Bucket = request.StorageConfig.S3.Bucket
		}
		if request.StorageConfig.S3.Prefix != "" {
			s3Prefix = request.StorageConfig.S3.Prefix
		}
	} else if request.ExtraParams != nil {
		if bucket, ok := request.ExtraParams["s3_bucket"].(string); ok && bucket != "" {
			s3Bucket = bucket
		}
		if prefix, ok := request.ExtraParams["s3_prefix"].(string); ok && prefix != "" {
			s3Prefix = prefix
		}
	}

	if s3Bucket == "" {
		provider.logger.Error("s3_bucket is required for Bedrock file operations (provide in storage_config.s3 or extra_params)")
		return nil, providerUtils.NewRakshaOperationError("s3_bucket is required for Bedrock file operations (provide in storage_config.s3 or extra_params)", nil)
	}

	// Parse bucket name and optional prefix from s3Bucket (could be "bucket-name" or "s3://bucket-name/prefix/")
	bucketName, bucketPrefix := parseS3URI(s3Bucket)
	if bucketPrefix != "" {
		s3Prefix = bucketPrefix + s3Prefix
	}

	region := DefaultBedrockRegion
	if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
		region = key.BedrockKeyConfig.Region.GetValue()
	}

	// Generate S3 key for the file
	filename := request.Filename
	if filename == "" {
		filename = fmt.Sprintf("file-%d.jsonl", time.Now().UnixNano())
	}

	cleanedPrefix := strings.Trim(s3Prefix, "/")
	s3Key := filename
	if cleanedPrefix != "" {
		s3Key = cleanedPrefix + "/" + filename
	}

	provider.logger.Debug("uploading file to s3: %s", s3Key)

	// Build S3 PUT request URL
	// Escape each path segment individually to handle special characters while preserving "/"
	reqURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, escapeS3KeyForURL(s3Key))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(request.File))
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("error creating request", err)
	}

	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.ContentLength = int64(len(request.File))

	// Sign request for S3
	if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, "s3"); err != nil {
		provider.logger.Error("error signing request: %s", err.Error.Message)
		return nil, err
	}

	// Execute request
	startTime := time.Now()
	resp, err := provider.client.Do(httpReq)
	latency := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}
		}
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		provider.logger.Error("s3 upload failed: %d", resp.StatusCode)
		return nil, providerUtils.NewProviderAPIError(fmt.Sprintf("S3 upload failed: %s", string(body)), nil, resp.StatusCode, nil, nil)
	}

	// Return S3 URI as the file ID
	s3URI := fmt.Sprintf("s3://%s/%s", bucketName, s3Key)

	return &schemas.RakshaFileUploadResponse{
		ID:             s3URI,
		Object:         "file",
		Bytes:          int64(len(request.File)),
		CreatedAt:      time.Now().Unix(),
		Filename:       filename,
		Purpose:        request.Purpose,
		Status:         schemas.FileStatusProcessed,
		StorageBackend: schemas.FileStorageS3,
		StorageURI:     s3URI,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, nil
}

// FileList lists files in the S3 bucket used for Bedrock batch processing from all provided keys.
// FileList lists S3 files using serial pagination across keys.
// Exhausts all pages from one key before moving to the next.
func (provider *BedrockProvider) FileList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileListRequest) (*schemas.RakshaFileListResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.FileListRequest); err != nil {
		return nil, err
	}

	// Get S3 bucket from storage config or extra params
	s3Bucket := ""
	s3Prefix := ""
	if request.StorageConfig != nil && request.StorageConfig.S3 != nil {
		if request.StorageConfig.S3.Bucket != "" {
			s3Bucket = request.StorageConfig.S3.Bucket
		}
		if request.StorageConfig.S3.Prefix != "" {
			s3Prefix = request.StorageConfig.S3.Prefix
		}
	}
	if request.ExtraParams != nil {
		if bucket, ok := request.ExtraParams["s3_bucket"].(string); ok && bucket != "" {
			s3Bucket = bucket
		}
		if prefix, ok := request.ExtraParams["s3_prefix"].(string); ok && prefix != "" {
			s3Prefix = prefix
		}
	}

	if s3Bucket == "" {
		return nil, providerUtils.NewRakshaOperationError("s3_bucket is required for Bedrock file operations (provide in storage_config.s3 or extra_params)", nil)
	}

	bucketName, bucketPrefix := parseS3URI(s3Bucket)
	if bucketPrefix != "" {
		s3Prefix = bucketPrefix + s3Prefix
	}

	// Initialize serial pagination helper
	helper, err := providerUtils.NewSerialListHelper(keys, request.After, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.RakshaFileListResponse{
			Object:  "list",
			Data:    []schemas.FileObject{},
			HasMore: false,
		}, nil
	}

	region := DefaultBedrockRegion
	if key.BedrockKeyConfig != nil {
		if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
			region = key.BedrockKeyConfig.Region.GetValue()
		}
	}

	// Build S3 ListObjectsV2 request
	params := url.Values{}
	params.Set("list-type", "2")
	params.Set("prefix", s3Prefix)
	if request.Limit > 0 {
		params.Set("max-keys", fmt.Sprintf("%d", request.Limit))
	}
	// Use native cursor from serial helper
	if nativeCursor != "" {
		params.Set("continuation-token", nativeCursor)
	}

	requestURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/?%s", bucketName, region, params.Encode())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("error creating request", err)
	}

	// Sign request for S3
	if rakshaErr := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, "s3"); rakshaErr != nil {
		return nil, rakshaErr
	}

	// Execute request
	startTime := time.Now()
	resp, err := provider.client.Do(httpReq)
	latency := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}
		}
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("error reading response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, providerUtils.NewProviderAPIError(fmt.Sprintf("S3 list failed: %s", string(body)), nil, resp.StatusCode, nil, nil)
	}

	// Parse S3 ListObjectsV2 XML response
	var listResp S3ListObjectsResponse
	if err := parseS3ListResponse(body, &listResp); err != nil {
		return nil, providerUtils.NewRakshaOperationError("error parsing S3 response", err)
	}

	// Convert files to Raksha format
	files := make([]schemas.FileObject, 0, len(listResp.Contents))
	for _, obj := range listResp.Contents {
		s3URI := fmt.Sprintf("s3://%s/%s", bucketName, obj.Key)
		filename := obj.Key
		if idx := strings.LastIndex(obj.Key, "/"); idx >= 0 {
			filename = obj.Key[idx+1:]
		}
		files = append(files, schemas.FileObject{
			ID:        s3URI,
			Object:    "file",
			Bytes:     obj.Size,
			CreatedAt: obj.LastModified.Unix(),
			Filename:  filename,
			Purpose:   schemas.FilePurposeBatch,
			Status:    schemas.FileStatusProcessed,
		})
	}

	// Build cursor for next request
	// S3 uses NextContinuationToken for pagination
	nextCursor, hasMore := helper.BuildNextCursor(listResp.IsTruncated, listResp.NextContinuationToken)

	// Convert to Raksha response
	rakshaResp := &schemas.RakshaFileListResponse{
		Object:  "list",
		Data:    files,
		HasMore: hasMore,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}
	if nextCursor != "" {
		rakshaResp.After = &nextCursor
	}

	return rakshaResp, nil
}

// FileRetrieve retrieves S3 object metadata for Bedrock batch processing by trying each key until found.
func (provider *BedrockProvider) FileRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileRetrieveRequest) (*schemas.RakshaFileRetrieveResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.FileRetrieveRequest); err != nil {
		return nil, err
	}

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("file_id (S3 URI) is required", nil)
	}

	// Parse S3 URI
	bucketName, s3Key := parseS3URI(request.FileID)
	if bucketName == "" || s3Key == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid S3 URI format, expected s3://bucket/key", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		region := DefaultBedrockRegion
		if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
			region = key.BedrockKeyConfig.Region.GetValue()
		}

		// Build S3 HEAD request
		// Escape each path segment individually to handle special characters while preserving "/"
		reqURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, escapeS3KeyForURL(s3Key))

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodHead, reqURL, nil)
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error creating request", err)
			continue
		}

		// Sign request for S3
		if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, "s3"); err != nil {
			lastErr = err
			continue
		}

		// Execute request
		startTime := time.Now()
		resp, err := provider.client.Do(httpReq)
		latency := time.Since(startTime)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, &schemas.RakshaError{
					IsRakshaError: false,
					Error: &schemas.ErrorField{
						Type:    schemas.Ptr(schemas.RequestCancelled),
						Message: schemas.ErrRequestCancelled,
						Error:   err,
					},
				}
			}
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = providerUtils.NewProviderAPIError(fmt.Sprintf("S3 HEAD failed with status %d", resp.StatusCode), nil, resp.StatusCode, nil, nil)
			continue
		}

		resp.Body.Close()

		// Extract metadata from headers
		filename := s3Key
		if idx := strings.LastIndex(s3Key, "/"); idx >= 0 {
			filename = s3Key[idx+1:]
		}

		var createdAt int64
		if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
			if t, err := time.Parse(time.RFC1123, lastMod); err == nil {
				createdAt = t.Unix()
			}
		}

		return &schemas.RakshaFileRetrieveResponse{
			ID:             request.FileID,
			Object:         "file",
			Bytes:          resp.ContentLength,
			CreatedAt:      createdAt,
			Filename:       filename,
			Purpose:        schemas.FilePurposeBatch,
			Status:         schemas.FileStatusProcessed,
			StorageBackend: schemas.FileStorageS3,
			StorageURI:     request.FileID,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}, nil
	}

	return nil, lastErr
}

// FileDelete deletes an S3 object used for Bedrock batch processing by trying each key until successful.
func (provider *BedrockProvider) FileDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileDeleteRequest) (*schemas.RakshaFileDeleteResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.FileDeleteRequest); err != nil {
		return nil, err
	}

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("file_id (S3 URI) is required", nil)
	}

	// Parse S3 URI
	bucketName, s3Key := parseS3URI(request.FileID)
	if bucketName == "" || s3Key == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid S3 URI format, expected s3://bucket/key", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		region := DefaultBedrockRegion
		if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
			region = key.BedrockKeyConfig.Region.GetValue()
		}

		// Build S3 DELETE request
		// Escape each path segment individually to handle special characters while preserving "/"
		reqURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, escapeS3KeyForURL(s3Key))

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error creating request", err)
			continue
		}

		// Sign request for S3
		if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, "s3"); err != nil {
			lastErr = err
			continue
		}

		// Execute request
		startTime := time.Now()
		resp, err := provider.client.Do(httpReq)
		latency := time.Since(startTime)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, &schemas.RakshaError{
					IsRakshaError: false,
					Error: &schemas.ErrorField{
						Type:    schemas.Ptr(schemas.RequestCancelled),
						Message: schemas.ErrRequestCancelled,
						Error:   err,
					},
				}
			}
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
			continue
		}

		// S3 DELETE returns 204 No Content on success
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = providerUtils.NewProviderAPIError(fmt.Sprintf("S3 DELETE failed: %s", string(body)), nil, resp.StatusCode, nil, nil)
			continue
		}

		resp.Body.Close()

		return &schemas.RakshaFileDeleteResponse{
			ID:      request.FileID,
			Object:  "file",
			Deleted: true,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}, nil
	}

	return nil, lastErr
}

// FileContent downloads S3 object content for Bedrock batch processing by trying each key until found.
func (provider *BedrockProvider) FileContent(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaFileContentRequest) (*schemas.RakshaFileContentResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.FileContentRequest); err != nil {
		return nil, err
	}

	if request.FileID == "" {
		return nil, providerUtils.NewRakshaOperationError("file_id (S3 URI) is required", nil)
	}

	// Parse S3 URI
	bucketName, s3Key := parseS3URI(request.FileID)
	if bucketName == "" || s3Key == "" {
		return nil, providerUtils.NewRakshaOperationError("invalid S3 URI format, expected s3://bucket/key", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		region := DefaultBedrockRegion
		if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
			region = key.BedrockKeyConfig.Region.GetValue()
		}

		// Build S3 GET request
		// Escape each path segment individually to handle special characters while preserving "/"
		reqURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, escapeS3KeyForURL(s3Key))

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error creating request", err)
			continue
		}

		// Sign request for S3
		if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, "s3"); err != nil {
			lastErr = err
			continue
		}

		// Execute request
		startTime := time.Now()
		resp, err := provider.client.Do(httpReq)
		latency := time.Since(startTime)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, &schemas.RakshaError{
					IsRakshaError: false,
					Error: &schemas.ErrorField{
						Type:    schemas.Ptr(schemas.RequestCancelled),
						Message: schemas.ErrRequestCancelled,
						Error:   err,
					},
				}
			}
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = providerUtils.NewProviderAPIError(fmt.Sprintf("S3 GET failed: %s", string(body)), nil, resp.StatusCode, nil, nil)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error reading S3 object content", err)
			continue
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		return &schemas.RakshaFileContentResponse{
			FileID:      request.FileID,
			Content:     body,
			ContentType: contentType,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}, nil
	}

	return nil, lastErr
}

// BatchCreate creates a new batch inference job on AWS Bedrock.
func (provider *BedrockProvider) BatchCreate(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaBatchCreateRequest) (*schemas.RakshaBatchCreateResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.BatchCreateRequest); err != nil {
		provider.logger.Error("batch create is not allowed for Bedrock provider", "error", err)
		return nil, err
	}

	// Require RoleArn in extra params
	roleArn := ""
	// First we will honor the role_arn coming from the client side if present
	if request.ExtraParams != nil {
		if r, ok := request.ExtraParams["role_arn"].(string); ok {
			roleArn = r
		}
	}
	// If its empty then we will honor the role_arn from the key config
	if roleArn == "" {
		if key.BedrockKeyConfig.RoleARN != nil {
			roleArn = key.BedrockKeyConfig.RoleARN.GetValue()
		}
	}
	// And if still we don't get role ARN
	if roleArn == "" {
		provider.logger.Error("role_arn is required for Bedrock batch API (provide in extra_params)")
		return nil, providerUtils.NewRakshaOperationError("role_arn is required for Bedrock batch API (provide in extra_params)", nil)
	}
	// Get output S3 URI from extra params
	outputS3Uri := ""
	if request.ExtraParams != nil {
		if o, ok := request.ExtraParams["output_s3_uri"].(string); ok {
			outputS3Uri = o
		}
	}
	if outputS3Uri == "" {
		provider.logger.Error("output_s3_uri is required for Bedrock batch API (provide in extra_params)")
		return nil, providerUtils.NewRakshaOperationError("output_s3_uri is required for Bedrock batch API (provide in extra_params)", nil)
	}

	if request.Model == nil {
		provider.logger.Error("model is required for Bedrock batch API")
		return nil, providerUtils.NewRakshaOperationError("model is required for Bedrock batch API", nil)
	}

	// Generate job name
	jobName := fmt.Sprintf("raksha-batch-%d", time.Now().Unix())
	if request.Metadata != nil {
		if name, ok := request.Metadata["job_name"]; ok {
			jobName = name
		}
	}

	// Determine input file ID (S3 URI)
	inputFileID := request.InputFileID

	// If no S3 URI provided but inline requests are available, upload them to S3 first
	if inputFileID == "" && len(request.Requests) > 0 {
		// Get region for S3 upload
		region := DefaultBedrockRegion
		if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
			region = key.BedrockKeyConfig.Region.GetValue()
		}

		var sessionKey *string
		if key.BedrockKeyConfig.SessionToken != nil && key.BedrockKeyConfig.SessionToken.GetValue() != "" {
			sessionKey = schemas.Ptr(key.BedrockKeyConfig.SessionToken.GetValue())
		}

		// Convert inline requests to Bedrock JSONL format
		jsonlData, err := ConvertBedrockRequestsToJSONL(request.Requests, request.Model)
		if err != nil {
			return nil, providerUtils.NewRakshaOperationError("failed to convert requests to JSONL", err)
		}

		// Generate S3 key for the input file
		inputKey := generateBatchInputS3Key(jobName)

		// Derive bucket from output S3 URI
		inputS3URI := deriveInputS3URIFromOutput(outputS3Uri, inputKey)
		bucket, s3Key := parseS3URI(inputS3URI)

		// Upload to S3 using Bedrock credentials
		if rakshaErr := uploadToS3(
			ctx,
			key.BedrockKeyConfig.AccessKey.GetValue(),
			key.BedrockKeyConfig.SecretKey.GetValue(),
			sessionKey,
			region,
			bucket,
			s3Key,
			jsonlData,
		); rakshaErr != nil {
			return nil, rakshaErr
		}

		inputFileID = inputS3URI
	}

	// Validate that we have an input file ID (either provided or uploaded)
	if inputFileID == "" {
		provider.logger.Error("either input_file_id (S3 URI) or requests array is required for Bedrock batch API")
		return nil, providerUtils.NewRakshaOperationError("either input_file_id (S3 URI) or requests array is required for Bedrock batch API", nil)
	}

	// Build request
	bedrockReq := &BedrockBatchJobRequest{
		JobName: jobName,
		ModelID: request.Model,
		RoleArn: roleArn,
		InputDataConfig: BedrockInputDataConfig{
			S3InputDataConfig: BedrockS3InputDataConfig{
				S3Uri:         inputFileID,
				S3InputFormat: "JSONL",
			},
		},
		OutputDataConfig: BedrockOutputDataConfig{
			S3OutputDataConfig: BedrockS3OutputDataConfig{
				S3Uri: outputS3Uri,
			},
		},
	}

	// Set timeout if provided
	if request.CompletionWindow != "" {
		// Parse completion window (e.g., "24h" -> 24)
		if d, err := time.ParseDuration(request.CompletionWindow); err == nil {
			bedrockReq.TimeoutDurationInHours = int(d.Hours())
		}
	}

	jsonData, err := providerUtils.MarshalSorted(bedrockReq)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, err)
	}

	sendBackRawRequest := provider.sendBackRawRequest
	sendBackRawResponse := provider.sendBackRawResponse

	region := DefaultBedrockRegion
	if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
		region = key.BedrockKeyConfig.Region.GetValue()
	}

	// Create HTTP request
	reqURL := fmt.Sprintf("https://bedrock.%s.amazonaws.com/model-invocation-job", region)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error creating request", err), jsonData, nil, sendBackRawRequest, sendBackRawResponse)
	}

	// Sign request
	if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, bedrockSigningService); err != nil {
		return nil, providerUtils.EnrichError(ctx, err, jsonData, nil, sendBackRawRequest, sendBackRawResponse)
	}

	// Execute request
	startTime := time.Now()
	resp, err := provider.client.Do(httpReq)
	latency := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, providerUtils.EnrichError(ctx, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}, jsonData, nil, sendBackRawRequest, sendBackRawResponse)
		}
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err), jsonData, nil, sendBackRawRequest, sendBackRawResponse)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError("error reading response", err), jsonData, nil, sendBackRawRequest, sendBackRawResponse)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, providerUtils.EnrichError(ctx, parseBedrockHTTPError(resp.StatusCode, resp.Header, body), jsonData, body, sendBackRawRequest, sendBackRawResponse)
	}

	var bedrockResp BedrockBatchJobResponse
	if err := sonic.Unmarshal(body, &bedrockResp); err != nil {
		return nil, providerUtils.EnrichError(ctx, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err), jsonData, body, sendBackRawRequest, sendBackRawResponse)
	}

	// AWS CreateModelInvocationJob only returns jobArn, not status or other details.
	// Retrieve the job to get full status details.
	retrieveResp, rakshaErr := provider.BatchRetrieve(ctx, []schemas.Key{key}, &schemas.RakshaBatchRetrieveRequest{
		Provider: request.Provider,
		BatchID:  bedrockResp.JobArn,
	})
	if rakshaErr != nil {
		// Return basic response if retrieve fails
		return &schemas.RakshaBatchCreateResponse{
			ID:          bedrockResp.JobArn,
			Object:      "batch",
			InputFileID: inputFileID,
			Status:      schemas.BatchStatusValidating,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}, nil
	}

	// Use retrieved response for complete data
	result := &schemas.RakshaBatchCreateResponse{
		ID:          retrieveResp.ID,
		Object:      "batch",
		InputFileID: inputFileID,
		Status:      retrieveResp.Status,
		CreatedAt:   retrieveResp.CreatedAt,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}

	if retrieveResp.ExpiresAt != nil {
		result.ExpiresAt = retrieveResp.ExpiresAt
	}

	return result, nil
}

// BatchList lists batch inference jobs using serial pagination across keys.
// Exhausts all pages from one key before moving to the next.
func (provider *BedrockProvider) BatchList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchListRequest) (*schemas.RakshaBatchListResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.BatchListRequest); err != nil {
		return nil, err
	}

	// Initialize serial pagination helper (Bedrock uses PageToken for pagination)
	helper, err := providerUtils.NewSerialListHelper(keys, request.PageToken, provider.logger, true)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("invalid pagination cursor", err)
	}

	// Get current key to query
	key, nativeCursor, ok := helper.GetCurrentKey()
	if !ok {
		// All keys exhausted
		return &schemas.RakshaBatchListResponse{
			Object:  "list",
			Data:    []schemas.RakshaBatchRetrieveResponse{},
			HasMore: false,
		}, nil
	}

	region := DefaultBedrockRegion
	if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
		region = key.BedrockKeyConfig.Region.GetValue()
	}

	// Build URL with query params
	params := url.Values{}
	if request.Limit > 0 {
		params.Set("maxResults", fmt.Sprintf("%d", request.Limit))
	}
	// Use native cursor from serial helper
	if nativeCursor != "" {
		params.Set("nextToken", nativeCursor)
	}

	reqURL := fmt.Sprintf("https://bedrock.%s.amazonaws.com/model-invocation-jobs", region)
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("error creating request", err)
	}

	// Sign request
	if rakshaErr := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, bedrockSigningService); rakshaErr != nil {
		return nil, rakshaErr
	}

	// Execute request
	startTime := time.Now()
	resp, err := provider.client.Do(httpReq)
	latency := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Type:    schemas.Ptr(schemas.RequestCancelled),
					Message: schemas.ErrRequestCancelled,
					Error:   err,
				},
			}
		}
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("error reading response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseBedrockHTTPError(resp.StatusCode, resp.Header, body)
	}

	var bedrockResp BedrockBatchJobListResponse
	if err := sonic.Unmarshal(body, &bedrockResp); err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	// Convert batches to Raksha format
	batches := make([]schemas.RakshaBatchRetrieveResponse, 0, len(bedrockResp.InvocationJobSummaries))
	for _, job := range bedrockResp.InvocationJobSummaries {
		var createdAt int64
		if job.SubmitTime != nil {
			createdAt = job.SubmitTime.Unix()
		}

		// Store Bedrock-specific fields in Metadata for later conversion back to Bedrock format
		metadata := make(map[string]string)
		if job.JobName != "" {
			metadata["job_name"] = job.JobName
		}
		if job.ModelID != "" {
			metadata["model_id"] = job.ModelID
		}

		batches = append(batches, schemas.RakshaBatchRetrieveResponse{
			ID:        job.JobArn,
			Object:    "batch",
			Status:    ToRakshaBatchStatus(job.Status),
			CreatedAt: createdAt,
			Metadata:  metadata,
		})
	}

	// Build cursor for next request
	// Bedrock uses NextToken for pagination
	nativeNextToken := ""
	apiHasMore := false
	if bedrockResp.NextToken != nil && *bedrockResp.NextToken != "" {
		nativeNextToken = *bedrockResp.NextToken
		apiHasMore = true
	}
	nextCursor, hasMore := helper.BuildNextCursor(apiHasMore, nativeNextToken)

	// Convert to Raksha response
	rakshaResp := &schemas.RakshaBatchListResponse{
		Object:  "list",
		Data:    batches,
		HasMore: hasMore,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}
	if nextCursor != "" {
		rakshaResp.NextCursor = &nextCursor
	}

	return rakshaResp, nil
}

// fetchBatchManifest fetches the manifest.json.out from S3 to get record counts.
// Returns nil if manifest doesn't exist (job still in progress) or on error.
func (provider *BedrockProvider) fetchBatchManifest(ctx *schemas.RakshaContext, key schemas.Key, region, outputS3Uri string) *BedrockBatchManifest {
	if outputS3Uri == "" {
		return nil
	}

	// Parse the output S3 URI and construct manifest path
	bucketName, prefix := parseS3URI(outputS3Uri)
	if bucketName == "" {
		return nil
	}

	// Manifest is at: {output_s3_uri}/manifest.json.out
	base := strings.Trim(prefix, "/")
	manifestKey := "manifest.json.out"
	if base != "" {
		manifestKey = base + "/manifest.json.out"
	}

	// Build S3 GET request
	reqURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, escapeS3KeyForURL(manifestKey))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		provider.logger.Error("failed to create manifest request: %v", err)
		return nil
	}

	// Sign request for S3
	if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, "s3"); err != nil {
		provider.logger.Error("failed to sign manifest request: %v", err)
		return nil
	}

	resp, err := provider.client.Do(httpReq)
	if err != nil {
		provider.logger.Error("failed to fetch manifest: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// 404 is expected if job is still in progress
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		provider.logger.Debug("failed to read manifest body: %v", err)
		return nil
	}

	var manifest BedrockBatchManifest
	if err := sonic.Unmarshal(body, &manifest); err != nil {
		provider.logger.Error("failed to parse manifest: %v", err)
		return nil
	}

	return &manifest
}

// BatchRetrieve retrieves a specific batch inference job from AWS Bedrock by trying each key until found.
func (provider *BedrockProvider) BatchRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchRetrieveRequest) (*schemas.RakshaBatchRetrieveResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.BatchRetrieveRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewRakshaOperationError("batch_id (job ARN) is required", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		region := DefaultBedrockRegion
		if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
			region = key.BedrockKeyConfig.Region.GetValue()
		}

		// URL encode the job ARN
		encodedJobArn := url.PathEscape(request.BatchID)
		reqURL := fmt.Sprintf("https://bedrock.%s.amazonaws.com/model-invocation-job/%s", region, encodedJobArn)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error creating request", err)
			continue
		}

		// Sign request
		if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, bedrockSigningService); err != nil {
			lastErr = err
			continue
		}

		// Execute request
		startTime := time.Now()
		resp, err := provider.client.Do(httpReq)
		latency := time.Since(startTime)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, &schemas.RakshaError{
					IsRakshaError: false,
					Error: &schemas.ErrorField{
						Type:    schemas.Ptr(schemas.RequestCancelled),
						Message: schemas.ErrRequestCancelled,
						Error:   err,
					},
				}
			}
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error reading response", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = parseBedrockHTTPError(resp.StatusCode, resp.Header, body)
			continue
		}

		var bedrockResp BedrockBatchJobResponse
		if err := sonic.Unmarshal(body, &bedrockResp); err != nil {
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
			continue
		}

		// Store Bedrock-specific fields in Metadata for later conversion back to Bedrock format
		metadata := make(map[string]string)
		if bedrockResp.JobName != "" {
			metadata["job_name"] = bedrockResp.JobName
		}
		if bedrockResp.ModelID != "" {
			metadata["model_id"] = bedrockResp.ModelID
		}

		result := &schemas.RakshaBatchRetrieveResponse{
			ID:       bedrockResp.JobArn,
			Object:   "batch",
			Status:   ToRakshaBatchStatus(bedrockResp.Status),
			Metadata: metadata,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}

		// Surface the AWS job message (e.g. a validation failure reason) so
		// callers see why a job failed without dropping to the AWS CLI.
		if bedrockResp.Message != "" {
			result.Errors = &schemas.BatchErrors{
				Object: "list",
				Data:   []schemas.BatchError{{Message: bedrockResp.Message}},
			}
		}

		if bedrockResp.InputDataConfig != nil {
			result.InputFileID = bedrockResp.InputDataConfig.S3InputDataConfig.S3Uri
		}

		if bedrockResp.OutputDataConfig != nil {
			outputURI := bedrockResp.OutputDataConfig.S3OutputDataConfig.S3Uri
			result.OutputFileID = &outputURI
			// Fetch manifest to get record counts (only available after job starts processing)
			manifest := provider.fetchBatchManifest(ctx, key, region, outputURI)
			if manifest != nil {
				result.RequestCounts = schemas.BatchRequestCounts{
					Total:     manifest.TotalRecordCount,
					Completed: manifest.ProcessedRecordCount - manifest.ErrorRecordCount,
					Failed:    manifest.ErrorRecordCount,
				}
			}
		}

		// Capture VPC config in metadata if present
		if bedrockResp.VpcConfig != nil {
			if len(bedrockResp.VpcConfig.SecurityGroupIds) > 0 {
				metadata["vpc_security_group_ids"] = strings.Join(bedrockResp.VpcConfig.SecurityGroupIds, ",")
			}
			if len(bedrockResp.VpcConfig.SubnetIds) > 0 {
				metadata["vpc_subnet_ids"] = strings.Join(bedrockResp.VpcConfig.SubnetIds, ",")
			}
		}

		if bedrockResp.SubmitTime != nil {
			result.CreatedAt = bedrockResp.SubmitTime.Unix()
		}
		if bedrockResp.EndTime != nil {
			completedAt := bedrockResp.EndTime.Unix()
			result.CompletedAt = &completedAt
		}
		if bedrockResp.JobExpirationTime != nil {
			expiresAt := bedrockResp.JobExpirationTime.Unix()
			result.ExpiresAt = &expiresAt
		}

		return result, nil
	}

	return nil, lastErr
}

// BatchCancel stops a batch inference job on AWS Bedrock by trying each key until successful.
func (provider *BedrockProvider) BatchCancel(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchCancelRequest) (*schemas.RakshaBatchCancelResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.BatchCancelRequest); err != nil {
		return nil, err
	}

	if request.BatchID == "" {
		return nil, providerUtils.NewRakshaOperationError("batch_id (job ARN) is required", nil)
	}

	var lastErr *schemas.RakshaError
	for _, key := range keys {
		region := DefaultBedrockRegion
		if key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
			region = key.BedrockKeyConfig.Region.GetValue()
		}

		// URL encode the job ARN
		encodedJobArn := url.PathEscape(request.BatchID)
		reqURL := fmt.Sprintf("https://bedrock.%s.amazonaws.com/model-invocation-job/%s/stop", region, encodedJobArn)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error creating request", err)
			continue
		}

		// Sign request
		if err := signAWSRequest(ctx, httpReq, key.BedrockKeyConfig, region, bedrockSigningService); err != nil {
			lastErr = err
			continue
		}

		// Execute request
		startTime := time.Now()
		resp, err := provider.client.Do(httpReq)
		latency := time.Since(startTime)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, &schemas.RakshaError{
					IsRakshaError: false,
					Error: &schemas.ErrorField{
						Type:    schemas.Ptr(schemas.RequestCancelled),
						Message: schemas.ErrRequestCancelled,
						Error:   err,
					},
				}
			}
			lastErr = providerUtils.NewRakshaOperationError(schemas.ErrProviderDoRequest, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = providerUtils.NewRakshaOperationError("error reading response", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = parseBedrockHTTPError(resp.StatusCode, resp.Header, body)
			continue
		}

		// After stopping, retrieve the job to get updated status
		retrieveResp, rakshaErr := provider.BatchRetrieve(ctx, keys, &schemas.RakshaBatchRetrieveRequest{
			Provider: request.Provider,
			BatchID:  request.BatchID,
		})
		if rakshaErr != nil {
			// Return basic response if retrieve fails
			// Compute total latency including stop + failed retrieve
			totalLatency := time.Since(startTime)
			return &schemas.RakshaBatchCancelResponse{
				ID:     request.BatchID,
				Object: "batch",
				Status: schemas.BatchStatusCancelling,
				ExtraFields: schemas.RakshaResponseExtraFields{
					Latency: totalLatency.Milliseconds(),
				},
			}, nil
		}

		return &schemas.RakshaBatchCancelResponse{
			ID:     retrieveResp.ID,
			Object: "batch",
			Status: retrieveResp.Status,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: latency.Milliseconds(),
			},
		}, nil
	}

	return nil, lastErr
}

// BatchDelete is not supported by the Bedrock provider.
func (provider *BedrockProvider) BatchDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchDeleteRequest) (*schemas.RakshaBatchDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.BatchDeleteRequest, provider.GetProviderKey())
}

// BatchResults retrieves batch results from AWS Bedrock by trying each key until successful.
// For Bedrock, results are stored in S3 at the output S3 URI prefix.
// The output includes JSONL files with results (*.jsonl.out) and a manifest file.
func (provider *BedrockProvider) BatchResults(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaBatchResultsRequest) (*schemas.RakshaBatchResultsResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.BatchResultsRequest); err != nil {
		return nil, err
	}

	// First, retrieve the batch to get the output S3 URI prefix (using all keys)
	batchResp, rakshaErr := provider.BatchRetrieve(ctx, keys, &schemas.RakshaBatchRetrieveRequest{
		Provider: request.Provider,
		BatchID:  request.BatchID,
	})
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	if batchResp.OutputFileID == nil || *batchResp.OutputFileID == "" {
		return nil, providerUtils.NewRakshaOperationError("batch results not available: output S3 URI is empty (batch may not be completed)", nil)
	}

	outputS3URI := *batchResp.OutputFileID
	var allResults []schemas.BatchResultItem
	var totalLatency int64
	// The output S3 URI is a prefix/folder. List files in that folder to find output JSONL files.
	var (
		listResp  *schemas.RakshaFileListResponse
		pageToken *string
		allFiles  []schemas.FileObject
	)
	for {
		listResp, rakshaErr = provider.FileList(ctx, keys, &schemas.RakshaFileListRequest{
			Provider: request.Provider,
			StorageConfig: &schemas.FileStorageConfig{
				S3: &schemas.S3StorageConfig{
					Bucket: outputS3URI,
				},
			},
			Limit: 100,
			After: pageToken,
		})
		if rakshaErr != nil {
			break
		}
		totalLatency += listResp.ExtraFields.Latency
		allFiles = append(allFiles, listResp.Data...)
		if !listResp.HasMore || listResp.After == nil {
			break
		}
		pageToken = listResp.After
	}
	if rakshaErr != nil {
		// If listing fails, try direct download (in case outputS3URI is already a file path)
		fileContentResp, directErr := provider.FileContent(ctx, keys, &schemas.RakshaFileContentRequest{
			Provider: request.Provider,
			FileID:   outputS3URI,
		})
		if directErr != nil {
			return nil, providerUtils.NewRakshaOperationError(
				fmt.Sprintf("failed to access batch results at %s: listing failed and direct access failed", outputS3URI),
				nil)
		}

		// Direct download succeeded, parse the content
		results, parseErrors := parseBatchResultsJSONL(fileContentResp.Content, provider)
		batchResultsResp := &schemas.RakshaBatchResultsResponse{
			BatchID: request.BatchID,
			Results: results,
			ExtraFields: schemas.RakshaResponseExtraFields{
				Latency: fileContentResp.ExtraFields.Latency,
			},
		}
		if len(parseErrors) > 0 {
			batchResultsResp.ExtraFields.ParseErrors = parseErrors
		}
		return batchResultsResp, nil
	}
	// Find and download JSONL output files (files ending with .jsonl.out or containing results)
	var allParseErrors []schemas.BatchError
	for _, file := range allFiles {
		// Skip manifest files, only process JSONL output files
		if strings.HasSuffix(file.ID, ".jsonl.out") || strings.HasSuffix(file.ID, ".jsonl") {
			fileContentResp, fileErr := provider.FileContent(ctx, keys, &schemas.RakshaFileContentRequest{
				Provider: request.Provider,
				FileID:   file.ID,
			})
			if fileErr != nil {
				provider.logger.Warn("failed to download batch result file %s: %v", file.ID, fileErr)
				continue
			}

			totalLatency += fileContentResp.ExtraFields.Latency
			results, parseErrors := parseBatchResultsJSONL(fileContentResp.Content, provider)
			allResults = append(allResults, results...)
			allParseErrors = append(allParseErrors, parseErrors...)
		}
	}

	batchResultsResp := &schemas.RakshaBatchResultsResponse{
		BatchID: request.BatchID,
		Results: allResults,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency: totalLatency,
		},
	}

	if len(allParseErrors) > 0 {
		batchResultsResp.ExtraFields.ParseErrors = allParseErrors
	}

	return batchResultsResp, nil
}

// getModelPathAndRegion is a helper that calls parseBedrockRegionAndModel
// once and returns both the request path and the AWS signing region.
// Honors per-alias Region and BedrockAliasCfg.InferenceProfileARN overrides
// via the resolved alias in ctx.
func (provider *BedrockProvider) getModelPathAndRegion(ctx *schemas.RakshaContext, basePath, model string, key schemas.Key) (path, region string) {
	r, bareModel := parseBedrockRegionAndModel(model)
	if r == "" {
		if ra := schemas.GetResolvedAlias(ctx); ra != nil && ra.Config != nil && ra.Config.Region != nil {
			if v := ra.Config.Region.GetValue(); v != "" {
				r = v
			}
		}
		if r == "" {
			if key.BedrockKeyConfig != nil && key.BedrockKeyConfig.Region != nil && key.BedrockKeyConfig.Region.GetValue() != "" {
				r = key.BedrockKeyConfig.Region.GetValue()
			} else {
				r = DefaultBedrockRegion
			}
		}
	}
	p := fmt.Sprintf("%s/%s", bareModel, basePath)
	if arn := resolveBedrockARN(ctx, key); arn != "" {
		encodedModelIdentifier := url.PathEscape(fmt.Sprintf("%s/%s", arn, bareModel))
		p = fmt.Sprintf("%s/%s", encodedModelIdentifier, basePath)
	}
	return p, r
}

func (provider *BedrockProvider) CountTokens(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaResponsesRequest) (*schemas.RakshaCountTokensResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Bedrock, provider.customProviderConfig, schemas.CountTokensRequest); err != nil {
		return nil, err
	}

	// Convert to Bedrock Converse format using the existing responses converter
	converseReq, convErr := ToBedrockResponsesRequest(ctx, request)
	if convErr != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, convErr)
	}

	// Wrap in the CountTokens request envelope
	countTokensReq := &BedrockCountTokensRequest{}
	countTokensReq.Input.Converse = converseReq

	jsonData, err := providerUtils.MarshalSorted(countTokensReq)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrProviderRequestMarshal, err)
	}

	// Format the path with proper model identifier
	path, _ := provider.getModelPathAndRegion(ctx, "count-tokens", request.Model, key)

	// Send the request
	responseBody, latency, providerResponseHeaders, rakshaErr := provider.completeRequest(ctx, jsonData, path, key, request.Model)
	if providerResponseHeaders != nil {
		ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)
	}
	if rakshaErr != nil {
		if isCountTokensUnsupported(rakshaErr) {
			estimated := estimateTokenCount(jsonData)
			return &schemas.RakshaCountTokensResponse{
				Model:       request.Model,
				InputTokens: estimated,
				TotalTokens: &estimated,
				Object:      "response.input_tokens",
				ExtraFields: schemas.RakshaResponseExtraFields{
					Latency:                 latency.Milliseconds(),
					ProviderResponseHeaders: providerResponseHeaders,
				},
			}, nil
		}
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Parse the response
	bedrockResponse := &BedrockCountTokensResponse{}
	rawRequest, rawResponse, rakshaErr := providerUtils.HandleProviderResponse(
		responseBody,
		bedrockResponse,
		jsonData,
		providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest),
		providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse),
	)
	if rakshaErr != nil {
		return nil, providerUtils.EnrichError(ctx, rakshaErr, jsonData, responseBody, provider.sendBackRawRequest, provider.sendBackRawResponse, latency)
	}

	// Convert to Raksha format
	response := bedrockResponse.ToRakshaCountTokensResponse(request.Model)

	response.ExtraFields.Latency = latency.Milliseconds()
	response.ExtraFields.ProviderResponseHeaders = providerResponseHeaders
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		response.ExtraFields.RawRequest = rawRequest
	}

	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		response.ExtraFields.RawResponse = rawResponse
	}

	return response, nil
}

// Compaction is not supported by the Bedrock provider.
func (provider *BedrockProvider) Compaction(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCompactionRequest) (*schemas.RakshaCompactionResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CompactionRequest, provider.GetProviderKey())
}

// ContainerCreate is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerCreateRequest) (*schemas.RakshaContainerCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerCreateRequest, provider.GetProviderKey())
}

// ContainerList is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerListRequest) (*schemas.RakshaContainerListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerListRequest, provider.GetProviderKey())
}

// ContainerRetrieve is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerRetrieveRequest) (*schemas.RakshaContainerRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerRetrieveRequest, provider.GetProviderKey())
}

// ContainerDelete is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerDeleteRequest) (*schemas.RakshaContainerDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerDeleteRequest, provider.GetProviderKey())
}

// ContainerFileCreate is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerFileCreate(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaContainerFileCreateRequest) (*schemas.RakshaContainerFileCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileCreateRequest, provider.GetProviderKey())
}

// ContainerFileList is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerFileList(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileListRequest) (*schemas.RakshaContainerFileListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileListRequest, provider.GetProviderKey())
}

// ContainerFileRetrieve is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerFileRetrieve(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileRetrieveRequest) (*schemas.RakshaContainerFileRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileRetrieveRequest, provider.GetProviderKey())
}

// ContainerFileContent is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerFileContent(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileContentRequest) (*schemas.RakshaContainerFileContentResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileContentRequest, provider.GetProviderKey())
}

// ContainerFileDelete is not supported by the Bedrock provider.
func (provider *BedrockProvider) ContainerFileDelete(_ *schemas.RakshaContext, _ []schemas.Key, _ *schemas.RakshaContainerFileDeleteRequest) (*schemas.RakshaContainerFileDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.ContainerFileDeleteRequest, provider.GetProviderKey())
}

// Passthrough is not supported by the Bedrock provider.
func (provider *BedrockProvider) Passthrough(_ *schemas.RakshaContext, _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughRequest, provider.GetProviderKey())
}

func (provider *BedrockProvider) PassthroughStream(_ *schemas.RakshaContext, _ schemas.PostHookRunner, _ func(context.Context), _ schemas.Key, _ *schemas.RakshaPassthroughRequest) (chan *schemas.RakshaStreamChunk, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.PassthroughStreamRequest, provider.GetProviderKey())
}
