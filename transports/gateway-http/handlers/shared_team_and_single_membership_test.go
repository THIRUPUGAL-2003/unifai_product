package handlers

import (
	"context"
	"testing"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
)

type mockSingleTeamStore struct {
	configstore.ConfigStore
	customers map[string]*tables.TableCustomer
	teams     map[string]*tables.TableTeam
}

func (m *mockSingleTeamStore) GetCustomer(ctx context.Context, id string) (*tables.TableCustomer, error) {
	if c, ok := m.customers[id]; ok {
		return c, nil
	}
	return nil, configstore.ErrNotFound
}

func (m *mockSingleTeamStore) GetTeam(ctx context.Context, id string) (*tables.TableTeam, error) {
	if t, ok := m.teams[id]; ok {
		return t, nil
	}
	return nil, configstore.ErrNotFound
}

func TestValidateVKEntityAssignmentAllowsSharedTeam(t *testing.T) {
	custA := "cust-alpha"
	store := &mockSingleTeamStore{
		customers: map[string]*tables.TableCustomer{
			"cust-alpha": {ID: "cust-alpha", Name: "Customer Alpha"},
			"cust-beta":  {ID: "cust-beta", Name: "Customer Beta"},
		},
		teams: map[string]*tables.TableTeam{
			"team-shared": {ID: "team-shared", Name: "Shared Multi-Customer Team", CustomerID: nil},
			"team-alpha":  {ID: "team-alpha", Name: "Alpha Dedicated Team", CustomerID: &custA},
		},
	}
	h := &GovernanceHandler{configStore: store}

	// 1. Shared team (CustomerID == nil) assigned to Customer Alpha key -> MUST SUCCEED
	err := h.validateVKEntityAssignment(context.Background(), []string{"team-shared"}, []string{"cust-alpha"})
	if err != nil {
		t.Fatalf("expected shared team to be assignable to cust-alpha, got error: %v", err)
	}

	// 2. Shared team assigned to Customer Beta key -> MUST SUCCEED
	err = h.validateVKEntityAssignment(context.Background(), []string{"team-shared"}, []string{"cust-beta"})
	if err != nil {
		t.Fatalf("expected shared team to be assignable to cust-beta, got error: %v", err)
	}

	// 3. Unknown customer -> MUST FAIL
	err = h.validateVKEntityAssignment(context.Background(), []string{"team-shared"}, []string{"nonexistent-cust"})
	if err == nil {
		t.Fatalf("expected unknown customer to fail")
	}

	// 4. Unknown team -> MUST FAIL
	err = h.validateVKEntityAssignment(context.Background(), []string{"nonexistent-team"}, []string{"cust-alpha"})
	if err == nil {
		t.Fatalf("expected unknown team to fail")
	}
}
