package handlers

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/router"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/valyala/fasthttp"
)

func TestGitSmartHTTPCloneWithoutSystemGit(t *testing.T) {
	spec := &GitRepoSpec{
		Label: "demo",
		Files: []GitRepoFile{{Path: "skills/demo/SKILL.md", Content: []byte("hello skill")}},
	}
	storage, err := buildGitRepo(spec)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repoHeadHash(storage)
	if err != nil {
		t.Fatal(err)
	}

	v1 := uploadPackRequestV1(head.String())
	body, err := uploadPackResponse(storage, v1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, pktLine("NAK\n")) {
		t.Fatalf("v1 response missing NAK")
	}
	assertPackContains(t, body[len(pktLine("NAK\n")):], "hello skill")

	v2 := uploadPackRequestV2(head.String())
	body, err = uploadPackResponse(storage, v2, true)
	if err != nil {
		t.Fatal(err)
	}
	pack := unpackSideband(t, body)
	assertPackContains(t, pack, "hello skill")

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Log("system git not installed; protocol unit checks passed")
		return
	}
	for _, version := range []string{"0", "2"} {
		if err := cloneWithGit(t, gitPath, version, spec); err != nil {
			t.Fatalf("protocol v%s clone: %v", version, err)
		}
	}
}

func uploadPackRequestV1(hash string) []byte {
	var b bytes.Buffer
	b.Write(pktLine("want " + hash + " ofs-delta\n"))
	b.Write(pktFlush())
	b.Write(pktLine("done\n"))
	return b.Bytes()
}

func uploadPackRequestV2(hash string) []byte {
	var b bytes.Buffer
	b.Write(pktLine("command=fetch\n"))
	b.Write([]byte("0001"))
	b.Write(pktLine("ofs-delta\n"))
	b.Write(pktLine("want " + hash + "\n"))
	b.Write(pktLine("done\n"))
	b.Write(pktFlush())
	return b.Bytes()
}

func assertPackContains(t *testing.T, pack []byte, needle string) {
	t.Helper()
	st := memory.NewStorage()
	parser, err := packfile.NewParserWithStorage(packfile.NewScanner(bytes.NewReader(pack)), st)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(); err != nil {
		t.Fatal(err)
	}
	found := false
	iter, err := st.IterEncodedObjects(plumbing.BlobObject)
	if err != nil {
		t.Fatal(err)
	}
	_ = iter.ForEach(func(obj plumbing.EncodedObject) error {
		r, err := obj.Reader()
		if err != nil {
			return err
		}
		defer r.Close()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r)
		if strings.Contains(buf.String(), needle) {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("pack missing %q", needle)
	}
}

func unpackSideband(t *testing.T, body []byte) []byte {
	t.Helper()
	rest := body
	prefix := pktLine("packfile\n")
	if !bytes.HasPrefix(rest, prefix) {
		t.Fatalf("missing packfile section")
	}
	rest = rest[len(prefix):]
	var pack bytes.Buffer
	for len(rest) >= 4 {
		if bytes.HasPrefix(rest, []byte("0000")) {
			return pack.Bytes()
		}
		n := 0
		for _, c := range rest[:4] {
			n <<= 4
			switch {
			case c >= '0' && c <= '9':
				n += int(c - '0')
			case c >= 'a' && c <= 'f':
				n += int(c-'a') + 10
			default:
				t.Fatalf("bad pkt header %q", rest[:4])
			}
		}
		if n < 5 || n > len(rest) {
			t.Fatalf("bad pkt length %d", n)
		}
		payload := rest[4:n]
		rest = rest[n:]
		if payload[0] != 1 {
			continue
		}
		pack.Write(payload[1:])
	}
	t.Fatal("sideband missing flush")
	return nil
}

func cloneWithGit(t *testing.T, gitPath, version string, spec *GitRepoSpec) error {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	r := router.New()
	r.GET("/info/refs", func(ctx *fasthttp.RequestCtx) { serveGitRepo(ctx, spec, "") })
	r.POST("/git-upload-pack", func(ctx *fasthttp.RequestCtx) { serveGitRepo(ctx, spec, "") })
	go fasthttp.Serve(ln, r.Handler) //nolint:errcheck
	probe, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Skipf("loopback connections are unavailable in this environment: %v", err)
	}
	_ = probe.Close()

	// Keep the clone in the writable project tree. Some sandboxed runners block
	// writes beneath the system temp directory even when the test itself is valid.
	dest, err := os.MkdirTemp(".", "skill-clone-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dest)
	cmd := exec.Command(gitPath, "-c", "protocol.version="+version, "clone", "http://"+ln.Addr().String(), dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "Could not connect to server") || strings.Contains(string(out), "Failed to connect to 127.0.0.1") {
			t.Skipf("local Git subprocess cannot access loopback in this environment: %s", out)
		}
		return &cloneError{err: err, out: out}
	}
	data, err := os.ReadFile(filepath.Join(dest, "skills", "demo", "SKILL.md"))
	if err != nil {
		return err
	}
	if string(data) != "hello skill" {
		t.Fatalf("cloned file %q", data)
	}
	return nil
}

type cloneError struct {
	err error
	out []byte
}

func (e *cloneError) Error() string {
	return e.err.Error() + ": " + string(e.out)
}
