package logstore

import (
	"context"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeDomain_AdminInputForms(t *testing.T) {
	cases := map[string]string{
		"chatgpt.com":                        "chatgpt.com",
		"  ChatGPT.COM  ":                    "chatgpt.com",
		"https://www.chatgpt.com/c/abc?x=1":  "chatgpt.com",
		"http://gemini.google.com:443/app":   "gemini.google.com",
		"*.deepseek.com":                     "deepseek.com",
		".claude.ai":                         "claude.ai",
		"copilot.microsoft.com.":             "copilot.microsoft.com",
		"*.www.grok.com/":                    "grok.com",
		"chat.deepseek.com":                  "chat.deepseek.com",
		"ai.acme-internal.io":                "ai.acme-internal.io",
		"10.0.0.5":                           "10.0.0.5",
		"bücher.example":                     "xn--bcher-kva.example",
		"":                                   "",
		"chat gpt.com":                       "",
		"my_host.local":                      "",
		"a..b.com":                           "",
		"https://":                           "",
	}
	for in, want := range cases {
		if got := NormalizeDomain(in); got != want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

// Uses RAKSHA_TEST_PG_DSN (throwaway database, tables are dropped) when set; sqlite otherwise.
func newTargetsDomainManager(t *testing.T) *BrowserAIManager {
	t.Helper()
	var db *gorm.DB
	var err error
	if dsn := os.Getenv("RAKSHA_TEST_PG_DSN"); dsn != "" {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			_ = db.Migrator().DropTable(&BrowserTargetWebsite{})
		}
	} else {
		db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared&_targets_domain="+t.Name()), &gorm.Config{})
	}
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	m := NewBrowserAIManager(db)
	if err := m.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return m
}

func TestTargets_AnyDomainAndSubdomainReachPAC(t *testing.T) {
	ctx := context.Background()
	m := newTargetsDomainManager(t)

	parents := map[string]string{
		"ChatGPT":  "https://chatgpt.com/",
		"Claude":   "claude.ai",
		"Gemini":   "gemini.google.com",
		"DeepSeek": "*.deepseek.com",
		"Copilot":  "copilot.microsoft.com.",
		"Grok":     "grok.com",
		"Acme AI":  "https://ai.acme-internal.io:8443/chat",
	}
	ids := map[string]string{}
	for name, domain := range parents {
		tg := &BrowserTargetWebsite{Domain: domain, PlatformName: name}
		if err := m.CreateTarget(ctx, tg); err != nil {
			t.Fatalf("create %s (%q): %v", name, domain, err)
		}
		ids[name] = tg.ID
	}
	children := []struct{ parent, domain, role string }{
		{"ChatGPT", "files.oaiusercontent.com", "file"},
		{"ChatGPT", "ab.chatgpt.com", "chat"},
		{"Claude", "*.claudeusercontent.com", "file"},
		{"Gemini", "content-push.googleapis.com", "file"},
		{"DeepSeek", "cdn.deepseek.com", ""},
		{"Copilot", "substrate.office.com", "chat"},
	}
	for _, c := range children {
		tg := &BrowserTargetWebsite{Domain: c.domain, PlatformName: c.parent, ParentID: ids[c.parent], HostRole: c.role}
		if err := m.CreateTarget(ctx, tg); err != nil {
			t.Fatalf("create child %q: %v", c.domain, err)
		}
		if !tg.Monitored || tg.ParentID != ids[c.parent] {
			t.Fatalf("child %q must inherit Monitor from %s: monitored=%v parent=%q", c.domain, c.parent, tg.Monitored, tg.ParentID)
		}
	}
	if err := m.CreateTarget(ctx, &BrowserTargetWebsite{Domain: "chat gpt.com"}); err == nil {
		t.Fatalf("a domain with a space must be rejected, not saved and silently left out of proxy.pac")
	}

	pac, _ := m.BuildProxyPAC(ctx, "127.0.0.1:18103")
	for _, host := range []string{
		"chatgpt.com", "claude.ai", "gemini.google.com", "deepseek.com", "copilot.microsoft.com",
		"grok.com", "ai.acme-internal.io", "files.oaiusercontent.com", "claudeusercontent.com",
		"content-push.googleapis.com", "substrate.office.com",
	} {
		if !strings.Contains(pac, `"`+host+`"`) {
			t.Errorf("proxy.pac is missing %q", host)
		}
	}
	for _, covered := range []string{`"ab.chatgpt.com"`, `"cdn.deepseek.com"`} {
		if strings.Contains(pac, covered) {
			t.Errorf("%s is covered by its parent and should be collapsed", covered)
		}
	}
	if strings.Contains(pac, "        \"*") {
		t.Errorf("wildcard entries must not reach proxy.pac:\n%s", pac)
	}
	for _, notAdded := range []string{`"google.com"`, `"bing.com"`, `"msn.com"`, `"duckduckgo.com"`, `"search.yahoo.com"`, "shExpMatch(host, \"*google.*\")"} {
		if strings.Contains(pac, notAdded) {
			t.Errorf("only admin Target Websites may be routed; found %s", notAdded)
		}
	}

	// Pausing the parent stops routing (and monitoring) for it.
	if err := m.UpdateTarget(ctx, ids["Grok"], map[string]any{"monitored": false}); err != nil {
		t.Fatalf("pause grok: %v", err)
	}
	pac, _ = m.BuildProxyPAC(ctx, "127.0.0.1:18103")
	if strings.Contains(pac, `"grok.com"`) {
		t.Errorf("paused target must leave proxy.pac")
	}
}
