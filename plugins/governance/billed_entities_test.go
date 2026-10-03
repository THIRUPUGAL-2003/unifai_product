package governance

import (
	"context"
	"testing"

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
