package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

const (
	rbacScopeAllTeams     = "all_teams"
	rbacScopeTeam         = "team"
	rbacScopeAllCustomers = "all_customers"
	rbacScopeCustomer     = "customer"
)

// rbacScopeGrant is the permission selection saved for a team, a customer, or every
// team / customer. Members inherit AllowedSections on top of their own sections; the
// API is still gated by each member's role.
type rbacScopeGrant struct {
	PermissionIDs   []uint    `json:"permission_ids"`
	AllowedSections string    `json:"allowed_sections"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type rbacScopeGrants struct {
	AllTeams     *rbacScopeGrant            `json:"all_teams,omitempty"`
	AllCustomers *rbacScopeGrant            `json:"all_customers,omitempty"`
	Teams        map[string]*rbacScopeGrant `json:"teams"`
	Customers    map[string]*rbacScopeGrant `json:"customers"`
}

func (g *rbacScopeGrants) isEmpty() bool {
	return g.AllTeams == nil && g.AllCustomers == nil && len(g.Teams) == 0 && len(g.Customers) == 0
}

// Serializes read-modify-write of the single grants row.
var rbacScopeGrantsMu sync.Mutex

var sectionKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9/_-]{0,127}$`)

func loadRBACScopeGrants(ctx context.Context, ws configstore.WorkspaceStore) (*rbacScopeGrants, error) {
	out := &rbacScopeGrants{Teams: map[string]*rbacScopeGrant{}, Customers: map[string]*rbacScopeGrant{}}
	row, err := ws.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingRBACScopeGrants)
	if errors.Is(err, configstore.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if row == nil || strings.TrimSpace(row.Data) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(row.Data), out); err != nil {
		return nil, err
	}
	if out.Teams == nil {
		out.Teams = map[string]*rbacScopeGrant{}
	}
	if out.Customers == nil {
		out.Customers = map[string]*rbacScopeGrant{}
	}
	return out, nil
}

func saveRBACScopeGrants(ctx context.Context, ws configstore.WorkspaceStore, grants *rbacScopeGrants) error {
	raw, err := json.Marshal(grants)
	if err != nil {
		return err
	}
	return ws.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingRBACScopeGrants, string(raw))
}

// mergeSectionLists unions comma-separated section lists, keeping first-seen order.
func mergeSectionLists(lists ...string) string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, list := range lists {
		for _, part := range strings.Split(list, ",") {
			key := strings.TrimSpace(part)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}
	return strings.Join(out, ",")
}

func sanitizeSectionList(list string) (string, bool) {
	merged := mergeSectionLists(strings.ToLower(list))
	if merged == "" {
		return "", true
	}
	for _, key := range strings.Split(merged, ",") {
		if !sectionKeyPattern.MatchString(key) {
			return "", false
		}
	}
	return merged, true
}

// effectiveAllowedSections is the user's own allowed_sections plus the sections granted
// to their teams, those teams' customers, and the all-teams / all-customers policies.
// Admin and the built-in "user" role are not section-scoped, so they are returned as-is.
func effectiveAllowedSections(ctx context.Context, store configstore.ConfigStore, user *tables.TableUser) string {
	if user == nil {
		return ""
	}
	own := user.AllowedSections
	role := strings.ToLower(strings.TrimSpace(user.Role))
	if role == "" || role == "admin" || role == "user" {
		return own
	}
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		return own
	}
	grants, err := loadRBACScopeGrants(ctx, ws)
	if err != nil || grants.isEmpty() {
		return own
	}
	links, err := ws.ListTeamsForUser(ctx, user.ID)
	if err != nil || len(links) == 0 {
		return own
	}
	lists := []string{own}
	inTeam, inCustomer := false, false
	for _, link := range links {
		team, err := store.GetTeam(ctx, link.TeamID)
		if err != nil || team == nil {
			continue
		}
		inTeam = true
		if g := grants.Teams[team.ID]; g != nil {
			lists = append(lists, g.AllowedSections)
		}
		if team.CustomerID == nil || *team.CustomerID == "" {
			continue
		}
		inCustomer = true
		if g := grants.Customers[*team.CustomerID]; g != nil {
			lists = append(lists, g.AllowedSections)
		}
	}
	if inTeam && grants.AllTeams != nil {
		lists = append(lists, grants.AllTeams.AllowedSections)
	}
	if inCustomer && grants.AllCustomers != nil {
		lists = append(lists, grants.AllCustomers.AllowedSections)
	}
	return mergeSectionLists(lists...)
}

// removeRBACScopeGrant drops a deleted team's or customer's grant. Best effort: a
// leftover grant is never applied because nobody can be a member of a deleted scope.
func removeRBACScopeGrant(ctx context.Context, store configstore.ConfigStore, scopeType, scopeID string) {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil || scopeID == "" {
		return
	}
	rbacScopeGrantsMu.Lock()
	defer rbacScopeGrantsMu.Unlock()
	grants, err := loadRBACScopeGrants(ctx, ws)
	if err != nil {
		return
	}
	switch scopeType {
	case rbacScopeTeam:
		if _, ok := grants.Teams[scopeID]; !ok {
			return
		}
		delete(grants.Teams, scopeID)
	case rbacScopeCustomer:
		if _, ok := grants.Customers[scopeID]; !ok {
			return
		}
		delete(grants.Customers, scopeID)
	default:
		return
	}
	_ = saveRBACScopeGrants(ctx, ws, grants)
}

// getRBACScopeGrants handles GET /api/rbac/scope-grants.
func (h *WorkspaceHandler) getRBACScopeGrants(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	grants, err := loadRBACScopeGrants(ctx, store)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load scope grants")
		return
	}
	SendJSON(ctx, grants)
}

// updateRBACScopeGrant handles PUT /api/rbac/scope-grants. An empty permission list
// removes the grant.
func (h *WorkspaceHandler) updateRBACScopeGrant(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var body struct {
		ScopeType       string `json:"scope_type"`
		ScopeID         string `json:"scope_id"`
		PermissionIDs   []uint `json:"permission_ids"`
		AllowedSections string `json:"allowed_sections"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	body.ScopeID = strings.TrimSpace(body.ScopeID)
	switch body.ScopeType {
	case rbacScopeAllTeams, rbacScopeAllCustomers:
		body.ScopeID = ""
	case rbacScopeTeam, rbacScopeCustomer:
		if body.ScopeID == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "scope_id is required")
			return
		}
	default:
		SendError(ctx, fasthttp.StatusBadRequest, "scope_type must be one of all_teams, team, all_customers, customer")
		return
	}
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	switch body.ScopeType {
	case rbacScopeTeam:
		if team, err := h.store.ConfigStore.GetTeam(ctx, body.ScopeID); err != nil || team == nil {
			SendError(ctx, fasthttp.StatusNotFound, "team not found")
			return
		}
	case rbacScopeCustomer:
		if customer, err := h.store.ConfigStore.GetCustomer(ctx, body.ScopeID); err != nil || customer == nil {
			SendError(ctx, fasthttp.StatusNotFound, "customer not found")
			return
		}
	}

	known := map[uint]bool{}
	for _, perm := range configstore.RBACPermissions() {
		known[perm.ID] = true
	}
	ids := make([]uint, 0, len(body.PermissionIDs))
	seen := map[uint]bool{}
	for _, id := range body.PermissionIDs {
		if known[id] && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sections, ok := sanitizeSectionList(body.AllowedSections)
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "allowed_sections contains an invalid section key")
		return
	}

	var grant *rbacScopeGrant
	if len(ids) > 0 {
		grant = &rbacScopeGrant{PermissionIDs: ids, AllowedSections: sections, UpdatedAt: time.Now().UTC()}
	}

	rbacScopeGrantsMu.Lock()
	defer rbacScopeGrantsMu.Unlock()
	grants, err := loadRBACScopeGrants(ctx, store)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load scope grants")
		return
	}
	switch body.ScopeType {
	case rbacScopeAllTeams:
		grants.AllTeams = grant
	case rbacScopeAllCustomers:
		grants.AllCustomers = grant
	case rbacScopeTeam:
		if grant == nil {
			delete(grants.Teams, body.ScopeID)
		} else {
			grants.Teams[body.ScopeID] = grant
		}
	case rbacScopeCustomer:
		if grant == nil {
			delete(grants.Customers, body.ScopeID)
		} else {
			grants.Customers[body.ScopeID] = grant
		}
	}
	if err := saveRBACScopeGrants(ctx, store, grants); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save scope grant")
		return
	}
	SendJSON(ctx, grants)
}
