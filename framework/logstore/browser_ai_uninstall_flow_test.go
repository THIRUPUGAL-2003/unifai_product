package logstore

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newUninstallFlowManager(t *testing.T) *BrowserAIManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_uninstall_flow="+t.Name()), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	m := NewBrowserAIManager(db)
	if err := m.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return m
}

func TestUninstallKeyFlow_PerGuardCompanyRotateExpire(t *testing.T) {
	ctx := context.Background()
	m := newUninstallFlowManager(t)

	for _, id := range []string{"guard-a", "guard-b"} {
		if _, err := m.UpsertAgentHeartbeat(ctx, &BrowserAIAgent{ID: id, Hostname: id + "-laptop"}); err != nil {
			t.Fatalf("heartbeat %s: %v", id, err)
		}
	}
	keyA, _, err := m.GetAgentUninstallKeyReveal(ctx, "guard-a")
	if err != nil || keyA == "" {
		t.Fatalf("reveal guard-a: key=%q err=%v", keyA, err)
	}
	keyB, _, err := m.GetAgentUninstallKeyReveal(ctx, "guard-b")
	if err != nil || keyB == "" || keyB == keyA {
		t.Fatalf("guard-b must have its own key: err=%v same=%v", err, keyB == keyA)
	}
	if again, _, _ := m.GetAgentUninstallKeyReveal(ctx, "guard-a"); again != keyA {
		t.Fatalf("repeat reveal on the same day must return the same key")
	}

	verify := func(agent, key string) bool {
		ok, _, err := m.VerifyAgentUninstallKey(ctx, agent, key)
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
		return ok
	}
	if !verify("guard-a", keyA) {
		t.Fatal("guard-a key must uninstall guard-a")
	}
	if verify("guard-a", keyB) {
		t.Fatal("guard-b key must not uninstall guard-a")
	}

	const company = "Company-Uninstall-Key-2026"
	if _, err := m.SaveUninstallKey(ctx, company, "admin", nil); err != nil {
		t.Fatalf("save company key: %v", err)
	}
	if !verify("guard-a", company) || !verify("guard-b", company) {
		t.Fatal("company key must uninstall every Guard")
	}
	if verify("guard-a", "wrong-key") {
		t.Fatal("wrong key must be rejected")
	}

	rotated, _, err := m.RotateAgentUninstallKey(ctx, "guard-a")
	if err != nil || rotated == "" || rotated == keyA {
		t.Fatalf("rotate must issue a new key: err=%v", err)
	}
	if verify("guard-a", keyA) {
		t.Fatal("old key must stop working after rotation")
	}
	if !verify("guard-a", rotated) {
		t.Fatal("rotated key must work")
	}

	yesterday := time.Now().UTC().Add(-25 * time.Hour)
	if err := m.db.Model(&BrowserAIAgent{}).Where("id = ?", "guard-a").Update("uninstall_key_rotated_at", yesterday).Error; err != nil {
		t.Fatalf("age key: %v", err)
	}
	if verify("guard-a", rotated) {
		t.Fatal("yesterday's key must stop working")
	}
	if !verify("guard-a", company) {
		t.Fatal("company key must still work when the daily key expired")
	}
	n, err := m.AutoRotateDailyAgentUninstallKeys(ctx)
	if err != nil || n < 1 {
		t.Fatalf("daily auto-rotate: n=%d err=%v", n, err)
	}
	today, _, err := m.GetAgentUninstallKeyReveal(ctx, "guard-a")
	if err != nil || today == "" || today == rotated {
		t.Fatalf("admin must see a new key after daily rotation: err=%v", err)
	}
	if !verify("guard-a", today) {
		t.Fatal("today's key must work")
	}
}

func TestUninstalledAgentStatusPreservedAcrossHeartbeats(t *testing.T) {
	ctx := context.Background()
	m := newUninstallFlowManager(t)

	agentID := "guard-test-uninstalled"
	// 1. Initial heartbeat registers active agent
	agent, err := m.UpsertAgentHeartbeat(ctx, &BrowserAIAgent{
		ID:       agentID,
		Hostname: "test-laptop",
		Username: "employee1",
	})
	if err != nil || agent.Status != AgentStatusActive {
		t.Fatalf("expected active agent on initial heartbeat, got status=%v err=%v", agent.Status, err)
	}

	// 2. Mark agent uninstalled (as happens when uninstall key is verified)
	uninstalledAgent, err := m.MarkAgentUninstalled(ctx, agentID)
	if err != nil {
		t.Fatalf("MarkAgentUninstalled failed: %v", err)
	}
	if uninstalledAgent.Status != AgentStatusUninstalled {
		t.Fatalf("expected status uninstalled, got %q", uninstalledAgent.Status)
	}
	if uninstalledAgent.UninstalledAt == nil {
		t.Fatal("expected non-nil UninstalledAt")
	}
	originalUninstalledAt := *uninstalledAgent.UninstalledAt

	// 3. Trailing heartbeat from shutting-down agent process lands
	hbAgent, err := m.UpsertAgentHeartbeat(ctx, &BrowserAIAgent{
		ID:       agentID,
		Hostname: "test-laptop",
		Username: "employee1",
	})
	if err != nil {
		t.Fatalf("trailing heartbeat failed: %v", err)
	}

	// 4. Must STILL be uninstalled, not resurrected to active!
	if hbAgent.Status != AgentStatusUninstalled {
		t.Fatalf("trailing heartbeat resurrected agent to %q, expected %q", hbAgent.Status, AgentStatusUninstalled)
	}
	if hbAgent.UninstalledAt == nil || !hbAgent.UninstalledAt.Equal(originalUninstalledAt) {
		t.Fatalf("UninstalledAt timestamp was modified or cleared by heartbeat")
	}

	// 5. ListAgents must list this agent with status "uninstalled"
	agents, total, err := m.ListAgents(ctx, "", "", 10, 0)
	if err != nil || total != 1 || len(agents) != 1 {
		t.Fatalf("ListAgents failed: total=%d len=%d err=%v", total, len(agents), err)
	}
	if agents[0].Status != AgentStatusUninstalled {
		t.Fatalf("ListAgents returned agent with status %q, expected %q", agents[0].Status, AgentStatusUninstalled)
	}
}

