package main

import (
	"fmt"
	"strings"

	"github.com/gateway/gateway/scripts/gateway-migration-cli/litellm"
)

// LiteLLMTeamToGatewayTeam transforms a LiteLLM team into a Gateway
// create-team request. The mapping is pure (no I/O); customerID is resolved by
// the caller (team.organization_id -> org alias -> Gateway customer id) and is
// nil for a standalone team or one whose customer could not be resolved.
//
// Mapping (mirrors LiteLLMOrganizationToGatewayCustomer; budget/rate-limit
// fields are inline on the team rather than in a budget table):
//   - team_alias       -> name (required)
//   - max_budget       -> budgets[0].max_limit       (omitted when <= 0)
//   - budget_duration  -> budgets[0].reset_duration  ("mo" -> "M")
//   - tpm_limit        -> rate_limit.token_max_limit   (reset "1m")
//   - rpm_limit        -> rate_limit.request_max_limit (reset "1m")
//   - organization_id  -> customer_id (via the resolved customerID)
func LiteLLMTeamToGatewayTeam(team litellm.LiteLLMTeam, customerID *string, cfg MigrationRunConfig) (*GatewayCreateTeamRequest, error) {
	name := strings.TrimSpace(team.TeamAlias)
	if name == "" {
		return nil, fmt.Errorf("team %q has no team_alias; a team name is required", team.TeamID)
	}

	req := &GatewayCreateTeamRequest{Name: name, CustomerID: customerID}

	// A team's budget/rate-limit fields are inline; adapt them to the same
	// LiteLLMBudget shape the organization migration maps, so the spend-cap and
	// rate-limit rules stay identical across entities.
	b := litellm.LiteLLMBudget{
		MaxBudget:      team.MaxBudget,
		BudgetDuration: team.BudgetDuration,
		TPMLimit:       team.TPMLimit,
		RPMLimit:       team.RPMLimit,
	}

	budget, err := toBudget(b, cfg.MaxBudgetPeriod)
	if err != nil {
		return nil, fmt.Errorf("team %q: %w", team.TeamID, err)
	}
	if budget != nil {
		req.Budgets = []GatewayCreateBudgetRequest{*budget}
	}
	req.RateLimit = toRateLimit(b)

	return req, nil
}
