package governance

import (
	"context"
	"strings"
	"testing"
	"time"

	configstoreTables "github.com/raksha/raksha/framework/configstore/tables"
)

func strPtr(s string) *string { return &s }

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
	if len(customers) != 1 || customers[0] != "cust-bank" {
		t.Fatalf("billed customers = %v, want [cust-bank]", customers)
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

	if scope, name := gs.ExhaustedBilledEntity(context.Background(), customerKey, []string{"team-dev"}); scope != "team" || name != "Developers" {
		t.Fatalf("dev member on customer key = %q/%q, want team/Developers", scope, name)
	}
	if scope, _ := gs.ExhaustedBilledEntity(context.Background(), customerKey, []string{"team-qa"}); scope != "" {
		t.Fatalf("QA member on customer key blocked by %q, want usable", scope)
	}

	gs.budgets.Store("b-cust", &configstoreTables.TableBudget{ID: "b-cust", MaxLimit: 100, CurrentUsage: 100, ResetDuration: "1M", LastReset: now})
	if scope, name := gs.ExhaustedBilledEntity(context.Background(), customerKey, []string{"team-qa"}); scope != "customer" || name != "Bank" {
		t.Fatalf("QA member with spent customer budget = %q/%q, want customer/Bank", scope, name)
	}

	gs.budgets.Store("b-team", &configstoreTables.TableBudget{ID: "b-team", MaxLimit: 10, CurrentUsage: 10, ResetDuration: "1M", LastReset: now.AddDate(0, -2, 0)})
	directKey := &configstoreTables.TableVirtualKey{ID: "vk-direct"}
	if scope, _ := gs.ExhaustedBilledEntity(context.Background(), directKey, []string{"team-dev"}); scope != "" {
		t.Fatalf("direct key billed to no team/customer blocked by %q", scope)
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

	chain := buildScopeChain(vk, teamID, customerID)
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
