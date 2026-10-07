package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
)

// ResolveAllowedVirtualKeyIDsForUser resolves all Virtual Key IDs accessible to a user.
// Access is inherited across 3 levels:
// 1. Direct user assignment (governance_virtual_key_users)
// 2. Team assignment for any team the user belongs to (governance_virtual_key_teams or vk.team_id)
// 3. Customer assignment for any customer owning those teams (governance_virtual_key_customers or vk.customer_id)
func ResolveAllowedVirtualKeyIDsForUser(ctx context.Context, store configstore.ConfigStore, userID string) (map[string]bool, error) {
	allowed := make(map[string]bool)
	if store == nil || userID == "" {
		return allowed, nil
	}

	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		return allowed, nil
	}

	// 1. Direct user assignment
	userLinks, err := ws.ListVirtualKeysForUser(ctx, userID)
	if err == nil {
		for _, l := range userLinks {
			if l.VirtualKeyID != "" {
				allowed[l.VirtualKeyID] = true
			}
		}
	}

	// 2. Teams the user belongs to
	teamMemberships, err := ws.ListTeamsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams for user: %w", err)
	}
	var teamIDs []string
	var custIDs []string
	seenTeams := make(map[string]bool)
	seenCustomers := make(map[string]bool)

	for _, tm := range teamMemberships {
		teamID := strings.TrimSpace(tm.TeamID)
		if teamID == "" || seenTeams[teamID] {
			continue
		}
		seenTeams[teamID] = true
		teamIDs = append(teamIDs, teamID)

		// VKs assigned to this team
		teamLinks, err := ws.ListVirtualKeysForTeam(ctx, teamID)
		if err == nil {
			for _, tl := range teamLinks {
				if tl.VirtualKeyID != "" {
					allowed[tl.VirtualKeyID] = true
				}
			}
		}

		// Check Customer owning this team
		team, err := store.GetTeam(ctx, teamID)
		if err == nil && team != nil && team.CustomerID != nil && *team.CustomerID != "" {
			custID := strings.TrimSpace(*team.CustomerID)
			if custID != "" && !seenCustomers[custID] {
				seenCustomers[custID] = true
				custIDs = append(custIDs, custID)

				// VKs assigned to this customer
				custLinks, err := ws.ListVirtualKeysForCustomer(ctx, custID)
				if err == nil {
					for _, cl := range custLinks {
						if cl.VirtualKeyID != "" {
							allowed[cl.VirtualKeyID] = true
						}
					}
				}
			}
		}
	}

	// 3. Check legacy single-column fields (vk.team_id, vk.customer_id) in database
	if store.DB() != nil {
		db := store.DB().WithContext(ctx)
		if len(teamIDs) > 0 {
			var legacyTeamVKs []string
			_ = db.Model(&tables.TableVirtualKey{}).Where("team_id IN ?", teamIDs).Pluck("id", &legacyTeamVKs).Error
			for _, id := range legacyTeamVKs {
				if id != "" {
					allowed[id] = true
				}
			}
		}
		if len(custIDs) > 0 {
			var legacyCustVKs []string
			_ = db.Model(&tables.TableVirtualKey{}).Where("customer_id IN ?", custIDs).Pluck("id", &legacyCustVKs).Error
			for _, id := range legacyCustVKs {
				if id != "" {
					allowed[id] = true
				}
			}
		}

		// 4. Virtual keys created by this user
		var createdVKs []string
		_ = db.Model(&tables.TableVirtualKey{}).Where("created_by_user_id = ?", userID).Pluck("id", &createdVKs).Error
		for _, id := range createdVKs {
			if id != "" {
				allowed[id] = true
			}
		}
	}

	return allowed, nil
}
