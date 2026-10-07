package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/encrypt"
	"github.com/valyala/fasthttp"
)

func (h *WorkspaceHandler) scimWorkspaceStore() configstore.WorkspaceStore {
	if h.workspace != nil {
		return h.workspace
	}
	if h.store != nil && h.store.ConfigStore != nil {
		if ws, ok := configstore.AsWorkspaceStore(h.store.ConfigStore); ok && ws != nil {
			return ws
		}
	}
	return nil
}

func (h *WorkspaceHandler) scimDefaultRole(ctx *fasthttp.RequestCtx) string {
	store := h.scimWorkspaceStore()
	if store == nil {
		return ""
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM)
	if err != nil || row == nil || strings.TrimSpace(row.Data) == "" {
		return ""
	}
	var cfg scimConfigPayload
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil || cfg.Config == nil {
		return ""
	}
	for _, key := range []string{"defaultRole", "default_role"} {
		if raw, ok := cfg.Config[key]; ok {
			switch v := raw.(type) {
			case string:
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func (h *WorkspaceHandler) scimDefaultTeam(ctx *fasthttp.RequestCtx) string {
	store := h.scimWorkspaceStore()
	if store == nil {
		return ""
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM)
	if err != nil || row == nil || strings.TrimSpace(row.Data) == "" {
		return ""
	}
	var cfg scimConfigPayload
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil || cfg.Config == nil {
		return ""
	}
	for _, key := range []string{"defaultTeam", "default_team", "defaultTeamID", "default_team_id"} {
		if raw, ok := cfg.Config[key]; ok {
			switch v := raw.(type) {
			case string:
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// scimAssignDefaultTeam adds the user to the default SCIM team (if configured)
// and fires the lifecycle hook so audit logs and budget limits are applied.
func (h *WorkspaceHandler) scimAssignDefaultTeam(ctx *fasthttp.RequestCtx, userID string) {
	teamID := h.scimDefaultTeam(ctx)
	if teamID == "" {
		return
	}
	ws := h.scimWorkspaceStore()
	if ws == nil {
		return
	}
	// Skip if already a member
	if members, err := ws.ListTeamMembers(ctx, teamID); err == nil {
		for _, m := range members {
			if m.UserID == userID {
				return
			}
		}
	}
	if err := ws.AddTeamMember(ctx, teamID, userID); err == nil && h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnTeamMemberAdded(ctx, teamID, userID)
	}
}

func (h *WorkspaceHandler) scimServiceProviderConfig(ctx *fasthttp.RequestCtx) {
	SendSCIMJSON(ctx, map[string]any{
		"schemas":        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":          map[string]any{"supported": true},
		"bulk":           map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":         map[string]any{"supported": true, "maxResults": 200},
		"changePassword": map[string]any{"supported": false},
		"sort":           map[string]any{"supported": false},
		"etag":           map[string]any{"supported": false},
		"authenticationSchemes": []map[string]any{{
			"type":        "oauthbearertoken",
			"name":        "OAuth Bearer Token",
			"description": "Authentication scheme using the bearer token configured in SCIM settings",
		}},
	})
}

func (h *WorkspaceHandler) scimResourceTypes(ctx *fasthttp.RequestCtx) {
	SendSCIMJSON(ctx, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": 2,
		"itemsPerPage": 2,
		"startIndex":   1,
		"Resources": []map[string]any{
			{
				"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"},
				"id":          "User",
				"name":        "User",
				"endpoint":    "/Users",
				"description": "User Account",
				"schema":      "urn:ietf:params:scim:schemas:core:2.0:User",
				"meta":        map[string]any{"resourceType": "ResourceType", "location": "/scim/v2/ResourceTypes/User"},
			},
			{
				"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"},
				"id":          "Group",
				"name":        "Group",
				"endpoint":    "/Groups",
				"description": "Group / Team",
				"schema":      "urn:ietf:params:scim:schemas:core:2.0:Group",
				"meta":        map[string]any{"resourceType": "ResourceType", "location": "/scim/v2/ResourceTypes/Group"},
			},
		},
	})
}

func (h *WorkspaceHandler) scimSchemas(ctx *fasthttp.RequestCtx) {
	SendSCIMJSON(ctx, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": 2,
		"itemsPerPage": 2,
		"startIndex":   1,
		"Resources": []map[string]any{
			{
				"id":          "urn:ietf:params:scim:schemas:core:2.0:User",
				"name":        "User",
				"description": "User Account",
				"attributes": []map[string]any{
					{"name": "userName", "type": "string", "required": true},
					{"name": "displayName", "type": "string"},
					{"name": "active", "type": "boolean"},
				},
			},
			{
				"id":          "urn:ietf:params:scim:schemas:core:2.0:Group",
				"name":        "Group",
				"description": "Group / Team",
				"attributes": []map[string]any{
					{"name": "displayName", "type": "string", "required": true},
					{"name": "members", "type": "complex", "multiValued": true},
				},
			},
		},
	})
}

func (h *WorkspaceHandler) scimListUsers(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	users, err := h.store.ConfigStore.GetUsers(ctx)
	if err != nil {
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to list users")
		return
	}
	filter := string(ctx.QueryArgs().Peek("filter"))
	if filter != "" {
		filtered, ok := scimFilterUsers(users, filter)
		if !ok {
			scimError(ctx, fasthttp.StatusBadRequest, "unsupported filter expression", "invalidFilter")
			return
		}
		users = filtered
	}
	// startIndex paging needs a stable order.
	sort.SliceStable(users, func(i, j int) bool {
		if !users[i].CreatedAt.Equal(users[j].CreatedAt) {
			return users[i].CreatedAt.Before(users[j].CreatedAt)
		}
		return users[i].ID < users[j].ID
	})
	startIndex := queryInt(ctx, "startIndex", 1)
	if startIndex < 1 {
		startIndex = 1
	}
	count := queryInt(ctx, "count", 100)
	if count <= 0 {
		count = 100
	}
	total := len(users)
	offset := startIndex - 1
	if offset > total {
		offset = total
	}
	end := offset + count
	if end > total {
		end = total
	}
	page := users[offset:end]
	resources := make([]map[string]any, 0, len(page))
	for _, user := range page {
		resources = append(resources, scimUserResource(user))
	}
	SendSCIMJSON(ctx, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": total,
		"itemsPerPage": len(resources),
		"startIndex":   startIndex,
		"Resources":    resources,
	})
}

var scimEmailFilterRegex = regexp.MustCompile(`(?i)emails(?:\[[^\]]+\])?\.value\s+eq\s+["']?([^"']+)["']?`)

// scimFilterUsers applies supported SCIM eq filters. ok=false means the filter syntax is
// unsupported (IdPs must get 400 invalidFilter, not an empty 200 list).
func scimFilterUsers(users []*tables.TableUser, filter string) ([]*tables.TableUser, bool) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return users, true
	}
	normalized := filter
	const userSchemaPrefix = "urn:ietf:params:scim:schemas:core:2.0:user:"
	if strings.HasPrefix(strings.ToLower(normalized), userSchemaPrefix) {
		normalized = normalized[len(userSchemaPrefix):]
	}

	if m := scimEmailFilterRegex.FindStringSubmatch(normalized); len(m) > 1 {
		raw := strings.Trim(strings.TrimSpace(m[1]), `"'`)
		out := make([]*tables.TableUser, 0)
		for _, user := range users {
			if strings.EqualFold(user.Email, raw) || strings.EqualFold(user.Username, raw) {
				out = append(out, user)
			}
		}
		return out, true
	}

	normLower := strings.ToLower(strings.TrimSpace(normalized))
	for _, prefix := range []string{"username eq ", "externalid eq ", "id eq ", "email eq ", "emails.value eq ", "displayname eq "} {
		if strings.HasPrefix(normLower, prefix) {
			raw := strings.TrimSpace(normalized[len(prefix):])
			raw = strings.Trim(raw, `"'`)
			out := make([]*tables.TableUser, 0)
			for _, user := range users {
				switch prefix {
				case "username eq ":
					if strings.EqualFold(user.Username, raw) || strings.EqualFold(user.Email, raw) {
						out = append(out, user)
					}
				case "displayname eq ":
					if strings.EqualFold(user.Username, raw) {
						out = append(out, user)
					}
				case "externalid eq ":
					if user.ExternalID != "" && strings.EqualFold(user.ExternalID, raw) {
						out = append(out, user)
					}
				case "id eq ":
					if strings.EqualFold(user.ID, raw) {
						out = append(out, user)
					}
				case "email eq ", "emails.value eq ":
					if strings.EqualFold(user.Email, raw) || strings.EqualFold(user.Username, raw) {
						out = append(out, user)
					}
				}
			}
			return out, true
		}
	}
	return nil, false
}

func (h *WorkspaceHandler) scimGetUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		scimError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	SendSCIMJSON(ctx, scimUserResource(user))
}

func scimNormalizeRole(raw string) string {
	cleaned := strings.ToLower(strings.TrimSpace(raw))
	switch cleaned {
	case "admin", "administrator":
		return "admin"
	case "sub_admin", "sub-admin", "sub admin", "subadmin", "manager", "workspace_manager":
		return "sub_admin"
	case "user", "member":
		return "user"
	default:
		return cleaned
	}
}

func (h *WorkspaceHandler) scimCreateUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	var body struct {
		UserName    string `json:"userName"`
		DisplayName string `json:"displayName"`
		ExternalID  string `json:"externalId"`
		Emails      []struct {
			Value   string `json:"value"`
			Primary bool   `json:"primary"`
		} `json:"emails"`
		// Pointer: an omitted "active" means active (RFC 7643 default), not disabled.
		Active *bool `json:"active"`
		Roles  []struct {
			Value string `json:"value"`
		} `json:"roles"`
		Name struct {
			Formatted  string `json:"formatted"`
			GivenName  string `json:"givenName"`
			FamilyName string `json:"familyName"`
		} `json:"name"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		scimError(ctx, fasthttp.StatusBadRequest, "invalid scim payload", "invalidSyntax")
		return
	}
	rawUsername := strings.TrimSpace(body.UserName)
	email := strings.ToLower(scimEmailFromBody(body.Emails))
	externalID := strings.TrimSpace(body.ExternalID)
	if email == "" && strings.Contains(rawUsername, "@") {
		email = strings.ToLower(rawUsername)
	}

	displayName := strings.TrimSpace(body.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(body.Name.Formatted)
	}
	if displayName == "" && (body.Name.GivenName != "" || body.Name.FamilyName != "") {
		displayName = strings.TrimSpace(strings.TrimSpace(body.Name.GivenName) + " " + strings.TrimSpace(body.Name.FamilyName))
	}

	preferredName := displayName
	if preferredName == "" {
		preferredName = rawUsername
	}
	if preferredName == "" && email != "" {
		preferredName = email
	}
	if preferredName == "" {
		scimError(ctx, fasthttp.StatusBadRequest, "userName or displayName is required", "invalidValue")
		return
	}
	if isBuiltinAdminIdentity(ctx, h.store.ConfigStore, preferredName, email, rawUsername) {
		scimError(ctx, fasthttp.StatusConflict, "userName or email is reserved for the built-in admin account", "uniqueness")
		return
	}

	requestedRole := ""
	if len(body.Roles) > 0 {
		requestedRole = strings.TrimSpace(body.Roles[0].Value)
	}

	// Link by stable IdP identity (externalId / email / UPN) — never by displayName alone,
	// or two "John Smith" accounts would overwrite each other.
	if existing := h.scimFindUser(ctx, email, externalID, rawUsername); existing != nil {
		if externalID != "" && existing.ExternalID != "" && !strings.EqualFold(existing.ExternalID, externalID) {
			scimError(ctx, fasthttp.StatusConflict, "a user with this userName or email is linked to a different externalId", "uniqueness")
			return
		}
		prev := *existing
		if externalID != "" {
			existing.ExternalID = externalID
		}
		if body.Active != nil {
			existing.Status = scimStatusFromActive(*body.Active)
		}
		if requestedRole != "" {
			existing.Role = h.scimValidRole(ctx, requestedRole, existing.Role)
		}
		existing.Username = h.scimAllocateUsername(ctx, preferredName, rawUsername, email, existing.ID)
		if email != "" {
			existing.Email = email
		}
		existing.UpdatedAt = time.Now().UTC()
		if err := h.store.ConfigStore.UpdateUser(ctx, existing); err != nil {
			if scimIsUniqueViolation(err) {
				scimError(ctx, fasthttp.StatusConflict, "userName or email is already used by another user", "uniqueness")
				return
			}
			scimError(ctx, fasthttp.StatusInternalServerError, "failed to update user")
			return
		}
		h.scimAfterUserChange(ctx, &prev, existing)
		if existing.IsApproved() {
			h.scimAssignDefaultTeam(ctx, existing.ID)
		}
		SendSCIMJSONWithStatus(ctx, scimUserResource(existing), fasthttp.StatusOK)
		return
	}

	username := h.scimAllocateUsername(ctx, preferredName, rawUsername, email, "")
	role := h.scimValidRole(ctx, requestedRole, "")
	// SCIM users sign in through the IdP / password reset; store a random hashed secret so the
	// column never holds a usable plain-text password.
	secret, err := encrypt.Hash(uuid.NewString())
	if err != nil {
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to create user")
		return
	}
	active := body.Active == nil || *body.Active
	now := time.Now().UTC()
	user := &tables.TableUser{
		ID:         uuid.NewString(),
		Username:   username,
		Email:      email,
		Password:   secret,
		Role:       role,
		Status:     scimStatusFromActive(active),
		ExternalID: externalID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.store.ConfigStore.CreateUser(ctx, user); err != nil {
		if scimIsUniqueViolation(err) {
			scimError(ctx, fasthttp.StatusConflict, "userName or email is already used by another user", "uniqueness")
			return
		}
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to create user")
		return
	}
	if h.promptLifecycle != nil && user.IsApproved() {
		_ = h.promptLifecycle.OnUserCreated(ctx, user, true)
	}
	if user.IsApproved() {
		h.scimAssignDefaultTeam(ctx, user.ID)
		trySendProvisionedEmail(h.store.ConfigStore, ctx, user.Username, user.Email)
	}
	ctx.Response.Header.Set("Location", "/scim/v2/Users/"+user.ID)
	SendSCIMJSONWithStatus(ctx, scimUserResource(user), fasthttp.StatusCreated)
}

func scimIsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "constraint")
}

func (h *WorkspaceHandler) scimUsernameTaken(ctx context.Context, username, excludeID string) bool {
	if strings.TrimSpace(username) == "" || h.store == nil || h.store.ConfigStore == nil {
		return false
	}
	users, err := h.store.ConfigStore.GetUsers(ctx)
	if err != nil {
		return false
	}
	for _, u := range users {
		if u == nil || (excludeID != "" && u.ID == excludeID) {
			continue
		}
		if strings.EqualFold(u.Username, username) {
			return true
		}
	}
	return false
}

// scimAllocateUsername prefers displayName (human-readable), then falls back to email/UPN
// or a numeric suffix when the name is already taken.
func (h *WorkspaceHandler) scimAllocateUsername(ctx context.Context, preferred, rawUsername, email, excludeID string) string {
	base := strings.TrimSpace(preferred)
	if base == "" {
		base = strings.TrimSpace(rawUsername)
	}
	if base == "" {
		base = strings.TrimSpace(email)
	}
	if base == "" {
		return uuid.NewString()
	}
	if !h.scimUsernameTaken(ctx, base, excludeID) {
		return base
	}
	candidates := make([]string, 0, 6)
	if email != "" && !strings.EqualFold(email, base) {
		local := strings.Split(email, "@")[0]
		if local != "" {
			// Keep a human-readable display name; fall back to email/UPN last.
			candidates = append(candidates, base+" ("+local+")", base+"-"+local)
		}
		candidates = append(candidates, email)
	}
	if rawUsername != "" && !strings.EqualFold(rawUsername, base) {
		candidates = append(candidates, rawUsername)
	}
	for _, c := range candidates {
		if c != "" && !h.scimUsernameTaken(ctx, c, excludeID) {
			return c
		}
	}
	for i := 2; i < 1000; i++ {
		c := fmt.Sprintf("%s-%d", base, i)
		if !h.scimUsernameTaken(ctx, c, excludeID) {
			return c
		}
	}
	return base + "-" + uuid.NewString()[:8]
}

// scimFindUser matches by stable IdP identity: externalId, email, then UPN (raw userName).
// Display names are never used for lookup — they are not unique across people.
func (h *WorkspaceHandler) scimFindUser(ctx context.Context, email, externalID, rawUserName string) *tables.TableUser {
	users, err := h.store.ConfigStore.GetUsers(ctx)
	if err != nil {
		return nil
	}
	if externalID != "" {
		for _, u := range users {
			if u != nil && u.ExternalID != "" && strings.EqualFold(u.ExternalID, externalID) {
				return u
			}
		}
	}
	if email != "" {
		for _, u := range users {
			if u != nil && u.Email != "" && strings.EqualFold(u.Email, email) {
				return u
			}
		}
	}
	if rawUserName != "" {
		for _, u := range users {
			if u == nil {
				continue
			}
			if strings.EqualFold(u.Username, rawUserName) || (u.Email != "" && strings.EqualFold(u.Email, rawUserName)) {
				return u
			}
		}
	}
	return nil
}

// scimValidRole maps an IdP role value onto an existing workspace role. Unknown values
// (e.g. Entra's "msiam_access") fall back to the current role, then the configured default,
// then "user".
func (h *WorkspaceHandler) scimValidRole(ctx *fasthttp.RequestCtx, requested, current string) string {
	known := map[string]string{"admin": "admin", "user": "user"}
	if ws := h.scimWorkspaceStore(); ws != nil {
		_ = ws.EnsureRBACRoles(ctx)
		if roles, err := ws.ListRBACRoles(ctx); err == nil {
			for _, r := range roles {
				known[strings.ToLower(r.Name)] = r.Name
			}
		}
	}
	for _, candidate := range []string{requested, current, h.scimDefaultRole(ctx)} {
		if candidate == "" {
			continue
		}
		// An exact role name wins over an alias, so a custom role called "manager" is not
		// escalated to sub_admin.
		if name, ok := known[strings.ToLower(strings.TrimSpace(candidate))]; ok {
			return name
		}
		if name, ok := known[scimNormalizeRole(candidate)]; ok {
			return name
		}
	}
	return "user"
}

// scimUniquenessConflict reports whether another user already owns the username or email.
func (h *WorkspaceHandler) scimUniquenessConflict(ctx context.Context, user *tables.TableUser) bool {
	users, err := h.store.ConfigStore.GetUsers(ctx)
	if err != nil {
		return false
	}
	for _, u := range users {
		if u == nil || u.ID == user.ID {
			continue
		}
		if strings.EqualFold(u.Username, user.Username) || (user.Email != "" && strings.EqualFold(u.Email, user.Email)) {
			return true
		}
	}
	return false
}

// scimAfterUserChange keeps sessions, prompt access and the prompt repository in step with
// a SCIM update: deactivation signs the user out, role changes apply to live sessions, and
// reactivation restores prompt access.
func (h *WorkspaceHandler) scimAfterUserChange(ctx *fasthttp.RequestCtx, prev, user *tables.TableUser) {
	cs := h.store.ConfigStore
	wasActive, isActive := prev.IsApproved(), user.IsApproved()
	if wasActive && !isActive {
		_ = cs.DeleteSessionsByUsername(ctx, prev.Username)
		if user.Username != prev.Username {
			_ = cs.DeleteSessionsByUsername(ctx, user.Username)
		}
	} else if isActive && !strings.EqualFold(prev.Role, user.Role) {
		_ = cs.UpdateSessionsRoleByUsername(ctx, prev.Username, user.Role)
		if user.Username != prev.Username {
			_ = cs.UpdateSessionsRoleByUsername(ctx, user.Username, user.Role)
		}
	}
	if h.promptLifecycle == nil {
		return
	}
	_ = h.promptLifecycle.OnUserUpdated(ctx, user, prev.Email, prev.Username)
	if !wasActive && isActive {
		_ = h.promptLifecycle.OnUserCreated(ctx, user, true)
	}
}

func (h *WorkspaceHandler) scimPutUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		scimError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		scimError(ctx, fasthttp.StatusBadRequest, "invalid scim payload", "invalidSyntax")
		return
	}
	prev := *user
	applySCIMUserPatch(user, body)
	h.scimSaveUser(ctx, &prev, user)
}

func (h *WorkspaceHandler) scimPatchUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		scimError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	var body struct {
		Operations []struct {
			Op    string `json:"op"`
			Path  string `json:"path"`
			Value any    `json:"value"`
		} `json:"Operations"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		scimError(ctx, fasthttp.StatusBadRequest, "invalid scim patch payload", "invalidSyntax")
		return
	}
	prev := *user
	for _, op := range body.Operations {
		opName := strings.ToLower(strings.TrimSpace(op.Op))
		switch opName {
		case "replace", "add":
			patch := map[string]any{}
			if op.Path != "" {
				patch[op.Path] = op.Value
			} else if m, ok := op.Value.(map[string]any); ok {
				patch = m
			}
			applySCIMUserPatch(user, patch)
		case "remove":
			if strings.EqualFold(strings.TrimSpace(op.Path), "externalId") {
				user.ExternalID = ""
			}
		}
	}
	h.scimSaveUser(ctx, &prev, user)
}

func (h *WorkspaceHandler) scimSaveUser(ctx *fasthttp.RequestCtx, prev, user *tables.TableUser) {
	if strings.TrimSpace(user.Username) == "" {
		scimError(ctx, fasthttp.StatusBadRequest, "userName cannot be empty", "invalidValue")
		return
	}
	if !strings.EqualFold(prev.Role, user.Role) {
		user.Role = h.scimValidRole(ctx, user.Role, prev.Role)
	}
	identityChanged := !strings.EqualFold(prev.Username, user.Username) || !strings.EqualFold(prev.Email, user.Email)
	if identityChanged && isBuiltinAdminIdentity(ctx, h.store.ConfigStore, user.Username, user.Email) {
		scimError(ctx, fasthttp.StatusConflict, "userName or email is reserved for the built-in admin account", "uniqueness")
		return
	}
	if identityChanged && h.scimUniquenessConflict(ctx, user) {
		// Display-name collisions are common across IdPs; uniquify before failing sync.
		if !strings.EqualFold(prev.Username, user.Username) {
			user.Username = h.scimAllocateUsername(ctx, user.Username, "", user.Email, user.ID)
		}
		if h.scimUniquenessConflict(ctx, user) {
			scimError(ctx, fasthttp.StatusConflict, "userName or email is already used by another user", "uniqueness")
			return
		}
	}
	user.UpdatedAt = time.Now().UTC()
	if err := h.store.ConfigStore.UpdateUser(ctx, user); err != nil {
		if scimIsUniqueViolation(err) {
			scimError(ctx, fasthttp.StatusConflict, "userName or email is already used by another user", "uniqueness")
			return
		}
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to update user")
		return
	}
	h.scimAfterUserChange(ctx, prev, user)
	SendSCIMJSON(ctx, scimUserResource(user))
}

func (h *WorkspaceHandler) scimDeleteUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		scimError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	purgeUserRelations(ctx, h.store.ConfigStore, h.promptLifecycle, user)
	if err := h.store.ConfigStore.DeleteUser(ctx, id); err != nil {
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to delete user")
		return
	}
	_ = h.store.ConfigStore.DeleteSessionsByUsername(ctx, user.Username)
	if syncer, ok := h.governanceManager.(UserGovernanceSyncer); ok && syncer != nil {
		syncer.DeleteUserGovernance(ctx, id)
	}
	if evicter, ok := h.governanceManager.(userModelConfigEvicter); ok && evicter != nil {
		evicter.DeleteUserModelConfigs(ctx, id)
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

func applySCIMUserPatch(user *tables.TableUser, patch map[string]any) {
	if user == nil || len(patch) == 0 {
		return
	}
	for k, v := range patch {
		cleanKey := strings.ToLower(strings.TrimSpace(k))
		const userSchema = "urn:ietf:params:scim:schemas:core:2.0:user:"
		if strings.HasPrefix(cleanKey, userSchema) {
			cleanKey = cleanKey[len(userSchema):]
		}
		switch cleanKey {
		case "displayname":
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				user.Username = strings.TrimSpace(s)
			}
		case "name.formatted":
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				user.Username = strings.TrimSpace(s)
			}
		case "name":
			if m, ok := v.(map[string]any); ok {
				if formatted, ok := m["formatted"].(string); ok && strings.TrimSpace(formatted) != "" {
					user.Username = strings.TrimSpace(formatted)
				}
			}
		case "username":
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				s = strings.TrimSpace(s)
				if strings.Contains(s, "@") {
					if user.Email == "" {
						user.Email = strings.ToLower(s)
					}
				} else if user.Username == "" {
					user.Username = s
				}
			}
		case "active":
			switch val := v.(type) {
			case bool:
				user.Status = scimStatusFromActive(val)
			case string:
				user.Status = scimStatusFromActive(strings.EqualFold(val, "true") || val == "1")
			}
		case "externalid":
			if s, ok := v.(string); ok {
				user.ExternalID = s
			}
		case "roles":
			if roles, ok := v.([]any); ok && len(roles) > 0 {
				if roleMap, ok := roles[0].(map[string]any); ok {
					if role, ok := roleMap["value"].(string); ok && role != "" {
						user.Role = strings.ToLower(strings.TrimSpace(role))
					}
				}
			}
		case "email":
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				user.Email = strings.ToLower(strings.TrimSpace(s))
			}
		case "emails":
			if emails, ok := v.([]any); ok {
				for _, item := range emails {
					if m, ok := item.(map[string]any); ok {
						if email, ok := m["value"].(string); ok && strings.TrimSpace(email) != "" {
							user.Email = strings.ToLower(strings.TrimSpace(email))
							break
						}
					}
				}
			} else if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				user.Email = strings.ToLower(strings.TrimSpace(s))
			}
		default:
			// Entra: roles[primary eq "True"].value = "<role>"
			if strings.HasPrefix(cleanKey, "roles[") {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					user.Role = strings.ToLower(strings.TrimSpace(s))
				}
				continue
			}
			if strings.HasPrefix(cleanKey, "emails") {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					user.Email = strings.ToLower(strings.TrimSpace(s))
				}
			}
		}
	}
}

func scimEmailFromBody(emails []struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary"`
}) string {
	email := ""
	for _, item := range emails {
		if item.Value != "" {
			email = item.Value
			if item.Primary {
				break
			}
		}
	}
	return email
}

func scimStatusFromActive(active bool) string {
	if active {
		return tables.UserStatusApproved
	}
	return tables.UserStatusDisabled
}

func scimUserResource(user *tables.TableUser) map[string]any {
	if user == nil {
		return map[string]any{}
	}
	email := user.Email
	if email == "" {
		email = user.Username
	}
	scimUserName := user.Email
	if scimUserName == "" {
		scimUserName = user.Username
	}
	displayName := user.Username
	if displayName == "" {
		displayName = email
	}
	externalID := user.ExternalID
	resource := map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":          user.ID,
		"userName":    scimUserName,
		"displayName": displayName,
		"name": map[string]any{
			"formatted": displayName,
		},
		"active": user.IsApproved(),
		"emails": []map[string]any{{
			"value":   email,
			"type":    "work",
			"primary": true,
		}},
		"roles": []map[string]any{{"value": user.Role}},
		"meta": map[string]any{
			"resourceType": "User",
			"created":      user.CreatedAt,
			"lastModified": user.UpdatedAt,
		},
	}
	if externalID != "" {
		resource["externalId"] = externalID
	}
	return resource
}

var scimMemberFilterRegex = regexp.MustCompile(`(?i)members\[value\s+eq\s+["']?([^"'\]]+)["']?\]`)

func scimExtractMemberIDFromPath(path string) string {
	m := scimMemberFilterRegex.FindStringSubmatch(path)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func scimExtractMemberIDs(value any) []string {
	var ids []string
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if uid, ok := m["value"].(string); ok && strings.TrimSpace(uid) != "" {
					ids = append(ids, strings.TrimSpace(uid))
				}
			} else if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				ids = append(ids, strings.TrimSpace(s))
			}
		}
	case map[string]any:
		if uid, ok := v["value"].(string); ok && strings.TrimSpace(uid) != "" {
			ids = append(ids, strings.TrimSpace(uid))
		}
		if members, ok := v["members"].([]any); ok {
			ids = append(ids, scimExtractMemberIDs(members)...)
		}
	case string:
		if strings.TrimSpace(v) != "" {
			ids = append(ids, strings.TrimSpace(v))
		}
	}
	return ids
}

func scimFilterGroups(teams []tables.TableTeam, filter string) ([]tables.TableTeam, bool) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return teams, true
	}
	normalized := filter
	const groupSchemaPrefix = "urn:ietf:params:scim:schemas:core:2.0:group:"
	if strings.HasPrefix(strings.ToLower(normalized), groupSchemaPrefix) {
		normalized = normalized[len(groupSchemaPrefix):]
	}
	normLower := strings.ToLower(strings.TrimSpace(normalized))
	for _, prefix := range []string{"displayname eq ", "externalid eq ", "id eq "} {
		if strings.HasPrefix(normLower, prefix) {
			raw := strings.TrimSpace(normalized[len(prefix):])
			raw = strings.Trim(raw, `"'`)
			out := make([]tables.TableTeam, 0)
			for _, t := range teams {
				switch prefix {
				case "displayname eq ":
					if strings.EqualFold(t.Name, raw) {
						out = append(out, t)
					}
				case "externalid eq ":
					if t.SourceID != nil && strings.EqualFold(*t.SourceID, raw) {
						out = append(out, t)
					}
				case "id eq ":
					if strings.EqualFold(t.ID, raw) {
						out = append(out, t)
					}
				}
			}
			return out, true
		}
	}
	return nil, false
}

func (h *WorkspaceHandler) scimGroupResource(ctx context.Context, team *tables.TableTeam, members []tables.TableTeamMember) map[string]any {
	if team == nil {
		return map[string]any{}
	}
	membersList := make([]map[string]any, 0, len(members))
	for _, m := range members {
		item := map[string]any{
			"value": m.UserID,
			"$ref":  fmt.Sprintf("../Users/%s", m.UserID),
		}
		if h.store != nil && h.store.ConfigStore != nil {
			if u, _ := h.store.ConfigStore.GetUserByID(ctx, m.UserID); u != nil {
				display := u.Username
				if u.Email != "" {
					display = u.Email
				}
				item["display"] = display
			}
		}
		membersList = append(membersList, item)
	}
	resource := map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		"id":          team.ID,
		"displayName": team.Name,
		"members":     membersList,
		"meta": map[string]any{
			"resourceType": "Group",
			"created":      team.CreatedAt,
			"lastModified": team.UpdatedAt,
		},
	}
	if team.SourceID != nil && *team.SourceID != "" {
		resource["externalId"] = *team.SourceID
	}
	return resource
}

func (h *WorkspaceHandler) scimListGroups(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	teams, err := h.store.ConfigStore.GetTeams(ctx, "")
	if err != nil {
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to list groups")
		return
	}
	filter := string(ctx.QueryArgs().Peek("filter"))
	if filter != "" {
		filtered, ok := scimFilterGroups(teams, filter)
		if !ok {
			scimError(ctx, fasthttp.StatusBadRequest, "unsupported filter expression", "invalidFilter")
			return
		}
		teams = filtered
	}
	sort.SliceStable(teams, func(i, j int) bool {
		if !teams[i].CreatedAt.Equal(teams[j].CreatedAt) {
			return teams[i].CreatedAt.Before(teams[j].CreatedAt)
		}
		return teams[i].ID < teams[j].ID
	})
	startIndex := queryInt(ctx, "startIndex", 1)
	if startIndex < 1 {
		startIndex = 1
	}
	count := queryInt(ctx, "count", 100)
	if count <= 0 {
		count = 100
	}
	total := len(teams)
	offset := startIndex - 1
	if offset > total {
		offset = total
	}
	end := offset + count
	if end > total {
		end = total
	}
	page := teams[offset:end]
	ws := h.scimWorkspaceStore()
	resources := make([]map[string]any, 0, len(page))
	for _, team := range page {
		var members []tables.TableTeamMember
		if ws != nil {
			members, _ = ws.ListTeamMembers(ctx, team.ID)
		}
		resources = append(resources, h.scimGroupResource(ctx, &team, members))
	}
	SendSCIMJSON(ctx, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": total,
		"itemsPerPage": len(resources),
		"startIndex":   startIndex,
		"Resources":    resources,
	})
}

func (h *WorkspaceHandler) scimGetGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		scimError(ctx, fasthttp.StatusNotFound, "group not found")
		return
	}
	ws := h.scimWorkspaceStore()
	var members []tables.TableTeamMember
	if ws != nil {
		members, _ = ws.ListTeamMembers(ctx, id)
	}
	SendSCIMJSON(ctx, h.scimGroupResource(ctx, team, members))
}

// scimResolveUserID maps a SCIM member value (our id, externalId, username or email) to a
// user id. Unknown members resolve to "" so no orphan membership rows are written.
func (h *WorkspaceHandler) scimResolveUserID(ctx context.Context, identifier string) string {
	raw := strings.TrimSpace(identifier)
	if raw == "" || h.store == nil || h.store.ConfigStore == nil {
		return ""
	}
	if u, err := h.store.ConfigStore.GetUserByID(ctx, raw); err == nil && u != nil {
		return u.ID
	}
	if users, err := h.store.ConfigStore.GetUsers(ctx); err == nil {
		for _, u := range users {
			if u == nil {
				continue
			}
			if strings.EqualFold(u.Username, raw) ||
				(u.ExternalID != "" && strings.EqualFold(u.ExternalID, raw)) ||
				(u.Email != "" && strings.EqualFold(u.Email, raw)) {
				return u.ID
			}
		}
	}
	return ""
}

func scimMemberSet(ws configstore.WorkspaceStore, ctx context.Context, teamID string) map[string]bool {
	set := map[string]bool{}
	if ws == nil {
		return set
	}
	if members, err := ws.ListTeamMembers(ctx, teamID); err == nil {
		for _, m := range members {
			set[m.UserID] = true
		}
	}
	return set
}

// scimAddMembers adds the given members; prompt hooks fire only for users who actually joined.
// Unknown member values are skipped (IdP may race ahead of user create). DB write failures return.
func (h *WorkspaceHandler) scimAddMembers(ctx context.Context, ws configstore.WorkspaceStore, teamID string, rawIDs []string) error {
	if ws == nil {
		return nil
	}
	current := scimMemberSet(ws, ctx, teamID)
	for _, raw := range rawIDs {
		uid := h.scimResolveUserID(ctx, raw)
		if uid == "" || current[uid] {
			continue
		}
		if err := ws.AddTeamMember(ctx, teamID, uid); err != nil {
			return fmt.Errorf("add member %s: %w", uid, err)
		}
		current[uid] = true
		if h.promptLifecycle != nil {
			_ = h.promptLifecycle.OnTeamMemberAdded(ctx, teamID, uid)
		}
	}
	return nil
}

// scimRemoveMembers removes the given members; hooks fire only for users who were members.
func (h *WorkspaceHandler) scimRemoveMembers(ctx context.Context, ws configstore.WorkspaceStore, teamID string, userIDs []string) error {
	if ws == nil {
		return nil
	}
	current := scimMemberSet(ws, ctx, teamID)
	for _, uid := range userIDs {
		if uid == "" || !current[uid] {
			continue
		}
		if err := ws.RemoveTeamMember(ctx, teamID, uid); err != nil {
			return fmt.Errorf("remove member %s: %w", uid, err)
		}
		delete(current, uid)
		if h.promptLifecycle != nil {
			_ = h.promptLifecycle.OnTeamMemberRemoved(ctx, teamID, uid)
		}
	}
	return nil
}

// scimSetMembers makes the team membership exactly rawIDs, touching only the difference so
// unchanged members keep their prompt placement.
func (h *WorkspaceHandler) scimSetMembers(ctx context.Context, ws configstore.WorkspaceStore, teamID string, rawIDs []string) error {
	if ws == nil {
		return nil
	}
	target := map[string]bool{}
	for _, raw := range rawIDs {
		if uid := h.scimResolveUserID(ctx, raw); uid != "" {
			target[uid] = true
		}
	}
	var leaving []string
	for uid := range scimMemberSet(ws, ctx, teamID) {
		if !target[uid] {
			leaving = append(leaving, uid)
		}
	}
	if err := h.scimRemoveMembers(ctx, ws, teamID, leaving); err != nil {
		return err
	}
	joining := make([]string, 0, len(target))
	for uid := range target {
		joining = append(joining, uid)
	}
	return h.scimAddMembers(ctx, ws, teamID, joining)
}

func (h *WorkspaceHandler) scimResolveAll(ctx context.Context, rawIDs []string) []string {
	out := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		if uid := h.scimResolveUserID(ctx, raw); uid != "" {
			out = append(out, uid)
		}
	}
	return out
}

func scimIsMembersPath(path string) bool {
	return path == "" || strings.EqualFold(path, "members")
}

func (h *WorkspaceHandler) scimCreateGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	var body struct {
		DisplayName string `json:"displayName"`
		ExternalID  string `json:"externalId"`
		Members     []struct {
			Value   string `json:"value"`
			Display string `json:"display"`
		} `json:"members"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		scimError(ctx, fasthttp.StatusBadRequest, "invalid scim group payload")
		return
	}
	displayName := strings.TrimSpace(body.DisplayName)
	if displayName == "" {
		scimError(ctx, fasthttp.StatusBadRequest, "displayName is required")
		return
	}
	team, _ := h.store.ConfigStore.GetTeamByName(ctx, displayName, "")
	if team == nil {
		team = &tables.TableTeam{
			ID:        uuid.NewString(),
			Name:      displayName,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if body.ExternalID != "" {
			team.SourceID = &body.ExternalID
		}
		if err := h.store.ConfigStore.CreateTeam(ctx, team); err != nil {
			scimError(ctx, fasthttp.StatusInternalServerError, "failed to create group")
			return
		}
		if h.promptLifecycle != nil {
			_ = h.promptLifecycle.OnTeamCreated(ctx, team)
		}
	} else if body.ExternalID != "" && (team.SourceID == nil || *team.SourceID == "") {
		team.SourceID = &body.ExternalID
		_ = h.store.ConfigStore.UpdateTeam(ctx, team)
	}
	ws := h.scimWorkspaceStore()
	rawIDs := make([]string, 0, len(body.Members))
	for _, m := range body.Members {
		rawIDs = append(rawIDs, m.Value)
	}
	if err := h.scimAddMembers(ctx, ws, team.ID, rawIDs); err != nil {
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to add group members: "+err.Error())
		return
	}
	var members []tables.TableTeamMember
	if ws != nil {
		members, _ = ws.ListTeamMembers(ctx, team.ID)
	}
	ctx.Response.Header.Set("Location", "/scim/v2/Groups/"+team.ID)
	SendSCIMJSONWithStatus(ctx, h.scimGroupResource(ctx, team, members), fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) scimPutGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		scimError(ctx, fasthttp.StatusNotFound, "group not found")
		return
	}
	var body struct {
		DisplayName string `json:"displayName"`
		ExternalID  string `json:"externalId"`
		Members     []struct {
			Value string `json:"value"`
		} `json:"members"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		scimError(ctx, fasthttp.StatusBadRequest, "invalid scim group payload")
		return
	}
	if body.ExternalID != "" {
		team.SourceID = &body.ExternalID
	}
	newName := team.Name
	if strings.TrimSpace(body.DisplayName) != "" {
		newName = strings.TrimSpace(body.DisplayName)
	}
	if !h.scimRenameTeam(ctx, team, newName) {
		return
	}

	ws := h.scimWorkspaceStore()
	rawIDs := make([]string, 0, len(body.Members))
	for _, m := range body.Members {
		rawIDs = append(rawIDs, m.Value)
	}
	if err := h.scimSetMembers(ctx, ws, team.ID, rawIDs); err != nil {
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to update group members: "+err.Error())
		return
	}
	var members []tables.TableTeamMember
	if ws != nil {
		members, _ = ws.ListTeamMembers(ctx, team.ID)
	}
	SendSCIMJSON(ctx, h.scimGroupResource(ctx, team, members))
}

// scimRenameTeam saves the team under newName and only then renames its prompt folder.
// On failure (e.g. the name is taken) it restores the old name, answers the identity
// provider with an error and returns false, so team and folder never disagree.
func (h *WorkspaceHandler) scimRenameTeam(ctx *fasthttp.RequestCtx, team *tables.TableTeam, newName string) bool {
	oldName := team.Name
	team.Name = newName
	team.UpdatedAt = time.Now().UTC()
	if err := h.store.ConfigStore.UpdateTeam(ctx, team); err != nil {
		team.Name = oldName
		scimError(ctx, fasthttp.StatusConflict, "failed to update group: "+err.Error())
		return false
	}
	if h.promptLifecycle != nil && oldName != team.Name {
		_ = h.promptLifecycle.OnTeamUpdated(ctx, team, oldName, team.CustomerID)
	}
	return true
}

func (h *WorkspaceHandler) scimPatchGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		scimError(ctx, fasthttp.StatusNotFound, "group not found")
		return
	}
	var body struct {
		Operations []struct {
			Op    string `json:"op"`
			Path  string `json:"path"`
			Value any    `json:"value"`
		} `json:"Operations"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		scimError(ctx, fasthttp.StatusBadRequest, "invalid scim patch payload")
		return
	}
	ws := h.scimWorkspaceStore()
	for _, op := range body.Operations {
		opName := strings.ToLower(op.Op)
		path := strings.TrimSpace(op.Path)
		cleanPath := path
		const groupSchema = "urn:ietf:params:scim:schemas:core:2.0:group:"
		if strings.HasPrefix(strings.ToLower(cleanPath), groupSchema) {
			cleanPath = cleanPath[len(groupSchema):]
		}

		switch opName {
		case "add":
			if strings.EqualFold(cleanPath, "displayname") {
				if name, ok := op.Value.(string); ok && strings.TrimSpace(name) != "" {
					if !h.scimRenameTeam(ctx, team, strings.TrimSpace(name)) {
						return
					}
				}
			}
			// Only a members path (or a path-less value object) carries members; a string value
			// for displayName/externalId must not be read as a member id.
			if strings.EqualFold(cleanPath, "members") {
				if err := h.scimAddMembers(ctx, ws, team.ID, scimExtractMemberIDs(op.Value)); err != nil {
					scimError(ctx, fasthttp.StatusInternalServerError, "failed to add group members: "+err.Error())
					return
				}
			} else if cleanPath == "" {
				if m, ok := op.Value.(map[string]any); ok {
					if name, ok := m["displayName"].(string); ok && strings.TrimSpace(name) != "" && strings.TrimSpace(name) != team.Name {
						if !h.scimRenameTeam(ctx, team, strings.TrimSpace(name)) {
							return
						}
					}
					if membersVal, ok := m["members"]; ok {
						if err := h.scimAddMembers(ctx, ws, team.ID, scimExtractMemberIDs(membersVal)); err != nil {
							scimError(ctx, fasthttp.StatusInternalServerError, "failed to add group members: "+err.Error())
							return
						}
					}
				}
			}
		case "remove":
			if cleanPath != "" {
				if filterUID := scimExtractMemberIDFromPath(cleanPath); filterUID != "" {
					if err := h.scimRemoveMembers(ctx, ws, team.ID, h.scimResolveAll(ctx, []string{filterUID})); err != nil {
						scimError(ctx, fasthttp.StatusInternalServerError, "failed to remove group members: "+err.Error())
						return
					}
					continue
				}
			}
			if !scimIsMembersPath(cleanPath) {
				continue
			}
			memberIDs := scimExtractMemberIDs(op.Value)
			if len(memberIDs) > 0 {
				if err := h.scimRemoveMembers(ctx, ws, team.ID, h.scimResolveAll(ctx, memberIDs)); err != nil {
					scimError(ctx, fasthttp.StatusInternalServerError, "failed to remove group members: "+err.Error())
					return
				}
			} else if strings.EqualFold(cleanPath, "members") {
				all := make([]string, 0)
				for uid := range scimMemberSet(ws, ctx, team.ID) {
					all = append(all, uid)
				}
				if err := h.scimRemoveMembers(ctx, ws, team.ID, all); err != nil {
					scimError(ctx, fasthttp.StatusInternalServerError, "failed to remove group members: "+err.Error())
					return
				}
			}
		case "replace":
			if strings.EqualFold(cleanPath, "displayname") {
				if name, ok := op.Value.(string); ok && strings.TrimSpace(name) != "" {
					if !h.scimRenameTeam(ctx, team, strings.TrimSpace(name)) {
						return
					}
				}
			} else if strings.EqualFold(cleanPath, "members") {
				if err := h.scimSetMembers(ctx, ws, team.ID, scimExtractMemberIDs(op.Value)); err != nil {
					scimError(ctx, fasthttp.StatusInternalServerError, "failed to update group members: "+err.Error())
					return
				}
			} else if cleanPath == "" {
				if m, ok := op.Value.(map[string]any); ok {
					if name, ok := m["displayName"].(string); ok && strings.TrimSpace(name) != "" {
						if !h.scimRenameTeam(ctx, team, strings.TrimSpace(name)) {
							return
						}
					}
					if membersVal, ok := m["members"]; ok {
						if err := h.scimSetMembers(ctx, ws, team.ID, scimExtractMemberIDs(membersVal)); err != nil {
							scimError(ctx, fasthttp.StatusInternalServerError, "failed to update group members: "+err.Error())
							return
						}
					}
				}
			}
		}
	}
	team.UpdatedAt = time.Now().UTC()
	_ = h.store.ConfigStore.UpdateTeam(ctx, team)

	var members []tables.TableTeamMember
	if ws != nil {
		members, _ = ws.ListTeamMembers(ctx, team.ID)
	}
	SendSCIMJSON(ctx, h.scimGroupResource(ctx, team, members))
}

func (h *WorkspaceHandler) scimDeleteGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		scimError(ctx, fasthttp.StatusNotFound, "group not found")
		return
	}
	// Same order as the governance team delete: the prompt hook needs the memberships,
	// which DeleteTeam removes.
	if h.governanceManager != nil {
		_ = h.governanceManager.RemoveTeam(ctx, team.ID)
	}
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnTeamDeleted(ctx, team)
	}
	if err := h.store.ConfigStore.DeleteTeam(ctx, id); err != nil {
		scimError(ctx, fasthttp.StatusInternalServerError, "failed to delete group")
		return
	}
	removeRBACScopeGrant(ctx, h.store.ConfigStore, rbacScopeTeam, id)
	if reloader, ok := h.governanceManager.(interface{ ReloadBusinessUnitTeamIndex(context.Context) }); ok {
		reloader.ReloadBusinessUnitTeamIndex(ctx)
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}
