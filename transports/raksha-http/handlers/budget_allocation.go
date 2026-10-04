package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/raksha/raksha/framework/configstore"
	configstoreTables "github.com/raksha/raksha/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

func budgetLimitsByPeriod(budgets []configstoreTables.TableBudget) map[string]float64 {
	limits := make(map[string]float64, len(budgets))
	for _, b := range budgets {
		limits[b.ResetDuration] += b.MaxLimit
	}
	return limits
}

func requestedLimitsByPeriod(budgets []CreateBudgetRequest) map[string]float64 {
	limits := make(map[string]float64, len(budgets))
	for _, b := range budgets {
		limits[b.ResetDuration] += b.MaxLimit
	}
	return limits
}

// checkCustomerBudgetAllocation rejects a change that would allot the customer's teams more, for
// any reset period, than the customer's own budget for that period. teamLimits replaces the stored
// budgets of teamID (pass teamID "" for a team being created); customerLimits, when non-nil,
// replaces the customer's stored budgets. Periods without a customer budget are unconstrained.
func (h *GovernanceHandler) checkCustomerBudgetAllocation(ctx context.Context, customerID, teamID string, teamLimits, customerLimits map[string]float64) error {
	if customerID == "" || h.configStore == nil {
		return nil
	}
	if customerLimits == nil {
		customer, err := h.configStore.GetCustomer(ctx, customerID)
		if errors.Is(err, configstore.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		customerLimits = budgetLimitsByPeriod(customer.Budgets)
	}
	if len(customerLimits) == 0 {
		return nil
	}
	teams, err := h.configStore.GetTeams(ctx, customerID)
	if err != nil {
		return err
	}
	allotted := make(map[string]float64)
	for _, team := range teams {
		if teamID != "" && team.ID == teamID {
			continue
		}
		for period, limit := range budgetLimitsByPeriod(team.Budgets) {
			allotted[period] += limit
		}
	}
	for period, limit := range teamLimits {
		allotted[period] += limit
	}
	for period, customerLimit := range customerLimits {
		total := allotted[period]
		// Use a small epsilon for float64 equality to absorb summation rounding errors.
		if total <= customerLimit+1e-9 {
			continue
		}
		if teamLimits == nil {
			return &badRequestError{err: fmt.Errorf(
				"teams under this customer already have $%.2f per %s allotted; the customer budget cannot be lower than that",
				total, period)}
		}
		// teamLimits[period] returns 0 when the period is absent, which is correct:
		// a new team that had no budget for this period is contributing $0, so
		// all of `total` came from other teams.
		thisTeamLimit := teamLimits[period]
		otherTeamsTotal := total - thisTeamLimit
		available := customerLimit - otherTeamsTotal
		if available < 0 {
			available = 0
		}
		return &badRequestError{err: fmt.Errorf(
			"team budgets under this customer would total $%.2f per %s, above the customer budget of $%.2f (available for this team: $%.2f)",
			total, period, customerLimit, available)}
	}
	return nil
}

// conflictingCustomerTeam returns the name of another team under customerID that userID already
// belongs to, or "". A user belongs to one team per customer so customer-key usage bills one team.
func (h *GovernanceHandler) conflictingCustomerTeam(ctx context.Context, ws configstore.WorkspaceStore, userID, customerID, teamID string) (string, error) {
	if customerID == "" {
		return "", nil
	}
	links, err := ws.ListTeamsForUser(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, link := range links {
		if link.TeamID == "" || link.TeamID == teamID {
			continue
		}
		other, err := h.configStore.GetTeam(ctx, link.TeamID)
		if err != nil || other == nil {
			continue
		}
		if other.CustomerID != nil && *other.CustomerID == customerID {
			return other.Name, nil
		}
	}
	return "", nil
}

func sendBudgetAllocationError(ctx *fasthttp.RequestCtx, err error) {
	var badReqErr *badRequestError
	if errors.As(err, &badReqErr) {
		SendError(ctx, 400, err.Error())
		return
	}
	logger.Error("failed to check customer budget allocation: %v", err)
	SendError(ctx, 500, "failed to check customer budget allocation")
}
