package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
)

type allocationStore struct {
	configstore.ConfigStore
	customer *tables.TableCustomer
	teams    []tables.TableTeam
}

func (s *allocationStore) GetCustomer(ctx context.Context, id string) (*tables.TableCustomer, error) {
	if s.customer == nil || s.customer.ID != id {
		return nil, configstore.ErrNotFound
	}
	return s.customer, nil
}

func (s *allocationStore) GetTeams(ctx context.Context, customerID string) ([]tables.TableTeam, error) {
	return s.teams, nil
}

func (s *allocationStore) GetTeam(ctx context.Context, id string) (*tables.TableTeam, error) {
	for i := range s.teams {
		if s.teams[i].ID == id {
			return &s.teams[i], nil
		}
	}
	return nil, configstore.ErrNotFound
}

type userTeamsWS struct {
	configstore.WorkspaceStore
	links []tables.TableTeamMember
}

func (w *userTeamsWS) ListTeamsForUser(ctx context.Context, userID string) ([]tables.TableTeamMember, error) {
	return w.links, nil
}

func TestUserMayBeInOnlyOneTeamPerCustomer(t *testing.T) {
	bank := "bank"
	h := &GovernanceHandler{configStore: &allocationStore{teams: []tables.TableTeam{
		{ID: "dev", Name: "Developers", CustomerID: &bank},
		{ID: "qa", Name: "QA", CustomerID: &bank},
		{ID: "solo", Name: "Solo"},
	}}}
	ws := &userTeamsWS{links: []tables.TableTeamMember{{TeamID: "dev", UserID: "ravi"}, {TeamID: "solo", UserID: "ravi"}}}

	if other, err := h.conflictingCustomerTeam(context.Background(), ws, "ravi", bank, "qa"); err != nil || other != "Developers" {
		t.Fatalf("adding ravi to QA under the same customer: conflict = %q, %v; want Developers", other, err)
	}
	if other, _ := h.conflictingCustomerTeam(context.Background(), ws, "ravi", bank, "dev"); other != "" {
		t.Fatalf("re-adding ravi to his own team conflicts with %q", other)
	}
	if other, _ := h.conflictingCustomerTeam(context.Background(), ws, "ravi", "other-customer", "x"); other != "" {
		t.Fatalf("a team under another customer conflicts with %q", other)
	}
}

func monthly(limit float64) []tables.TableBudget {
	return []tables.TableBudget{{MaxLimit: limit, ResetDuration: "1M"}}
}

func newAllocationHandler() *GovernanceHandler {
	return &GovernanceHandler{configStore: &allocationStore{
		customer: &tables.TableCustomer{ID: "cust", Budgets: monthly(1000)},
		teams: []tables.TableTeam{
			{ID: "t1", Budgets: monthly(200)},
			{ID: "t2", Budgets: monthly(300)},
			{ID: "t3", Budgets: monthly(200)},
		},
	}}
}

func TestTeamBudgetsMayFillCustomerBudget(t *testing.T) {
	h := newAllocationHandler()
	if err := h.checkCustomerBudgetAllocation(context.Background(), "cust", "", map[string]float64{"1M": 300}, nil); err != nil {
		t.Fatalf("200+300+200+300 = 1000 should fit the customer budget: %v", err)
	}
}

func TestTeamBudgetsCannotExceedCustomerBudget(t *testing.T) {
	h := newAllocationHandler()
	err := h.checkCustomerBudgetAllocation(context.Background(), "cust", "", map[string]float64{"1M": 301}, nil)
	if err == nil || !strings.Contains(err.Error(), "available for this team: $300.00") {
		t.Fatalf("expected over-allocation error with $300 available, got %v", err)
	}
}

func TestEditedTeamReplacesItsOwnBudget(t *testing.T) {
	h := newAllocationHandler()
	if err := h.checkCustomerBudgetAllocation(context.Background(), "cust", "t2", map[string]float64{"1M": 600}, nil); err != nil {
		t.Fatalf("t2 raised to 600 gives 200+600+200 = 1000: %v", err)
	}
}

func TestCustomerBudgetCannotDropBelowTeams(t *testing.T) {
	h := newAllocationHandler()
	if err := h.checkCustomerBudgetAllocation(context.Background(), "cust", "", nil, map[string]float64{"1M": 699}); err == nil {
		t.Fatal("customer budget 699 is below the 700 already allotted to teams")
	}
	if err := h.checkCustomerBudgetAllocation(context.Background(), "cust", "", nil, map[string]float64{"1M": 700}); err != nil {
		t.Fatalf("customer budget 700 covers the teams: %v", err)
	}
}

func TestOtherPeriodsAndStandaloneTeamsAreUnconstrained(t *testing.T) {
	h := newAllocationHandler()
	if err := h.checkCustomerBudgetAllocation(context.Background(), "cust", "", map[string]float64{"1d": 5000}, nil); err != nil {
		t.Fatalf("a daily team budget has no daily customer budget to fit: %v", err)
	}
	if err := h.checkCustomerBudgetAllocation(context.Background(), "", "", map[string]float64{"1M": 5000}, nil); err != nil {
		t.Fatalf("a team without a customer can have any budget: %v", err)
	}
}
