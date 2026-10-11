package handlers

import (
	"testing"

	"github.com/gateway/gateway/framework/featureflags"
	"github.com/valyala/fasthttp"
)

func TestSectionFlagForPath(t *testing.T) {
	cases := []struct {
		path string
		flag string
		ok   bool
	}{
		{path: "/api/browser-ai/intercept", flag: "section.browser-ai", ok: true},
		{path: "/api/logs", flag: "section.observability", ok: true},
		{path: "/api/providers/openai", flag: "section.models", ok: true},
		{path: "/api/feature-flags", ok: false},
		{path: "/api/config", ok: false},
		{path: "/api/session/login", ok: false},
	}
	for _, tc := range cases {
		flag, ok := SectionFlagForPath(tc.path)
		if ok != tc.ok || flag != tc.flag {
			t.Fatalf("%s: got %q %v, want %q %v", tc.path, flag, ok, tc.flag, tc.ok)
		}
	}
}

func TestSectionFlagMiddlewareBlocksDisabledSection(t *testing.T) {
	store, err := featureflags.New(featureflags.Config{})
	if err != nil {
		t.Fatal(err)
	}
	featureflags.SetActive(store)
	t.Cleanup(func() { featureflags.SetActive(nil) })
	if _, err := store.Set(t.Context(), "section.browser-ai", false); err != nil {
		t.Fatal(err)
	}

	var called bool
	handler := SectionFlagMiddleware()(func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(fasthttp.StatusOK)
	})

	blocked := &fasthttp.RequestCtx{}
	blocked.Request.SetRequestURI("/api/browser-ai/rules")
	handler(blocked)
	if called {
		t.Fatal("disabled section should not reach the handler")
	}
	if blocked.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("status %d", blocked.Response.StatusCode())
	}

	open := &fasthttp.RequestCtx{}
	open.Request.SetRequestURI("/api/config")
	handler(open)
	if !called {
		t.Fatal("settings API must stay available so the flag can be turned back on")
	}
}
