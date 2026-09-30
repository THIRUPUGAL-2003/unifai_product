package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

// Guard code bundle: the Guard's Python code (agent/*.py + the mitmproxy addon
// browser_ai_proxy.py + unifai_proxy_parts) is published by Rebuild and installed
// Guards switch to it without a new installer (guard_bootstrap loads it). Only new
// Python packages, runtime upgrades or installer changes need a new EXE/.app build.

const (
	guardProxyEntry    = "browser_ai_proxy.py"
	guardProxyPartsDir = "unifai_proxy_parts"
	guardAgentDir      = "agent"
	// Frozen into the EXE as the entry point; never hot-updated.
	guardBootstrapFile = "guard_bootstrap.py"
)

var guardModuleNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.py$`)

type guardProxyBundle struct {
	SHA256        string    `json:"sha256"`
	PublishedAt   time.Time `json:"published_at"`
	Files         int       `json:"files"`
	Size          int       `json:"size"`
	GuardVersions []string  `json:"guard_versions"`
}

var guardProxyBundleMu sync.Mutex

func guardProxySourceDir() (string, bool) {
	for _, dir := range []string{
		filepath.Join("apps", "browser-guard", "proxy"),
		"/app/guard-proxy",
		"/app/proxy",
	} {
		if info, err := os.Stat(filepath.Join(dir, guardProxyEntry)); err != nil || info.IsDir() {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, guardProxyPartsDir)); err == nil && info.IsDir() {
			return dir, true
		}
	}
	return "", false
}

func guardAgentSourceDir() (string, bool) {
	for _, dir := range []string{
		filepath.Join("apps", "browser-guard", "agent"),
		"/app/guard-agent",
	} {
		if info, err := os.Stat(filepath.Join(dir, "unifai_agent.py")); err == nil && !info.IsDir() {
			return dir, true
		}
	}
	return "", false
}

func guardProxyBundleStoreDir() string {
	base := strings.TrimSpace(os.Getenv("APP_DIR"))
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "guard_proxy_bundle")
}

// buildGuardProxyBundle zips the Guard sources deterministically (sorted names, fixed
// timestamps) so the SHA-256 only changes when the code changes. agentDir may be ""
// (proxy code only).
func buildGuardProxyBundle(proxyDir, agentDir string) ([]byte, int, error) {
	sources := map[string]string{guardProxyEntry: filepath.Join(proxyDir, guardProxyEntry)}
	entries, err := os.ReadDir(filepath.Join(proxyDir, guardProxyPartsDir))
	if err != nil {
		return nil, 0, err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !(strings.HasSuffix(n, ".py") || n == "MANIFEST.txt") {
			continue
		}
		sources[guardProxyPartsDir+"/"+n] = filepath.Join(proxyDir, guardProxyPartsDir, n)
	}
	if agentDir != "" {
		entries, err := os.ReadDir(agentDir)
		if err != nil {
			return nil, 0, err
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || n == guardBootstrapFile || !guardModuleNameRe.MatchString(n) {
				continue
			}
			sources[guardAgentDir+"/"+n] = filepath.Join(agentDir, n)
		}
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fixed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range names {
		data, err := os.ReadFile(sources[name])
		if err != nil {
			return nil, 0, err
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: fixed})
		if err != nil {
			return nil, 0, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, 0, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), len(names), nil
}

func guardBundleVersions() []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range []string{readGuardReleaseVersion(), readGuardMacReleaseVersion()} {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// publishGuardProxyBundle snapshots the server's proxy sources for Guard hot-update.
func publishGuardProxyBundle() (*guardProxyBundle, error) {
	src, ok := guardProxySourceDir()
	if !ok {
		return nil, fmt.Errorf("Guard proxy sources not found on server (apps/browser-guard/proxy or /app/guard-proxy)")
	}
	agentSrc, agentOK := guardAgentSourceDir()
	if !agentOK {
		return nil, fmt.Errorf("Guard agent sources not found on server (apps/browser-guard/agent or /app/guard-agent)")
	}
	data, files, err := buildGuardProxyBundle(src, agentSrc)
	if err != nil {
		return nil, fmt.Errorf("build Guard proxy bundle: %w", err)
	}
	sum := sha256.Sum256(data)
	meta := &guardProxyBundle{
		SHA256:        hex.EncodeToString(sum[:]),
		PublishedAt:   time.Now().UTC(),
		Files:         files,
		Size:          len(data),
		GuardVersions: guardBundleVersions(),
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}

	guardProxyBundleMu.Lock()
	defer guardProxyBundleMu.Unlock()
	dir := guardProxyBundleStoreDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create bundle dir: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(dir, "bundle.zip"), data); err != nil {
		return nil, fmt.Errorf("write bundle: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(dir, "bundle.json"), metaJSON); err != nil {
		return nil, fmt.Errorf("write bundle metadata: %w", err)
	}
	return meta, nil
}

func loadPublishedGuardProxyBundle() *guardProxyBundle {
	guardProxyBundleMu.Lock()
	defer guardProxyBundleMu.Unlock()
	raw, err := os.ReadFile(filepath.Join(guardProxyBundleStoreDir(), "bundle.json"))
	if err != nil {
		return nil
	}
	var meta guardProxyBundle
	if json.Unmarshal(raw, &meta) != nil || len(meta.SHA256) != 64 {
		return nil
	}
	return &meta
}

// heartbeatProxyBundle is the compact form sent to Guards on every heartbeat.
func heartbeatProxyBundle() map[string]any {
	meta := loadPublishedGuardProxyBundle()
	if meta == nil {
		return nil
	}
	return map[string]any{
		"sha256":         meta.SHA256,
		"published_at":   meta.PublishedAt,
		"guard_versions": meta.GuardVersions,
	}
}

func (h *BrowserAIHandler) getProxyBundleInfo(ctx *fasthttp.RequestCtx) {
	meta := loadPublishedGuardProxyBundle()
	if meta == nil {
		SendJSON(ctx, map[string]any{"published": false})
		return
	}
	SendJSON(ctx, map[string]any{"published": true, "bundle": meta})
}

func (h *BrowserAIHandler) downloadProxyBundle(ctx *fasthttp.RequestCtx) {
	meta := loadPublishedGuardProxyBundle()
	if meta == nil {
		SendError(ctx, fasthttp.StatusNotFound, "No Guard code bundle published — press Rebuild & Publish in Browser AI → Setup")
		return
	}
	guardProxyBundleMu.Lock()
	data, err := os.ReadFile(filepath.Join(guardProxyBundleStoreDir(), "bundle.zip"))
	guardProxyBundleMu.Unlock()
	if err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "Guard proxy bundle file missing — press Rebuild & Publish again")
		return
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != meta.SHA256 {
		SendError(ctx, fasthttp.StatusConflict, "Guard proxy bundle changed while reading — retry")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/zip")
	ctx.Response.Header.Set("X-UnifAI-Bundle-SHA256", meta.SHA256)
	ctx.Response.Header.Set("Cache-Control", "no-store")
	ctx.SetBody(data)
}
