package governance

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gateway/gateway/core/schemas"
	configstoreTables "github.com/gateway/gateway/framework/configstore/tables"
)

func strPtr(s string) *string { return &s }

func TestUserOnlyKeyChargesUserAndVKOnly(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-dev"}})
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{
		ID: "team-dev", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	// Direct user assignment only — no team / customer links on the VK.
	vk := &configstoreTables.TableVirtualKey{
		ID:      "vk-user",
		Budgets: []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-user", vk)

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-user", UserID: "user-1", Success: true, Cost: 5,
		UserTeamIDs: []string{"team-dev"},
	})

	want := map[string]float64{"b-user": 5, "b-vk": 5, "b-team": 0, "b-cust": 0}
	for id, expected := range want {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != expected {
			t.Errorf("budget %s usage = %v, want %v", id, got, expected)
		}
	}
}

func TestTeamOnlyKeyChargesTeamUserAndVKNotCustomer(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-dev"}})
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{
		ID: "team-dev", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID:      "vk-team",
		TeamID:  strPtr("team-dev"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-team", vk)

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-team", UserID: "user-1", Success: true, Cost: 5,
		UserTeamIDs: []string{"team-dev"},
	})

	want := map[string]float64{"b-user": 5, "b-vk": 5, "b-team": 5, "b-cust": 0}
	for id, expected := range want {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != expected {
			t.Errorf("budget %s usage = %v, want %v", id, got, expected)
		}
	}
}

func TestBilledTeamIDDoesNotBillForeignTeamWhenUserKnown(t *testing.T) {
	vk := &configstoreTables.TableVirtualKey{ID: "vk-multi", TeamIDs: []string{"team-a", "team-b"}}
	ctx := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-other"})
	if got := billedTeamID(ctx, vk); got != "" {
		t.Fatalf("billedTeamID = %q, want empty when user is not on any VK team", got)
	}
	if got := billedTeamID(context.Background(), vk); got != "team-a" {
		t.Fatalf("billedTeamID without membership = %q, want team-a", got)
	}
}

func TestTeamOnlyKeyDoesNotBillParentCustomer(t *testing.T) {
	gs := &LocalGovernanceStore{}
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{ID: "team-dev", CustomerID: strPtr("cust-bank")})

	vk := &configstoreTables.TableVirtualKey{ID: "vk-team", TeamID: strPtr("team-dev")}
	team, customers := gs.BilledEntities(context.Background(), vk)
	if team != "team-dev" {
		t.Fatalf("billed team = %q, want team-dev", team)
	}
	if len(customers) != 0 {
		t.Fatalf("team-only key billed customers %v, want none", customers)
	}
}

func TestCustomerKeyBillsCustomer(t *testing.T) {
	gs := &LocalGovernanceStore{}
	vk := &configstoreTables.TableVirtualKey{
		ID:        "vk-cust",
		Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
	}
	team, customers := gs.BilledEntities(context.Background(), vk)
	if team != "" {
		t.Fatalf("customer key billed team %q, want none", team)
	}
	if len(customers) != 0 {
		t.Fatalf("billed customers = %v, want none when no user is in the customer tree", customers)
	}
	if gs.customerAssignedVKAllowed(context.Background(), vk) {
		t.Fatal("customer key allowed with no user in the customer tree")
	}
}

func TestCustomerKeyBillsUsersTeamAndCustomer(t *testing.T) {
	gs := &LocalGovernanceStore{}
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{ID: "team-dev", CustomerID: strPtr("cust-bank")})
	gs.teams.Store("team-other", &configstoreTables.TableTeam{ID: "team-other", CustomerID: strPtr("cust-other")})
	vk := &configstoreTables.TableVirtualKey{
		ID:        "vk-cust",
		Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
	}
	ctx := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-other", "team-dev"})
	team, customers := gs.BilledEntities(ctx, vk)
	if team != "team-dev" {
		t.Fatalf("billed team = %q, want the user's team under the customer (team-dev)", team)
	}
	if len(customers) != 1 || customers[0] != "cust-bank" {
		t.Fatalf("billed customers = %v, want [cust-bank]", customers)
	}
}

func TestTwoCustomersChargeOnlyTheUsersCustomerAndTeam(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team1", "b-team2", "b-c1", "b-c2", "b-vk"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-1"}})
	gs.teams.Store("team-1", &configstoreTables.TableTeam{
		ID: "team-1", CustomerID: strPtr("cust-1"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team1"}},
	})
	gs.teams.Store("team-2", &configstoreTables.TableTeam{
		ID: "team-2", CustomerID: strPtr("cust-1"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team2"}},
	})
	gs.customers.Store("cust-1", &configstoreTables.TableCustomer{
		ID: "cust-1", Budgets: []configstoreTables.TableBudget{{ID: "b-c1"}},
	})
	gs.customers.Store("cust-2", &configstoreTables.TableCustomer{
		ID: "cust-2", Budgets: []configstoreTables.TableBudget{{ID: "b-c2"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID: "vk-two",
		Customers: []configstoreTables.TableCustomer{{ID: "cust-1"}, {ID: "cust-2"}},
		Budgets:   []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-two", vk)
	index := map[string][]buStamp{
		"team-1": {{ID: "bu-1", CustomerID: strPtr("cust-1")}},
	}
	gs.businessUnitsByTeam.Store(&index)
	gs.budgets.Store("bu_bu-1", &configstoreTables.TableBudget{ID: "bu_bu-1", MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-two", UserID: "user-1", Success: true, Cost: 5,
		UserTeamIDs: []string{"team-1"},
	})

	want := map[string]float64{
		"b-user": 5, "b-vk": 5, "b-team1": 5, "b-c1": 5, "bu_bu-1": 5,
		"b-team2": 0, "b-c2": 0,
	}
	for id, expected := range want {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != expected {
			t.Errorf("budget %s usage = %v, want %v", id, got, expected)
		}
	}
}

func TestBusinessUnitKeyChargesOnlyThatUnitNotItsCustomerOrSibling(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk", "bu_bu-a", "bu_bu-b"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-a"}})
	gs.teams.Store("team-a", &configstoreTables.TableTeam{
		ID: "team-a", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID:              "vk-bu",
		BusinessUnitIDs: []string{"bu-a", "bu-b"},
		Budgets:         []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-bu", vk)
	index := map[string][]buStamp{
		"team-a": {{ID: "bu-a", CustomerID: strPtr("cust-bank")}},
	}
	gs.businessUnitsByTeam.Store(&index)

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-bu", UserID: "user-1", Success: true, Cost: 5,
		UserTeamIDs: []string{"team-a"},
	})

	want := map[string]float64{
		"b-user": 5, "b-vk": 5, "b-team": 5, "bu_bu-a": 5,
		"bu_bu-b": 0, "b-cust": 0,
	}
	for id, expected := range want {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != expected {
			t.Errorf("budget %s usage = %v, want %v", id, got, expected)
		}
	}
}

func TestCustomerKeyUsageChargesUserTeamCustomerAndKey(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user")})
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{
		ID: "team-dev", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID:        "vk-cust",
		Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
		Budgets:   []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-cust", vk)

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-cust", UserID: "user-1", Success: true, Cost: 5,
		UserTeamIDs: []string{"team-dev"},
	})

	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != 5 {
			t.Errorf("budget %s usage = %v, want 5", id, got)
		}
	}
}

func TestCustomerKeyUsageResolvesUserTeamWhenUserTeamIDsEmpty(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-dev"}})
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{
		ID: "team-dev", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID:        "vk-cust",
		Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
		Budgets:   []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-cust", vk)

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	// UserTeamIDs is explicitly NOT provided; should resolve from UserGovernance.TeamIDs
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-cust", UserID: "user-1", Success: true, Cost: 5,
	})

	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != 5 {
			t.Errorf("budget %s usage = %v, want 5", id, got)
		}
	}
}

func TestExhaustedBilledEntity(t *testing.T) {
	gs := &LocalGovernanceStore{}
	now := time.Now()
	gs.budgets.Store("b-team", &configstoreTables.TableBudget{ID: "b-team", MaxLimit: 10, CurrentUsage: 10, ResetDuration: "1M", LastReset: now})
	gs.budgets.Store("b-cust", &configstoreTables.TableBudget{ID: "b-cust", MaxLimit: 100, CurrentUsage: 5, ResetDuration: "1M", LastReset: now})
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{
		ID: "team-dev", Name: "Developers", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.teams.Store("team-qa", &configstoreTables.TableTeam{ID: "team-qa", Name: "QA", CustomerID: strPtr("cust-bank")})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Name: "Bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	customerKey := &configstoreTables.TableVirtualKey{ID: "vk-cust", Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}}}
	gs.addVKScopedUsage("vk-cust", "b-team", 10)

	if scope, name := gs.ExhaustedBilledEntity(context.Background(), customerKey, []string{"team-dev"}); scope != "team" || name != "Developers" {
		t.Fatalf("dev member on customer key = %q/%q, want team/Developers", scope, name)
	}
	if scope, _ := gs.ExhaustedBilledEntity(context.Background(), customerKey, []string{"team-qa"}); scope != "" {
		t.Fatalf("QA member on customer key blocked by %q, want usable", scope)
	}

	gs.budgets.Store("b-cust", &configstoreTables.TableBudget{ID: "b-cust", MaxLimit: 100, CurrentUsage: 100, ResetDuration: "1M", LastReset: now})
	gs.addVKScopedUsage("vk-cust", "b-cust", 100)
	if scope, name := gs.ExhaustedBilledEntity(context.Background(), customerKey, []string{"team-qa"}); scope != "customer" || name != "Bank" {
		t.Fatalf("QA member with spent customer budget = %q/%q, want customer/Bank", scope, name)
	}

	gs.budgets.Store("b-team", &configstoreTables.TableBudget{ID: "b-team", MaxLimit: 10, CurrentUsage: 10, ResetDuration: "1M", LastReset: now.AddDate(0, -2, 0)})
	directKey := &configstoreTables.TableVirtualKey{ID: "vk-direct"}
	if scope, _ := gs.ExhaustedBilledEntity(context.Background(), directKey, []string{"team-dev"}); scope != "" {
		t.Fatalf("direct key billed to no team/customer blocked by %q", scope)
	}

	gs.budgets.Store("b-vk", &configstoreTables.TableBudget{ID: "b-vk", MaxLimit: 5, CurrentUsage: 5, ResetDuration: "1M", LastReset: now})
	spentVK := &configstoreTables.TableVirtualKey{
		ID: "vk-spent", Name: "SpentKey",
		Budgets: []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	if scope, name := gs.ExhaustedBilledEntity(context.Background(), spentVK, []string{"team-qa"}); scope != "vk" || name != "SpentKey" {
		t.Fatalf("spent VK budget = %q/%q, want vk/SpentKey", scope, name)
	}
}

func TestBudgetExhaustedReasonNamesTheSpentLevel(t *testing.T) {
	cases := map[string]string{
		"User:u1":     "Your personal budget is used up ($10.00 of $10.00 used)",
		"Team:t1":     "Team budget is used up ($10.00 of $10.00 used). This virtual key is blocked for your team — please use another virtual key.",
		"Team":        "Team budget is used up",
		"Customer:c1": "Customer budget is used up ($10.00 of $10.00 used). This virtual key is blocked for all of the customer's teams",
		"VK":          "Virtual key budget is used up ($10.00 of $10.00 used). Please use another virtual key.",
	}
	for entity, want := range cases {
		got := budgetExhaustedReason(&BudgetExceededError{Entity: entity, Usage: 10, Limit: 10}, DecisionBudgetExceeded)
		if !strings.HasPrefix(got, want) {
			t.Errorf("%s: reason = %q, want prefix %q", entity, got, want)
		}
	}
}

func TestAlignUserBudgetResetsOnTheFirstOfTheMonth(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	inWindow := &configstoreTables.TableBudget{ResetDuration: "1M", LastReset: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), CurrentUsage: 7}
	alignUserBudget(inWindow, now)
	if !inWindow.IsCalendarAligned || !inWindow.LastReset.Equal(start) || inWindow.CurrentUsage != 7 {
		t.Fatalf("budget inside its rolling window = %+v, want aligned to %v with usage kept", inWindow, start)
	}

	expired := &configstoreTables.TableBudget{ResetDuration: "1M", LastReset: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), CurrentUsage: 7}
	alignUserBudget(expired, now)
	gs := &LocalGovernanceStore{}
	if target := gs.budgetResetTarget(expired, now); target == nil || !target.Equal(start) {
		t.Fatalf("expired budget reset target = %v, want %v", target, start)
	}
}

func TestRoutingScopeChainFollowsBilledTeam(t *testing.T) {
	gs := &LocalGovernanceStore{}
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{ID: "team-dev", Name: "Developers", CustomerID: strPtr("cust-bank")})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{ID: "cust-bank", Name: "Bank"})
	vk := &configstoreTables.TableVirtualKey{
		ID:        "vk-cust",
		Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
	}
	ctx := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-dev"})
	teamID, teamName, customerID, customerName := gs.billedIdentity(ctx, vk)
	if teamID != "team-dev" || teamName != "Developers" || customerID != "cust-bank" || customerName != "Bank" {
		t.Fatalf("billed identity = %q/%q/%q/%q", teamID, teamName, customerID, customerName)
	}

	chain := buildScopeChain(vk, teamID, customerID, "")
	want := []ScopeLevel{
		{ScopeName: "virtual_key", ScopeID: "vk-cust"},
		{ScopeName: "team", ScopeID: "team-dev"},
		{ScopeName: "customer", ScopeID: "cust-bank"},
		{ScopeName: "global", ScopeID: ""},
	}
	if len(chain) != len(want) {
		t.Fatalf("scope chain = %v, want %v", chain, want)
	}
	for i := range want {
		if chain[i] != want[i] {
			t.Fatalf("scope chain = %v, want %v", chain, want)
		}
	}
}

func TestKeyOnTeamAndCustomerPrefersTeamsOwnCustomer(t *testing.T) {
	gs := &LocalGovernanceStore{}
	gs.teams.Store("team-qa", &configstoreTables.TableTeam{ID: "team-qa", CustomerID: strPtr("cust-b")})
	vk := &configstoreTables.TableVirtualKey{
		ID:        "vk-both",
		Teams:     []configstoreTables.TableTeam{{ID: "team-qa"}},
		Customers: []configstoreTables.TableCustomer{{ID: "cust-a"}, {ID: "cust-b"}},
	}
	_, customers := gs.BilledEntities(context.Background(), vk)
	if len(customers) != 1 || customers[0] != "cust-b" {
		t.Fatalf("billed customers = %v, want [cust-b]", customers)
	}
}

func TestSharedTeamMultiCustomerScoping(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team-shared", "b-cust-a", "b-cust-b", "b-vk-a", "b-vk-b"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	// User belongs to single team: team-shared
	gs.users.Store("user-shared", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-shared"}})
	// Shared team has NO single customer owning it (CustomerID = nil)
	gs.teams.Store("team-shared", &configstoreTables.TableTeam{
		ID: "team-shared", Name: "Shared Multi-Customer Team", CustomerID: nil,
		Budgets: []configstoreTables.TableBudget{{ID: "b-team-shared"}},
	})
	gs.customers.Store("cust-a", &configstoreTables.TableCustomer{
		ID: "cust-a", Name: "Customer A", Budgets: []configstoreTables.TableBudget{{ID: "b-cust-a"}},
	})
	gs.customers.Store("cust-b", &configstoreTables.TableCustomer{
		ID: "cust-b", Name: "Customer B", Budgets: []configstoreTables.TableBudget{{ID: "b-cust-b"}},
	})

	// VK-A: Scoped to Customer A, accessible by team-shared
	vkA := &configstoreTables.TableVirtualKey{
		ID:         "vk-a",
		CustomerID: strPtr("cust-a"),
		Teams:      []configstoreTables.TableTeam{{ID: "team-shared"}},
		Budgets:    []configstoreTables.TableBudget{{ID: "b-vk-a"}},
	}
	gs.virtualKeys.Store("sk-vk-a", vkA)

	// VK-B: Scoped to Customer B, accessible by team-shared
	vkB := &configstoreTables.TableVirtualKey{
		ID:         "vk-b",
		CustomerID: strPtr("cust-b"),
		Teams:      []configstoreTables.TableTeam{{ID: "team-shared"}},
		Budgets:    []configstoreTables.TableBudget{{ID: "b-vk-b"}},
	}
	gs.virtualKeys.Store("sk-vk-b", vkB)

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}

	// 1. User performs action for Customer A
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-vk-a", UserID: "user-shared", Success: true, Cost: 10,
		UserTeamIDs: []string{"team-shared"},
	})

	checkBudget := func(id string, want float64) {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != want {
			t.Errorf("budget %s usage = %v, want %v", id, got, want)
		}
	}

	checkBudget("b-vk-a", 10)
	checkBudget("b-user", 10)
	checkBudget("b-team-shared", 10)
	checkBudget("b-cust-a", 10) // VK assigned to Customer A bills Customer A.
	checkBudget("b-cust-b", 0)

	// 2. User performs action for Customer B
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-vk-b", UserID: "user-shared", Success: true, Cost: 25,
		UserTeamIDs: []string{"team-shared"},
	})

	checkBudget("b-vk-a", 10)
	checkBudget("b-vk-b", 25)
	checkBudget("b-user", 35)        // 10 + 25
	checkBudget("b-team-shared", 35) // 10 + 25
	checkBudget("b-cust-a", 10)
	checkBudget("b-cust-b", 25)
}

func TestCustomerKeyOutsiderIsNotBilled(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-out", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-other"}})
	gs.teams.Store("team-other", &configstoreTables.TableTeam{
		ID: "team-other", CustomerID: strPtr("cust-other"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID: "vk-cust", Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
		Budgets: []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-cust", vk)
	ctx := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-other"})
	if gs.customerAssignedVKAllowed(ctx, vk) {
		t.Fatal("outsider allowed on a customer virtual key")
	}
	team, customers := gs.BilledEntities(ctx, vk)
	if team != "" || len(customers) != 0 {
		t.Fatalf("outsider billed team %q customers %v", team, customers)
	}
}

func TestCustomerKeyDirectTeamDoesNotChargeBusinessUnit(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk", "bu_bu-1"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-direct"}})
	gs.teams.Store("team-direct", &configstoreTables.TableTeam{
		ID: "team-direct", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID: "vk-cust", Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
		Budgets: []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-cust", vk)

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-cust", UserID: "user-1", Success: true, Cost: 5,
		UserTeamIDs: []string{"team-direct"},
	})
	want := map[string]float64{"b-user": 5, "b-team": 5, "b-cust": 5, "b-vk": 5, "bu_bu-1": 0}
	for id, expected := range want {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != expected {
			t.Errorf("budget %s usage = %v, want %v", id, got, expected)
		}
	}
}

func TestCustomerKeyBusinessUnitTeamIsBilledWhenTeamHasNoCustomer(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-direct", "b-cust", "b-vk", "bu_bu-1"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-1", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-direct", "team-bu"}})
	gs.teams.Store("team-direct", &configstoreTables.TableTeam{
		ID: "team-direct", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-direct"}},
	})
	gs.teams.Store("team-bu", &configstoreTables.TableTeam{
		ID: "team-bu",
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	index := map[string][]buStamp{
		"team-bu": {{ID: "bu-1", CustomerID: strPtr("cust-bank")}},
	}
	gs.businessUnitsByTeam.Store(&index)
	vk := &configstoreTables.TableVirtualKey{
		ID: "vk-cust", Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}},
		Budgets: []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-cust", vk)
	ctx := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-direct", "team-bu"})
	if !gs.customerAssignedVKAllowed(ctx, vk) {
		t.Fatal("user in the customer business unit was blocked")
	}
	team, customers := gs.BilledEntities(ctx, vk)
	if team != "team-bu" {
		t.Fatalf("billed team = %q, want team-bu", team)
	}
	if len(customers) != 1 || customers[0] != "cust-bank" {
		t.Fatalf("billed customers = %v, want [cust-bank]", customers)
	}

	tracker := &UsageTracker{store: gs, logger: NewMockLogger(), billed: make(map[string]time.Time)}
	tracker.UpdateUsage(context.Background(), &UsageUpdate{
		VirtualKey: "sk-cust", UserID: "user-1", Success: true, Cost: 5,
		UserTeamIDs: []string{"team-direct", "team-bu"},
	})
	want := map[string]float64{"b-user": 5, "b-team": 5, "b-direct": 0, "b-cust": 5, "b-vk": 5, "bu_bu-1": 5}
	for id, expected := range want {
		raw, _ := gs.budgets.Load(id)
		if got := raw.(*configstoreTables.TableBudget).CurrentUsage; got != expected {
			t.Errorf("budget %s usage = %v, want %v", id, got, expected)
		}
	}
}

func TestTeamKeyRejectsUserOutsideTheTeam(t *testing.T) {
	gs := &LocalGovernanceStore{}
	vk := &configstoreTables.TableVirtualKey{ID: "vk-team", TeamID: strPtr("team-dev")}
	ctx := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-other"})
	if gs.customerAssignedVKAllowed(ctx, vk) {
		t.Fatal("user outside the team was allowed to use the team virtual key")
	}
	inside := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-dev"})
	if !gs.customerAssignedVKAllowed(inside, vk) {
		t.Fatal("team member was blocked from the team virtual key")
	}
}

func TestUserKeyRejectsEveryOtherUser(t *testing.T) {
	gs := &LocalGovernanceStore{}
	vk := &configstoreTables.TableVirtualKey{ID: "vk-user", UserIDs: []string{"user-1"}}
	other := context.WithValue(context.Background(), schemas.GatewayContextKeyUserID, "user-2")
	if gs.customerAssignedVKAllowed(other, vk) {
		t.Fatal("another user was allowed to use a user-assigned virtual key")
	}
	owner := context.WithValue(context.Background(), schemas.GatewayContextKeyUserID, "user-1")
	if !gs.customerAssignedVKAllowed(owner, vk) {
		t.Fatal("the assigned user was blocked")
	}
}

func TestOutsideTeamOnCustomerKeyDoesNotChargeCustomer(t *testing.T) {
	gs := &LocalGovernanceStore{}
	for _, id := range []string{"b-user", "b-team", "b-cust", "b-vk"} {
		gs.budgets.Store(id, &configstoreTables.TableBudget{ID: id, MaxLimit: 1000, ResetDuration: "1M", LastReset: time.Now()})
	}
	gs.users.Store("user-out", &UserGovernance{BudgetID: strPtr("b-user"), TeamIDs: []string{"team-out"}})
	gs.teams.Store("team-out", &configstoreTables.TableTeam{
		ID: "team-out", CustomerID: strPtr("cust-other"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	gs.customers.Store("cust-bank", &configstoreTables.TableCustomer{
		ID: "cust-bank", Budgets: []configstoreTables.TableBudget{{ID: "b-cust"}},
	})
	vk := &configstoreTables.TableVirtualKey{
		ID: "vk-both", CustomerID: strPtr("cust-bank"),
		Teams:   []configstoreTables.TableTeam{{ID: "team-out"}},
		Budgets: []configstoreTables.TableBudget{{ID: "b-vk"}},
	}
	gs.virtualKeys.Store("sk-both", vk)
	ctx := context.WithValue(context.Background(), governanceUserTeamIDsContextKey, []string{"team-out"})
	if !gs.customerAssignedVKAllowed(ctx, vk) {
		t.Fatal("team assigned on the key was blocked")
	}
	_, customers := gs.BilledEntities(ctx, vk)
	if len(customers) != 0 {
		t.Fatalf("outside team billed customers %v", customers)
	}
}

func TestTeamBudgetTotalBlocksEveryKey(t *testing.T) {
	gs := &LocalGovernanceStore{}
	now := time.Now()
	gs.budgets.Store("b-team", &configstoreTables.TableBudget{ID: "b-team", MaxLimit: 10, CurrentUsage: 10, ResetDuration: "1M", LastReset: now})
	gs.teams.Store("team-dev", &configstoreTables.TableTeam{
		ID: "team-dev", Name: "Developers", CustomerID: strPtr("cust-bank"),
		Budgets: []configstoreTables.TableBudget{{ID: "b-team"}},
	})
	vk := &configstoreTables.TableVirtualKey{ID: "vk-this", Customers: []configstoreTables.TableCustomer{{ID: "cust-bank"}}}
	if scope, name := gs.ExhaustedBilledEntity(context.Background(), vk, []string{"team-dev"}); scope != "team" || name != "Developers" {
		t.Fatalf("team total of 10 on a 10 limit = %q/%q, want team/Developers", scope, name)
	}
}

func TestBusinessUnitBudgetEnforcement(t *testing.T) {
	gs := &LocalGovernanceStore{logger: NewMockLogger()}
	now := time.Now()
	// BU budget exhausted (usage 10 >= limit 10)
	gs.budgets.Store("bu_bu-1", &configstoreTables.TableBudget{
		ID:            "bu_bu-1",
		MaxLimit:      10,
		CurrentUsage:  10,
		ResetDuration: "1M",
		LastReset:     now,
	})

	resolver := &BudgetResolver{store: gs, logger: NewMockLogger()}
	req := &EvaluationRequest{Model: "gpt-4"}

	// Evaluate exhausted BU request -> should be blocked with DecisionBudgetExceeded
	result := resolver.EvaluateBusinessUnitRequest(nil, "bu-1", req)
	if result.Decision != DecisionBudgetExceeded {
		t.Fatalf("expected DecisionBudgetExceeded, got %s", result.Decision)
	}
	if !strings.Contains(result.Reason, "Business unit budget is used up") {
		t.Fatalf("expected reason to contain 'Business unit budget is used up', got: %s", result.Reason)
	}

	// Active BU request -> should be allowed
	gs.budgets.Store("bu_bu-2", &configstoreTables.TableBudget{
		ID:            "bu_bu-2",
		MaxLimit:      100,
		CurrentUsage:  10,
		ResetDuration: "1M",
		LastReset:     now,
	})
	resultAllowed := resolver.EvaluateBusinessUnitRequest(nil, "bu-2", req)
	if resultAllowed.Decision != DecisionAllow {
		t.Fatalf("expected DecisionAllow, got %s", resultAllowed.Decision)
	}
}

