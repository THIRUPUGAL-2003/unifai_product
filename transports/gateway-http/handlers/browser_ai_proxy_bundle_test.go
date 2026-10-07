package handlers

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"
)

func writeFakeProxySource(t *testing.T, root string) {
	t.Helper()
	parts := filepath.Join(root, guardProxyPartsDir)
	if err := os.MkdirAll(filepath.Join(parts, "__pycache__"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, guardProxyEntry):             "print('loader')\n",
		filepath.Join(parts, "MANIFEST.txt"):             "a.py\nb.py\n",
		filepath.Join(parts, "gateway_proxy_parts.enc"):  "GATEWAYENC02\nfake\n",
		filepath.Join(parts, "a.py"):                     "A = 1\n",
		filepath.Join(parts, "b.py"):                     "B = 2\n",
		filepath.Join(parts, "bundle_crypto.py"):         "SEED = 'do not ship'\n",
		filepath.Join(parts, "notes.md"):                 "skip me",
		filepath.Join(parts, "__pycache__", "a.cpython"): "skip me",
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFakeAgentSource(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"gateway_agent.py":         "def main(): pass\n",
		"agent_config.py":         "AGENT_VERSION_BAKED = '1.1.14'\n",
		"guard_bootstrap.py":      "# frozen entry, never shipped\n",
		"agent-notes.py":          "not a module name",
		"README.txt":              "skip",
		"tests/test_something.py": "skip",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGuardCodeBundleIsDeterministicAndShipsAgentAndProxyCode(t *testing.T) {
	src := t.TempDir()
	writeFakeProxySource(t, src)
	agent := t.TempDir()
	writeFakeAgentSource(t, agent)

	first, n, err := buildGuardProxyBundle(src, agent)
	if err != nil {
		t.Fatal(err)
	}
	second, _, _ := buildGuardProxyBundle(src, agent)
	if !bytes.Equal(first, second) {
		t.Fatal("same sources must produce an identical bundle (stable SHA-256)")
	}
	if n != 5 {
		t.Fatalf("expected 5 files (loader + MANIFEST + encrypted bundle + 2 agent modules), got %d", n)
	}
	zr, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	for _, want := range []string{
		"browser_ai_proxy.py", "gateway_proxy_parts/MANIFEST.txt", "gateway_proxy_parts/gateway_proxy_parts.enc",
		"agent/gateway_agent.py", "agent/agent_config.py",
	} {
		if !got[want] {
			t.Errorf("bundle missing %s", want)
		}
	}
	if got["agent/guard_bootstrap.py"] {
		t.Error("the frozen bootstrap must never be shipped in the bundle")
	}
	if got["gateway_proxy_parts/a.py"] || got["gateway_proxy_parts/bundle_crypto.py"] {
		t.Error("plaintext proxy sources and the decryptor must not be published")
	}

	if err := os.WriteFile(filepath.Join(agent, "gateway_agent.py"), []byte("def main(): return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, _, _ := buildGuardProxyBundle(src, agent)
	if bytes.Equal(first, changed) {
		t.Fatal("agent code change must change the bundle")
	}
}

func TestPublishedProxyBundleIsServedAndAdvertised(t *testing.T) {
	product := t.TempDir()
	writeFakeProxySource(t, filepath.Join(product, "apps", "browser-guard", "proxy"))
	agentDir := filepath.Join(product, "apps", "browser-guard", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeAgentSource(t, agentDir)
	release := filepath.Join(product, "apps", "browser-guard", "release")
	if err := os.MkdirAll(release, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "VERSION.txt"), []byte("1.1.14\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(product); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	t.Setenv("APP_DIR", filepath.Join(product, "data"))

	if heartbeatProxyBundle() != nil {
		t.Fatal("nothing published yet")
	}
	meta, err := publishGuardProxyBundle()
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.GuardVersions) != 1 || meta.GuardVersions[0] != "1.1.14" {
		t.Fatalf("bundle must target the served Guard version, got %v", meta.GuardVersions)
	}
	hb := heartbeatProxyBundle()
	if hb == nil || hb["sha256"] != meta.SHA256 {
		t.Fatalf("heartbeat must advertise the published bundle, got %v", hb)
	}

	h := &BrowserAIHandler{}
	ctx := &fasthttp.RequestCtx{}
	h.downloadProxyBundle(ctx)
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("download status %d", ctx.Response.StatusCode())
	}
	sum := sha256.Sum256(ctx.Response.Body())
	if hex.EncodeToString(sum[:]) != meta.SHA256 || string(ctx.Response.Header.Peek("X-Gateway-Bundle-SHA256")) != meta.SHA256 {
		t.Fatal("served bytes must match the advertised SHA-256")
	}
}

func TestProxyBundleNeedsGuardKeyOrSession(t *testing.T) {
	for _, path := range []string{"/api/browser-ai/setup/proxy-bundle.json", "/api/browser-ai/setup/proxy-bundle.zip"} {
		if isPublicBrowserAIRoute("GET", path) {
			t.Errorf("%s must not be anonymous", path)
		}
		if !isGuardKeyBrowserAIRoute("GET", path) {
			t.Errorf("%s must accept the Guard key", path)
		}
		if isGuardKeyBrowserAIRoute("POST", path) {
			t.Errorf("POST %s must not be allowed", path)
		}
	}
}
