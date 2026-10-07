package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/transports/gateway-http/lib"
)

func TestRBACScopeGrants_SavedAndInheritedByMembers(t *testing.T) {
	store := newSCIMTestStore()
	handler := &WorkspaceHandler{workspace: store, store: &lib.Config{ConfigStore: store}}

	customerID := "cust-1"
	_ = store.CreateTeam(nil, &tables.TableTeam{ID: "team-a", Name: "Team A", CustomerID: &customerID})
	_ = store.CreateTeam(nil, &tables.TableTeam{ID: "team-b", Name: "Team B"})
	sub := &tables.TableUser{ID: "u-sub", Username: "sub", Role: "sub_admin", AllowedSections: "settings", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	plain := &tables.TableUser{ID: "u-plain", Username: "plain", Role: "user", AllowedSections: "prompt-repository", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = store.CreateUser(nil, sub)
	_ = store.CreateUser(nil, plain)
	_ = store.AddTeamMember(nil, "team-a", sub.ID)
	_ = store.AddTeamMember(nil, "team-a", plain.ID)

	put := func(body map[string]any) int {
		ctx := makeFastHTTPCtx("PUT", "/api/rbac/scope-grants", body, "")
		handler.updateRBACScopeGrant(ctx)
		return ctx.Response.StatusCode()
	}

	if code := put(map[string]any{"scope_type": "team", "scope_id": "team-a", "permission_ids": []uint{1}, "allowed_sections": "observability/llm-logs,observability"}); code != http.StatusOK {
		t.Fatalf("team grant save failed: %d", code)
	}
	if code := put(map[string]any{"scope_type": "all_teams", "permission_ids": []uint{1}, "allowed_sections": "governance/teams"}); code != http.StatusOK {
		t.Fatalf("all-teams grant save failed: %d", code)
	}
	if code := put(map[string]any{"scope_type": "team", "scope_id": "missing", "permission_ids": []uint{1}}); code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown team, got %d", code)
	}
	if code := put(map[string]any{"scope_type": "team", "scope_id": "team-a", "permission_ids": []uint{1}, "allowed_sections": "bad section!"}); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid section key, got %d", code)
	}

	grants, err := loadRBACScopeGrants(nil, store)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	grants.Customers[customerID] = &rbacScopeGrant{PermissionIDs: []uint{1}, AllowedSections: "governance/customers"}
	if err := saveRBACScopeGrants(nil, store, grants); err != nil {
		t.Fatalf("save: %v", err)
	}

	got := effectiveAllowedSections(nil, store, sub)
	want := "settings,observability/llm-logs,observability,governance/customers,governance/teams"
	if got != want {
		t.Fatalf("effective sections = %q, want %q", got, want)
	}
	if sub.AllowedSections != "settings" {
		t.Fatalf("team grants must not overwrite the user's own sections, got %q", sub.AllowedSections)
	}
	wantPlain := "prompt-repository,observability/llm-logs,observability,governance/customers,governance/teams"
	if got := effectiveAllowedSections(nil, store, plain); got != wantPlain {
		t.Fatalf("user role member should inherit customer and team grants, got %q, want %q", got, wantPlain)
	}

	// An empty selection removes the grant.
	if code := put(map[string]any{"scope_type": "team", "scope_id": "team-a", "permission_ids": []uint{}}); code != http.StatusOK {
		t.Fatalf("clearing team grant failed: %d", code)
	}
	grants, _ = loadRBACScopeGrants(nil, store)
	if _, ok := grants.Teams["team-a"]; ok {
		t.Fatalf("expected team-a grant to be removed")
	}

	removeRBACScopeGrant(nil, store, rbacScopeCustomer, customerID)
	grants, _ = loadRBACScopeGrants(nil, store)
	if _, ok := grants.Customers[customerID]; ok {
		t.Fatalf("expected customer grant to be removed on delete")
	}
}

func TestSessionSectionsAllow_UsesOwnAndInheritedSections(t *testing.T) {
	store := newSCIMTestStore()
	_ = store.CreateTeam(nil, &tables.TableTeam{ID: "team-a", Name: "Team A"})
	viewer := &tables.TableUser{ID: "u-view", Username: "viewer", Role: "auditor", AllowedSections: "observability/dashboard", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = store.CreateUser(nil, viewer)
	_ = store.AddTeamMember(nil, "team-a", viewer.ID)

	rawLogs := []string{"observability/llm-logs"}
	charts := []string{"observability/llm-logs", "observability/dashboard"}

	if !sessionSectionsAllow(nil, store, "auditor", "viewer", charts) {
		t.Fatalf("dashboard section must reach dashboard charts")
	}
	if sessionSectionsAllow(nil, store, "auditor", "viewer", rawLogs) {
		t.Fatalf("dashboard section must not reach raw logs")
	}
	if !sessionSectionsAllow(nil, store, "admin", "viewer", rawLogs) {
		t.Fatalf("admin must not be section-scoped")
	}
	// Built-in user role is section-scoped: this viewer only has observability/dashboard.
	if sessionSectionsAllow(nil, store, "user", "viewer", rawLogs) {
		t.Fatalf("built-in user role must honor allowed_sections (llm-logs not granted)")
	}
	if !sessionSectionsAllow(nil, store, "user", "viewer", charts) {
		t.Fatalf("built-in user role must allow their granted dashboard section")
	}
	if sessionSectionsAllow(nil, store, "auditor", "ghost", charts) {
		t.Fatalf("an unknown user must not pass the section check")
	}

	grants, _ := loadRBACScopeGrants(nil, store)
	grants.Teams["team-a"] = &rbacScopeGrant{PermissionIDs: []uint{1}, AllowedSections: "observability/llm-logs"}
	if err := saveRBACScopeGrants(nil, store, grants); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !sessionSectionsAllow(nil, store, "auditor", "viewer", rawLogs) {
		t.Fatalf("a team scope grant must unlock raw logs for its members")
	}
}
