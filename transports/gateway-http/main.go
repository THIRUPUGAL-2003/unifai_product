// Package main provides an HTTP service using FastHTTP that exposes endpoints
// for text and chat completions using various AI model providers (OpenAI, Anthropic, Bedrock, Mistral, Ollama, etc.).
//
// The HTTP service provides the following main endpoints:
//   - /v1/completions: For text completion requests
//   - /v1/chat/completions: For chat completion requests
//   - /v1/mcp/tool/execute: For MCP tool execution requests
//   - /providers/*: For provider configuration management
//
// Configuration is handled through a JSON config file, high-performance ConfigStore, and environment variables:
//   - Use -app-dir flag to specify the application data directory (contains config.json and logs)
//   - Use -port flag to specify the server port (default: 8001 / APP_PORT)
//   - When no config file exists, common environment variables are auto-detected (OPENAI_API_KEY, ANTHROPIC_API_KEY, MISTRAL_API_KEY)
//
// ConfigStore Features:
//   - Pure in-memory storage for ultra-fast config access
//   - Environment variable processing for secure configuration management
//   - Real-time configuration updates via HTTP API
//   - Explicit persistence control via POST /config/save endpoint
//   - Provider-specific key config support (Azure, Bedrock, Vertex)
//   - Thread-safe operations with concurrent request handling
//   - Statistics and monitoring endpoints for operational insights
//
// Performance Optimizations:
//   - Configuration data is processed once during startup and stored in memory
//   - Ultra-fast memory access eliminates I/O overhead on every request
//   - All environment variable processing done upfront during configuration loading
//   - Thread-safe concurrent access with read-write mutex protection
//
// Example usage:
//
//	go run main.go -app-dir ./data -port 8001 -host 0.0.0.0
//	after setting provider API keys like OPENAI_API_KEY in the environment.
//
//	To bind to all interfaces for container usage, set GATEWAY_HOST=0.0.0.0 or use -host 0.0.0.0
//
// Integration Support:
// Gateway supports multiple AI provider integrations through dedicated HTTP endpoints.
// Each integration exposes API-compatible endpoints that accept the provider's native request format,
// automatically convert it to Gateway's unified format, process it, and return the expected response format.
//
// Integration endpoints follow the pattern: /{provider}/{provider_api_path}
// Examples:
//   - OpenAI: POST /openai/v1/chat/completions (accepts OpenAI ChatCompletion requests)
//   - GenAI:  POST /genai/v1beta/models/{model} (accepts Google GenAI requests)
//   - Anthropic: POST /anthropic/v1/messages (accepts Anthropic Messages requests)
//
// This allows clients to use their existing integration code without modification while benefiting
// from Gateway's unified model routing, fallbacks, monitoring capabilities, and high-performance configuration management.
//
// NOTE: Streaming is supported for chat completions via Server-Sent Events (SSE)
package main

import (
	"bufio"
	"context"
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "go.uber.org/automaxprocs" // Automatically set GOMAXPROCS based on container cgroup limits

	gateway "github.com/gateway/gateway/core"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/transports/gateway-http/handlers"
	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/gateway/gateway/transports/gateway-http/profiling"
	gatewayServer "github.com/gateway/gateway/transports/gateway-http/server"
)

//go:embed all:ui
var uiContent embed.FS

var Version string

var logger = gateway.NewDefaultLogger(schemas.LogLevelInfo)
var server *gatewayServer.GatewayHTTPServer

// loadDotEnv searches for .env in current and parent directories and loads any unset env vars.
func loadDotEnv() {
	candidates := []string{
		".env",
		filepath.Join("..", ".env"),
		filepath.Join("..", "..", ".env"),
		filepath.Join("..", "..", "..", ".env"),
	}
	for _, path := range candidates {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()

		absPath, _ := filepath.Abs(path)
		fmt.Printf("Loaded environment from: %s\n", absPath)

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
		break
	}
}

// init initializes command line flags (but does not parse them).
// Flag parsing is deferred to main() to avoid conflicts with test flags.
// It sets up the following flags:
//   - host: Host to bind the server to (default: localhost, can be overridden with GATEWAY_HOST env var)
//   - port: Server port (default: 8001 / APP_PORT from .env)
//   - app-dir: Application data directory (default: current directory)
//   - log-level: Logger level (debug, info, warn, error). Default is info.
//   - log-style: Logger output type (json or pretty). Default is JSON.

func init() {
	// Discover and load .env automatically
	loadDotEnv()

	// Automatically locate configs directory if not specified
	defaultAppDir := gatewayServer.DefaultAppDir
	for _, candidate := range []string{"configs", filepath.Join("..", "configs"), filepath.Join("..", "..", "configs")} {
		if _, err := os.Stat(filepath.Join(candidate, "config.json")); err == nil {
			defaultAppDir = candidate
			break
		}
	}

	// Version is injected at release build time via -ldflags. Local/dev images
	// leave it empty so the UI does not show a placeholder like "vunknown".
	// Set default host from environment variable (GATEWAY_HOST or APP_HOST) or use localhost
	defaultHost := GatewayEnv("HOST")
	if defaultHost == "" {
		defaultHost = os.Getenv("APP_HOST")
	}
	if defaultHost == "" {
		defaultHost = gatewayServer.DefaultHost
	}
	defaultPort := gatewayServer.DefaultPort
	if envPort := os.Getenv("APP_PORT"); envPort != "" {
		defaultPort = envPort
	}
	defaultLogLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if defaultLogLevel == "" {
		defaultLogLevel = gatewayServer.DefaultLogLevel
	}
	defaultLogStyle := strings.ToLower(os.Getenv("LOG_STYLE"))
	if defaultLogStyle == "" {
		defaultLogStyle = gatewayServer.DefaultLogOutputStyle
	}
	// Initializing server
	server = gatewayServer.NewGatewayHTTPServer(Version, uiContent)
	// Updating server properties from flags
	flag.StringVar(&server.Port, "port", defaultPort, "Port to run the server on")
	flag.StringVar(&server.Host, "host", defaultHost, "Host to bind the server to (default: localhost, override with GATEWAY_HOST or APP_HOST env var)")
	flag.StringVar(&server.AppDir, "app-dir", defaultAppDir, "Application data directory (contains config.json and logs)")
	flag.StringVar(&server.LogLevel, "log-level", defaultLogLevel, "Logger level (debug, info, warn, error). Default is info.")
	flag.StringVar(&server.LogOutputStyle, "log-style", defaultLogStyle, "Logger output type (json or pretty). Default is JSON.")
}

// main is the entry point of the application.
func main() {
	// Parse command line flags
	flag.Parse()

	versionLine := ""
	if Version != "" {
		versionLine = fmt.Sprintf("║═══════════════════════════════════════════════════════════║\n║%s%s%s║\n", strings.Repeat(" ", (61-2-len(Version))/2), Version, strings.Repeat(" ", (61-2-len(Version)+1)/2))
	}
	// Welcome to gateway!
	fmt.Printf(`
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║     ██████╗  █████╗ ██╗  ██╗███████╗██╗  ██╗ █████╗       ║
║     ██╔══██╗██╔══██╗██║ ██╔╝██╔════╝██║  ██║██╔══██╗      ║
║     ██████╔╝███████║█████╔╝ ███████╗███████║███████║      ║
║     ██╔══██╗██╔══██║██╔═██╗ ╚════██║██╔══██║██╔══██║      ║
║     ██║  ██║██║  ██║██║  ██╗███████║██║  ██║██║  ██║      ║
║     ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝      ║
║                                                           ║
%s║═══════════════════════════════════════════════════════════║
║                 The Fastest LLM Gateway                   ║
║═══════════════════════════════════════════════════════════║
║             https://github.com/gateway/gateway              ║
╚═══════════════════════════════════════════════════════════╝

`, versionLine)

	// Start profiling
	pprofServer := profiling.Start()

	// Configure logger from flags
	logger.SetOutputType(schemas.LoggerOutputType(server.LogOutputStyle))
	logger.SetLevel(schemas.LogLevel(server.LogLevel))
	// Setting up logger
	lib.SetLogger(logger)
	gatewayServer.SetLogger(logger)
	handlers.SetLogger(logger)

	ctx := context.Background()
	t := time.Now()
	err := server.Bootstrap(ctx)
	if err != nil {
		logger.Error("failed to bootstrap server: %v", err)
		os.Exit(1)
	}
	logger.Info("Time spent in Gateway server bootstrap %d ms", time.Since(t).Milliseconds())
	err = server.Start()
	if err != nil {
		logger.Error("failed to start server: %v", err)
		os.Exit(1)
	}
	// server.Start() blocks until SIGINT/SIGTERM triggers graceful shutdown, so
	// by here the main server is draining/done. Shut the pprof server down too
	// to let any in-flight profile requests finish instead of being killed.
	if pprofServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := pprofServer.Shutdown(shutdownCtx); err != nil {
			logger.Warn("pprof server shutdown error: %v", err)
		}
	}
	logger.Info("🏁 server stopped")
}
