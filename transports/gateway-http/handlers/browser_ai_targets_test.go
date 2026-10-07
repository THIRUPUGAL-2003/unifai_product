package handlers

import (
	"context"
	"net"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/framework/logstore"
	"github.com/valyala/fasthttp"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestImportParentID_ClosestListedDomain(t *testing.T) {
	byDomain := map[string]logstore.BrowserTargetWebsite{
		"chatgpt.com":    {ID: "tgt-gpt"},
		"ab.chatgpt.com": {ID: "tgt-ab", ParentID: "tgt-gpt"},
		"google.com":     {ID: "tgt-google"},
		"notchatgpt.com": {ID: "tgt-other"},
	}
	cases := map[string]string{
		"x.ab.chatgpt.com":  "tgt-gpt",
		"ab.chatgpt.com":    "tgt-gpt",
		"gemini.google.com": "tgt-google",
		"chatgpt.com":       "",
		"evilchatgpt.com":   "",
		"claude.ai":         "",
	}
	for domain, want := range cases {
		if got := importParentID(domain, domain, "", byDomain); got != want {
			t.Errorf("importParentID(%q) = %q, want %q", domain, got, want)
		}
	}
}

func TestImportParentID_SamePlatformMainSite(t *testing.T) {
	byDomain := map[string]logstore.BrowserTargetWebsite{
		"chatgpt.com":     {ID: "tgt-gpt", PlatformName: "ChatGPT", HostRole: "ui"},
		"chat.openai.com": {ID: "tgt-chat", PlatformName: "ChatGPT", HostRole: "chat", ParentID: "tgt-gpt"},
		"claude.ai":       {ID: "tgt-claude", PlatformName: "Claude", HostRole: ""},
	}
	cases := []struct {
		domain, platform, role, want string
	}{
		{"files.oaiusercontent.com", "chatgpt", "file", "tgt-gpt"},
		{"api.openai.com", "ChatGPT", "chat", "tgt-gpt"},
		{"openai.com", "ChatGPT", "ui", ""},
		{"api.anthropic.com", "Claude", "chat", ""},
		{"poe.com", "Poe", "chat", ""},
	}
	for _, c := range cases {
		if got := importParentID(c.domain, c.platform, c.role, byDomain); got != c.want {
			t.Errorf("importParentID(%q, %q, %q) = %q, want %q", c.domain, c.platform, c.role, got, c.want)
		}
	}
}

func TestImportTargets_PausedRowsAndSubdomainNesting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	m := logstore.NewBrowserAIManager(db)
	if err := m.AutoMigrate(context.Background()); err != nil {
		t.Skipf("migrate: %v", err)
	}
	h := &BrowserAIHandler{manager: m}

	body := `{"overwrite":false,"targets":[
		{"domain":"ab.chatgpt.com","platform_name":"ChatGPT","host_role":"chat","monitored":true,"block_site":false},
		{"domain":"chatgpt.com","platform_name":"ChatGPT","host_role":"ui","monitored":false,"block_site":false},
		{"domain":"deepseek.com","platform_name":"DeepSeek","monitored":true,"block_site":true}
	]}`
	var rc fasthttp.RequestCtx
	rc.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}, nil)
	rc.Request.SetBody([]byte(body))
	h.importTargets(&rc)
	if rc.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status %d: %s", rc.Response.StatusCode(), rc.Response.Body())
	}
	var res map[string]any
	_ = sonic.Unmarshal(rc.Response.Body(), &res)
	if res["imported"].(float64) != 3 {
		t.Fatalf("imported = %v, want 3 (%s)", res["imported"], rc.Response.Body())
	}

	targets, _ := m.GetTargets(context.Background())
	byDomain := map[string]logstore.BrowserTargetWebsite{}
	for _, tg := range targets {
		byDomain[tg.Domain] = tg
	}
	parent := byDomain["chatgpt.com"]
	if parent.Monitored || parent.Status != "PAUSED" {
		t.Errorf("chatgpt.com imported with Monitored=FALSE should be paused, got monitored=%v status=%s", parent.Monitored, parent.Status)
	}
	child := byDomain["ab.chatgpt.com"]
	if child.ParentID != parent.ID {
		t.Errorf("ab.chatgpt.com parent_id = %q, want %q", child.ParentID, parent.ID)
	}
	if child.Monitored {
		t.Errorf("ab.chatgpt.com should follow its paused parent")
	}
	if ds := byDomain["deepseek.com"]; !ds.BlockSite || ds.Status != "BLOCKED" {
		t.Errorf("deepseek.com should be blocked, got block=%v status=%s", ds.BlockSite, ds.Status)
	}

	var again fasthttp.RequestCtx
	again.Request.SetBody([]byte(body))
	h.importTargets(&again)
	_ = sonic.Unmarshal(again.Response.Body(), &res)
	if res["already_exists"].(float64) != 3 || res["imported"].(float64) != 0 {
		t.Errorf("re-import should report 3 already existing, got %s", again.Response.Body())
	}
}

func TestImportTargets_SkipsFileDuplicatesAndNestsOldTopLevelRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	m := logstore.NewBrowserAIManager(db)
	if err := m.AutoMigrate(context.Background()); err != nil {
		t.Skipf("migrate: %v", err)
	}
	ctx := context.Background()
	parent := logstore.BrowserTargetWebsite{Domain: "chatgpt.com", PlatformName: "ChatGPT", HostRole: "ui"}
	orphan := logstore.BrowserTargetWebsite{Domain: "ab.chatgpt.com", PlatformName: "ChatGPT", HostRole: "chat"}
	if err := m.CreateTarget(ctx, &parent); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateTarget(ctx, &orphan); err != nil {
		t.Fatal(err)
	}
	h := &BrowserAIHandler{manager: m}

	body := `{"overwrite":false,"targets":[
		{"domain":"ab.chatgpt.com","platform_name":"ChatGPT","host_role":"chat"},
		{"domain":"chat.openai.com","platform_name":"ChatGPT","host_role":"chat"},
		{"domain":"CHAT.OPENAI.COM","platform_name":"ChatGPT","host_role":"chat"}
	]}`
	var rc fasthttp.RequestCtx
	rc.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}, nil)
	rc.Request.SetBody([]byte(body))
	h.importTargets(&rc)
	var res map[string]any
	_ = sonic.Unmarshal(rc.Response.Body(), &res)
	if res["imported"].(float64) != 1 || res["duplicates_in_file"].(float64) != 1 || res["nested"].(float64) != 1 {
		t.Fatalf("want 1 imported, 1 in-file duplicate, 1 nested; got %s", rc.Response.Body())
	}

	targets, _ := m.GetTargets(ctx)
	for _, tg := range targets {
		switch tg.Domain {
		case "ab.chatgpt.com", "chat.openai.com":
			if tg.ParentID != parent.ID {
				t.Errorf("%s parent_id = %q, want %q", tg.Domain, tg.ParentID, parent.ID)
			}
		}
	}
	if len(targets) != 3 {
		t.Errorf("want 3 targets, got %d", len(targets))
	}
}

func TestIsReservedBrowserAITargetPathID(t *testing.T) {
	if !isReservedBrowserAITargetPathID("import") {
		t.Errorf("expected 'import' to be reserved")
	}
	if !isReservedBrowserAITargetPathID("IMPORT") {
		t.Errorf("expected 'IMPORT' to be reserved")
	}
	if !isReservedBrowserAITargetPathID(" import ") {
		t.Errorf("expected ' import ' to be reserved")
	}
	if isReservedBrowserAITargetPathID("tgt-12345678") {
		t.Errorf("expected 'tgt-12345678' NOT to be reserved")
	}
	if isReservedBrowserAITargetPathID("") {
		t.Errorf("expected empty string NOT to be reserved")
	}
}

func TestImportTargetsPayload_Unmarshal(t *testing.T) {
	rawJSON := `{
		"overwrite": true,
		"targets": [
			{
				"domain": "https://chatgpt.com/",
				"platform_name": "ChatGPT",
				"host_role": "ui",
				"monitored": true,
				"block_site": false
			},
			{
				"domain": "deepseek.com",
				"platform_name": "DeepSeek",
				"host_role": "chat",
				"monitored": true,
				"block_site": true
			}
		]
	}`

	var payload importTargetsPayload
	if err := sonic.Unmarshal([]byte(rawJSON), &payload); err != nil {
		t.Fatalf("failed to unmarshal importTargetsPayload: %v", err)
	}

	if !payload.Overwrite {
		t.Errorf("expected Overwrite to be true")
	}
	if len(payload.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(payload.Targets))
	}

	t1 := payload.Targets[0]
	normalizedDomain1 := logstore.NormalizeDomain(t1.Domain)
	if normalizedDomain1 != "chatgpt.com" {
		t.Errorf("expected normalized domain 'chatgpt.com', got '%s'", normalizedDomain1)
	}
	if t1.PlatformName != "ChatGPT" {
		t.Errorf("expected platform name 'ChatGPT', got '%s'", t1.PlatformName)
	}
	if t1.HostRole != "ui" {
		t.Errorf("expected host role 'ui', got '%s'", t1.HostRole)
	}
	if t1.Monitored == nil || !*t1.Monitored {
		t.Errorf("expected monitored to be true")
	}
	if t1.BlockSite == nil || *t1.BlockSite {
		t.Errorf("expected block_site to be false")
	}

	t2 := payload.Targets[1]
	normalizedDomain2 := logstore.NormalizeDomain(t2.Domain)
	if normalizedDomain2 != "deepseek.com" {
		t.Errorf("expected normalized domain 'deepseek.com', got '%s'", normalizedDomain2)
	}
	if t2.BlockSite == nil || !*t2.BlockSite {
		t.Errorf("expected block_site to be true")
	}
}

func TestNormalizeTargetHostRoles(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ui", "ui"},
		{"chat", "chat"},
		{"file", "file"},
		{"", ""},
		{"auto", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		actual := logstore.NormalizeHostRole(tt.input)
		if actual != tt.expected {
			t.Errorf("NormalizeHostRole(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}
