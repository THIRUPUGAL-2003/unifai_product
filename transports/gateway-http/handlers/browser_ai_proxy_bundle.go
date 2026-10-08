package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

// Guard code bundle: gateway_guard_code.enc and gateway_proxy_parts.enc
// are published by Rebuild. Plain proxy, agent, and loader sources are never
// included. Installed Guards switch to it without a new installer.

const (
	guardProxyEntry    = "browser_ai_proxy.py"
	guardProxyPartsDir = "gateway_proxy_parts"
	guardCodeEnc       = "gateway_guard_code.enc"
)

type guardProxyBundle struct {
	SHA256        string    `json:"sha256"`
	PublishedAt   time.Time `json:"published_at"`
	Files         int       `json:"files"`
	Size          int       `json:"size"`
	GuardVersions []string  `json:"guard_versions"`
}

var guardProxyBundleMu sync.Mutex

// ensureProxyBundleEncrypted refreshes gateway_proxy_parts.enc from the proxy
// sources when Python is available. Publish never falls back to plaintext parts.
func ensureProxyBundleEncrypted(proxyDir string) error {
	script := filepath.Join("apps", "browser-guard", "installer", "encrypt_proxy_bundle.py")
	if _, err := os.Stat(script); err != nil {
		script = filepath.Join(filepath.Dir(proxyDir), "installer", "encrypt_proxy_bundle.py")
	}
	if _, err := os.Stat(script); err != nil {
		enc := filepath.Join(proxyDir, guardProxyPartsDir, "gateway_proxy_parts.enc")
		if info, statErr := os.Stat(enc); statErr == nil && !info.IsDir() {
			return nil
		}
		return fmt.Errorf("encrypt_proxy_bundle.py not found and no encrypted bundle exists")
	}
	pythonBin, err := exec.LookPath("python")
	if err != nil {
		pythonBin, err = exec.LookPath("python3")
	}
	if err != nil {
		enc := filepath.Join(proxyDir, guardProxyPartsDir, "gateway_proxy_parts.enc")
		if info, statErr := os.Stat(enc); statErr == nil && !info.IsDir() {
			return nil
		}
		return fmt.Errorf("python is required to encrypt the proxy bundle: %w", err)
	}
	cmd := exec.Command(pythonBin, script)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("encrypt proxy bundle: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

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
		if info, err := os.Stat(filepath.Join(dir, "gateway_agent.py")); err == nil && !info.IsDir() {
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
	codeEnc := filepath.Join(proxyDir, guardCodeEnc)
	if info, err := os.Stat(codeEnc); err != nil || info.IsDir() {
		return nil, 0, fmt.Errorf("encrypted guard code missing at %s; refusing to publish plaintext sources", codeEnc)
	}
	sources := map[string]string{guardCodeEnc: codeEnc}
	partsDir := filepath.Join(proxyDir, guardProxyPartsDir)
	encPath := filepath.Join(partsDir, "gateway_proxy_parts.enc")
	if info, err := os.Stat(encPath); err != nil || info.IsDir() {
		return nil, 0, fmt.Errorf("encrypted proxy bundle missing at %s; refusing to publish plaintext proxy sources", encPath)
	}
	sources[guardProxyPartsDir+"/gateway_proxy_parts.enc"] = encPath
	manifestPath := filepath.Join(partsDir, "MANIFEST.txt")
	if info, err := os.Stat(manifestPath); err == nil && !info.IsDir() {
		sources[guardProxyPartsDir+"/MANIFEST.txt"] = manifestPath
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
	if err := ensureProxyBundleEncrypted(src); err != nil {
		return nil, err
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
	ctx.Response.Header.Set("X-Gateway-Bundle-SHA256", meta.SHA256)
	ctx.Response.Header.Set("Cache-Control", "no-store")
	ctx.SetBody(data)
}
