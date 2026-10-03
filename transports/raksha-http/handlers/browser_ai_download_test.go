package handlers

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestFindFirstExistingSkipsGitLFSPointer(t *testing.T) {
	dir := t.TempDir()
	pointer := filepath.Join(dir, "Raksha_Guard_Setup.exe")
	real := filepath.Join(dir, "release", "Raksha_Guard_Setup.exe")
	_ = os.WriteFile(pointer, []byte("version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 48176415\n"), 0o644)
	if _, ok := findFirstExisting([]string{pointer}); ok {
		t.Fatal("LFS pointer must not be served as the installer")
	}
	_ = os.MkdirAll(filepath.Dir(real), 0o755)
	_ = os.WriteFile(real, []byte("MZ binary"), 0o644)
	if got, ok := findFirstExisting([]string{pointer, real}); !ok || got != real {
		t.Fatalf("findFirstExisting = %q, %v", got, ok)
	}
}

func TestGuardKeyAcceptsOlderBrandHeader(t *testing.T) {
	var ctx fasthttp.RequestCtx
	ctx.Request.Header.Set("X-Acme-Guard-Key", "secret")
	if got := extractGuardKey(&ctx); got != "secret" {
		t.Fatalf("extractGuardKey = %q", got)
	}
	setGuardVersionHeaders(&ctx, "1.1.16")
	if got := string(ctx.Response.Header.Peek("X-Acme-Guard-Version")); got != "1.1.16" {
		t.Fatalf("older-brand version header = %q", got)
	}
	if got := string(ctx.Response.Header.Peek("X-Raksha-Guard-Version")); got != "1.1.16" {
		t.Fatalf("current version header = %q", got)
	}

	var cur fasthttp.RequestCtx
	cur.Request.Header.Set("X-Raksha-Guard-Key", "k")
	cur.Request.Header.Set("X-Other-Guard-Key", "ignored")
	if got := extractGuardKey(&cur); got != "k" {
		t.Fatalf("current header must win, got %q", got)
	}

	var bad fasthttp.RequestCtx
	bad.Request.Header.Set("X-Guard-Key", "nope")
	if got := extractGuardKey(&bad); got != "" {
		t.Fatalf("unexpected key from %q", got)
	}
}

func TestWriteMacZipWithHelpersAddsUpdaterAndExecBits(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll("release", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("release", "Update_Raksha_Guard_macOS.command"), []byte("#!/bin/bash\r\necho update\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "Raksha_Guard_macOS.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"Install_Raksha_Guard.command":                 "#!/bin/bash\necho install\n",
		"Raksha_Guard.app/Contents/MacOS/Raksha_Guard": "binary",
		"INSTALL_MACOS.txt":                            "docs",
	} {
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fh.SetMode(0o644)
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := writeMacZipWithHelpers(&out, src); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*zip.File{}
	for _, f := range zr.File {
		if got[f.Name] != nil {
			t.Fatalf("duplicate entry %s", f.Name)
		}
		got[f.Name] = f
	}
	wantMode := map[string]os.FileMode{
		"Install_Raksha_Guard.command":                 0o755,
		"Raksha_Guard.app/Contents/MacOS/Raksha_Guard": 0o755,
		"INSTALL_MACOS.txt":                            0o644,
		"Update_Raksha_Guard_macOS.command":            0o755,
	}
	for name, mode := range wantMode {
		f := got[name]
		if f == nil {
			t.Fatalf("missing %s", name)
		}
		if f.Mode().Perm() != mode {
			t.Fatalf("%s mode = %v, want %v", name, f.Mode().Perm(), mode)
		}
	}
	rc, err := got["Update_Raksha_Guard_macOS.command"].Open()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if bytes.Contains(body, []byte("\r")) {
		t.Fatalf("updater script still has CRLF: %q", body)
	}
	rc, err = got["INSTALL_MACOS.txt"].Open()
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(rc)
	rc.Close()
	if string(body) != "docs" {
		t.Fatalf("existing entry content changed: %q", body)
	}
}
