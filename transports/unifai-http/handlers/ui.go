package handlers

import (
	"embed"
	"mime"
	"path"
	"path/filepath"
	"strings"

	"github.com/fasthttp/router"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

// UIHandler handles UI routes.
type UIHandler struct {
	uiContent embed.FS
}

// NewUIHandler creates a new UIHandler instance.
func NewUIHandler(uiContent embed.FS) *UIHandler {
	return &UIHandler{
		uiContent: uiContent,
	}
}

// RegisterRoutes registers the UI routes with the provided router.
func (h *UIHandler) RegisterRoutes(router *router.Router, middlewares ...schemas.UnifAIHTTPMiddleware) {
	router.GET("/", lib.ChainMiddlewares(h.serveDashboard, middlewares...))
	router.GET("/{filepath:*}", lib.ChainMiddlewares(h.serveDashboard, middlewares...))
}

var staticAssetExtensions = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".map": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".ico": true, ".webp": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".json": true, ".txt": true, ".xml": true, ".pdf": true, ".wasm": true,
}

func isStaticAsset(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return staticAssetExtensions[ext]
}

// ServeDashboard serves the dashboard UI.
func (h *UIHandler) serveDashboard(ctx *fasthttp.RequestCtx) {
	// Get the request path
	requestPath := string(ctx.Path())

	// Clean the path to prevent directory traversal
	cleanPath := path.Clean(requestPath)

	// Handle .txt files - map from /{page}.txt to /{page}/index.txt
	if strings.HasSuffix(cleanPath, ".txt") {
		// Remove .txt extension and add /index.txt
		basePath := strings.TrimSuffix(cleanPath, ".txt")
		if basePath == "/" || basePath == "" {
			basePath = "/index"
		}
		cleanPath = basePath + "/index.txt"
	}

	// Remove leading slash and add ui prefix
	if cleanPath == "/" {
		cleanPath = "ui/index.html"
	} else {
		cleanPath = "ui" + cleanPath
	}

	// Block hidden directories and files (any path segment starting with .)
	segments := strings.Split(cleanPath, "/")
	for _, segment := range segments {
		if strings.HasPrefix(segment, ".") {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
			ctx.SetBodyString("404 - Not found")
			return
		}
	}

	// Block sensitive files
	baseName := filepath.Base(cleanPath)
	sensitiveFiles := []string{"package.json", "package-lock.json"}
	for _, sensitive := range sensitiveFiles {
		if baseName == sensitive {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
			ctx.SetBodyString("404 - Not found")
			return
		}
	}

	// Check if this is a static asset request (known static extension or under assets/)
	isAsset := isStaticAsset(cleanPath) || strings.HasPrefix(cleanPath, "ui/assets/")

	// Try to read the file from embedded filesystem
	data, err := h.uiContent.ReadFile(cleanPath)
	if err != nil {
		// If it's an actual static asset and not found, return 404
		if isAsset {
			ctx.SetStatusCode(fasthttp.StatusNotFound)
			ctx.SetBodyString("404 - Static asset not found: " + requestPath)
			return
		}

		// For SPA routes, try {path}/index.html first
		indexPath := cleanPath + "/index.html"
		data, err = h.uiContent.ReadFile(indexPath)
		if err == nil {
			cleanPath = indexPath
		} else {
			// If that fails, serve root index.html as fallback
			data, err = h.uiContent.ReadFile("ui/index.html")
			if err != nil {
				ctx.SetStatusCode(fasthttp.StatusNotFound)
				ctx.SetBodyString("404 - File not found")
				return
			}
			cleanPath = "ui/index.html"
		}
	}

	// Set content type based on file extension
	ext := filepath.Ext(cleanPath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ctx.SetContentType(contentType)

	// Set cache headers for static assets
	if strings.HasPrefix(cleanPath, "ui/assets/") {
		ctx.Response.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if ext == ".html" {
		ctx.Response.Header.Set("Cache-Control", "no-cache")
	} else {
		ctx.Response.Header.Set("Cache-Control", "public, max-age=3600")
	}

	// Send the file content
	ctx.SetBody(data)
}
