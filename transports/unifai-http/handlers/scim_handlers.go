package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
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

func (h *WorkspaceHandler) scimServiceProviderConfig(ctx *fasthttp.RequestCtx) {
	SendJSON(ctx, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":   map[string]any{"supported": true},
		"bulk":    map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":  map[string]any{"supported": true, "maxResults": 200},
		"changePassword": map[string]any{"supported": false},
		"sort":    map[string]any{"supported": false},
		"etag":    map[string]any{"supported": false},
		"authenticationSchemes": []map[string]any{{
			"type":        "oauthbearertoken",
			"name":        "OAuth Bearer Token",
			"description": "Authentication scheme using the bearer token configured in SCIM settings",
		}},
	})
}

func (h *WorkspaceHandler) scimSchemas(ctx *fasthttp.RequestCtx) {
	SendJSON(ctx, map[string]any{
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
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	users, err := h.store.ConfigStore.GetUsers(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list users")
		return
	}
	filter := string(ctx.QueryArgs().Peek("filter"))
	if filter != "" {
		users = scimFilterUsers(users, filter)
	}
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
	SendJSON(ctx, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": total,
		"itemsPerPage": len(resources),
		"startIndex":   startIndex,
		"Resources":    resources,
	})
}

var scimEmailFilterRegex = regexp.MustCompile(`(?i)emails(?:\[[^\]]+\])?\.value\s+eq\s+["']?([^"']+)["']?`)

func scimFilterUsers(users []*tables.TableUser, filter string) []*tables.TableUser {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return users
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
		return out
	}

	normLower := strings.ToLower(strings.TrimSpace(normalized))
	for _, prefix := range []string{"username eq ", "externalid eq ", "id eq ", "email eq ", "emails.value eq "} {
		if strings.HasPrefix(normLower, prefix) {
			raw := strings.TrimSpace(normalized[len(prefix):])
			raw = strings.Trim(raw, `"'`)
			out := make([]*tables.TableUser, 0)
			for _, user := range users {
				switch prefix {
				case "username eq ":
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
			return out
		}
	}
	return []*tables.TableUser{}
}

func (h *WorkspaceHandler) scimGetUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	SendJSON(ctx, scimUserResource(user))
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
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	var body struct {
		UserName   string `json:"userName"`
		ExternalID string `json:"externalId"`
		Emails     []struct {
			Value   string `json:"value"`
			Primary bool   `json:"primary"`
		} `json:"emails"`
		Active bool `json:"active"`
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
		SendError(ctx, fasthttp.StatusBadRequest, "invalid scim payload")
		return
	}
	email := scimEmailFromBody(body.Emails)
	username := strings.TrimSpace(body.UserName)
	if email == "" && strings.Contains(username, "@") {
		email = username
	}
	if username == "" {
		if formatted := strings.TrimSpace(body.Name.Formatted); formatted != "" {
			username = formatted
		} else if email != "" {
			username = email
		}
	}
	if username == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "userName is required")
		return
	}

	role := "user"
	if len(body.Roles) > 0 && body.Roles[0].Value != "" {
		role = scimNormalizeRole(body.Roles[0].Value)
	} else if def := h.scimDefaultRole(ctx); def != "" {
		role = scimNormalizeRole(def)
	}

	// Gracefully adopt or link existing user if username or email is already present in UnifAI
	var existing *tables.TableUser
	if u, _ := h.store.ConfigStore.GetUserByUsername(ctx, username); u != nil {
		existing = u
	} else if email != "" {
		if u, _ := h.store.ConfigStore.GetUserByEmail(ctx, email); u != nil {
			existing = u
		}
	}
	if existing != nil {
		if body.ExternalID != "" && (existing.ExternalID == "" || existing.ExternalID != body.ExternalID) {
			existing.ExternalID = body.ExternalID
		}
		existing.Status = scimStatusFromActive(body.Active)
		if role != "" && (existing.Role == "" || existing.Role == "user") {
			existing.Role = role
		}
		existing.UpdatedAt = time.Now().UTC()
		_ = h.store.ConfigStore.UpdateUser(ctx, existing)
		if h.promptLifecycle != nil {
			_ = h.promptLifecycle.OnUserCreated(ctx, existing, true)
		}
		SendJSONWithStatus(ctx, scimUserResource(existing), fasthttp.StatusOK)
		return
	}

	user := &tables.TableUser{
		ID:       uuid.NewString(),
		Username: username,
		Email:    email,
		Password: uuid.NewString(),
		Role:     role,
		Status:   scimStatusFromActive(body.Active),
	}
	if body.ExternalID != "" {
		user.ExternalID = body.ExternalID
	}
	if err := h.store.ConfigStore.CreateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to create user")
		return
	}
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnUserCreated(ctx, user, true)
	}
	SendJSONWithStatus(ctx, scimUserResource(user), fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) scimPutUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	var body map[string]any
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid scim payload")
		return
	}
	oldEmail := user.Email
	oldUsername := user.Username
	applySCIMUserPatch(user, body)
	user.UpdatedAt = time.Now().UTC()
	if err := h.store.ConfigStore.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update user")
		return
	}
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnUserUpdated(ctx, user, oldEmail, oldUsername)
	}
	SendJSON(ctx, scimUserResource(user))
}

func (h *WorkspaceHandler) scimPatchUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
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
		SendError(ctx, fasthttp.StatusBadRequest, "invalid scim patch payload")
		return
	}
	oldEmail := user.Email
	oldUsername := user.Username
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
			// no-op for minimal support
		}
	}
	user.UpdatedAt = time.Now().UTC()
	if err := h.store.ConfigStore.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update user")
		return
	}
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnUserUpdated(ctx, user, oldEmail, oldUsername)
	}
	SendJSON(ctx, scimUserResource(user))
}

func (h *WorkspaceHandler) scimDeleteUser(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	user, err := h.store.ConfigStore.GetUserByID(ctx, id)
	if err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	if err := h.store.ConfigStore.DeleteUser(ctx, id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete user")
		return
	}
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnUserDeleted(ctx, user)
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
		case "username":
			if s, ok := v.(string); ok && s != "" {
				user.Username = s
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
						user.Role = scimNormalizeRole(role)
					}
				}
			}
		case "email":
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				user.Email = strings.TrimSpace(s)
			}
		case "emails":
			if emails, ok := v.([]any); ok {
				for _, item := range emails {
					if m, ok := item.(map[string]any); ok {
						if email, ok := m["value"].(string); ok && email != "" {
							user.Email = email
							break
						}
					}
				}
			} else if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				user.Email = strings.TrimSpace(s)
			}
		default:
			if strings.HasPrefix(cleanKey, "emails") {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					user.Email = strings.TrimSpace(s)
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
	return tables.UserStatusPending
}

func scimUserResource(user *tables.TableUser) map[string]any {
	if user == nil {
		return map[string]any{}
	}
	email := user.Email
	if email == "" {
		email = user.Username
	}
	externalID := user.ExternalID
	resource := map[string]any{
		"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":       user.ID,
		"userName": user.Username,
		"name": map[string]any{
			"formatted": user.Username,
		},
		"active":   user.IsApproved(),
		"emails": []map[string]any{{
			"value": email, "primary": true,
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

func scimFilterGroups(teams []tables.TableTeam, filter string) []tables.TableTeam {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return teams
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
			return out
		}
	}
	return []tables.TableTeam{}
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
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	teams, err := h.store.ConfigStore.GetTeams(ctx, "")
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list groups")
		return
	}
	filter := string(ctx.QueryArgs().Peek("filter"))
	if filter != "" {
		teams = scimFilterGroups(teams, filter)
	}
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
	SendJSON(ctx, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": total,
		"itemsPerPage": len(resources),
		"startIndex":   startIndex,
		"Resources":    resources,
	})
}

func (h *WorkspaceHandler) scimGetGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		SendError(ctx, fasthttp.StatusNotFound, "group not found")
		return
	}
	ws := h.scimWorkspaceStore()
	var members []tables.TableTeamMember
	if ws != nil {
		members, _ = ws.ListTeamMembers(ctx, id)
	}
	SendJSON(ctx, h.scimGroupResource(ctx, team, members))
}

func (h *WorkspaceHandler) scimResolveUserID(ctx context.Context, identifier string) string {
	raw := strings.TrimSpace(identifier)
	if raw == "" || h.store == nil || h.store.ConfigStore == nil {
		return raw
	}
	if u, err := h.store.ConfigStore.GetUserByID(ctx, raw); err == nil && u != nil {
		return u.ID
	}
	if u, err := h.store.ConfigStore.GetUserByUsername(ctx, raw); err == nil && u != nil {
		return u.ID
	}
	if users, err := h.store.ConfigStore.GetUsers(ctx); err == nil {
		for _, u := range users {
			if (u.ExternalID != "" && strings.EqualFold(u.ExternalID, raw)) || (u.Email != "" && strings.EqualFold(u.Email, raw)) {
				return u.ID
			}
		}
	}
	return raw
}

func (h *WorkspaceHandler) scimCreateGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
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
		SendError(ctx, fasthttp.StatusBadRequest, "invalid scim group payload")
		return
	}
	displayName := strings.TrimSpace(body.DisplayName)
	if displayName == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "displayName is required")
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
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to create group")
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
	if ws != nil {
		for _, m := range body.Members {
			rawUID := strings.TrimSpace(m.Value)
			if rawUID != "" {
				uid := h.scimResolveUserID(ctx, rawUID)
				_ = ws.AddTeamMember(ctx, team.ID, uid)
				if h.promptLifecycle != nil {
					_ = h.promptLifecycle.OnTeamMemberAdded(ctx, team.ID, uid)
				}
			}
		}
	}
	var members []tables.TableTeamMember
	if ws != nil {
		members, _ = ws.ListTeamMembers(ctx, team.ID)
	}
	SendJSONWithStatus(ctx, h.scimGroupResource(ctx, team, members), fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) scimPutGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		SendError(ctx, fasthttp.StatusNotFound, "group not found")
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
		SendError(ctx, fasthttp.StatusBadRequest, "invalid scim group payload")
		return
	}
	oldName := team.Name
	if strings.TrimSpace(body.DisplayName) != "" {
		team.Name = strings.TrimSpace(body.DisplayName)
	}
	if body.ExternalID != "" {
		team.SourceID = &body.ExternalID
	}
	team.UpdatedAt = time.Now().UTC()
	_ = h.store.ConfigStore.UpdateTeam(ctx, team)
	if h.promptLifecycle != nil && oldName != team.Name {
		_ = h.promptLifecycle.OnTeamUpdated(ctx, team, oldName, team.CustomerID)
	}

	ws := h.scimWorkspaceStore()
	if ws != nil {
		currentMembers, _ := ws.ListTeamMembers(ctx, team.ID)
		targetMap := make(map[string]bool)
		for _, m := range body.Members {
			rawUID := strings.TrimSpace(m.Value)
			if rawUID != "" {
				uid := h.scimResolveUserID(ctx, rawUID)
				targetMap[uid] = true
			}
		}
		for _, cur := range currentMembers {
			if !targetMap[cur.UserID] {
				_ = ws.RemoveTeamMember(ctx, team.ID, cur.UserID)
				if h.promptLifecycle != nil {
					_ = h.promptLifecycle.OnTeamMemberRemoved(ctx, team.ID, cur.UserID)
				}
			}
		}
		for uid := range targetMap {
			_ = ws.AddTeamMember(ctx, team.ID, uid)
			if h.promptLifecycle != nil {
				_ = h.promptLifecycle.OnTeamMemberAdded(ctx, team.ID, uid)
			}
		}
	}
	var members []tables.TableTeamMember
	if ws != nil {
		members, _ = ws.ListTeamMembers(ctx, team.ID)
	}
	SendJSON(ctx, h.scimGroupResource(ctx, team, members))
}

func (h *WorkspaceHandler) scimPatchGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		SendError(ctx, fasthttp.StatusNotFound, "group not found")
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
		SendError(ctx, fasthttp.StatusBadRequest, "invalid scim patch payload")
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
					oldName := team.Name
					team.Name = strings.TrimSpace(name)
					_ = h.store.ConfigStore.UpdateTeam(ctx, team)
					if h.promptLifecycle != nil && oldName != team.Name {
						_ = h.promptLifecycle.OnTeamUpdated(ctx, team, oldName, team.CustomerID)
					}
				}
			}
			memberIDs := scimExtractMemberIDs(op.Value)
			if ws != nil {
				for _, rawUID := range memberIDs {
					uid := h.scimResolveUserID(ctx, rawUID)
					_ = ws.AddTeamMember(ctx, team.ID, uid)
					if h.promptLifecycle != nil {
						_ = h.promptLifecycle.OnTeamMemberAdded(ctx, team.ID, uid)
					}
				}
			}
		case "remove":
			if cleanPath != "" {
				filterUID := scimExtractMemberIDFromPath(cleanPath)
				if filterUID != "" {
					if ws != nil {
						uid := h.scimResolveUserID(ctx, filterUID)
						_ = ws.RemoveTeamMember(ctx, team.ID, uid)
						if h.promptLifecycle != nil {
							_ = h.promptLifecycle.OnTeamMemberRemoved(ctx, team.ID, uid)
						}
					}
					continue
				}
			}
			memberIDs := scimExtractMemberIDs(op.Value)
			if ws != nil {
				if len(memberIDs) > 0 {
					for _, rawUID := range memberIDs {
						uid := h.scimResolveUserID(ctx, rawUID)
						_ = ws.RemoveTeamMember(ctx, team.ID, uid)
						if h.promptLifecycle != nil {
							_ = h.promptLifecycle.OnTeamMemberRemoved(ctx, team.ID, uid)
						}
					}
				} else if strings.EqualFold(cleanPath, "members") {
					all, _ := ws.ListTeamMembers(ctx, team.ID)
					for _, m := range all {
						_ = ws.RemoveTeamMember(ctx, team.ID, m.UserID)
						if h.promptLifecycle != nil {
							_ = h.promptLifecycle.OnTeamMemberRemoved(ctx, team.ID, m.UserID)
						}
					}
				}
			}
		case "replace":
			if strings.EqualFold(cleanPath, "displayname") {
				if name, ok := op.Value.(string); ok && strings.TrimSpace(name) != "" {
					oldName := team.Name
					team.Name = strings.TrimSpace(name)
					_ = h.store.ConfigStore.UpdateTeam(ctx, team)
					if h.promptLifecycle != nil && oldName != team.Name {
						_ = h.promptLifecycle.OnTeamUpdated(ctx, team, oldName, team.CustomerID)
					}
				}
			} else if strings.EqualFold(cleanPath, "members") {
				if ws != nil {
					all, _ := ws.ListTeamMembers(ctx, team.ID)
					for _, m := range all {
						_ = ws.RemoveTeamMember(ctx, team.ID, m.UserID)
						if h.promptLifecycle != nil {
							_ = h.promptLifecycle.OnTeamMemberRemoved(ctx, team.ID, m.UserID)
						}
					}
					memberIDs := scimExtractMemberIDs(op.Value)
					for _, rawUID := range memberIDs {
						uid := h.scimResolveUserID(ctx, rawUID)
						_ = ws.AddTeamMember(ctx, team.ID, uid)
						if h.promptLifecycle != nil {
							_ = h.promptLifecycle.OnTeamMemberAdded(ctx, team.ID, uid)
						}
					}
				}
			} else if cleanPath == "" {
				if m, ok := op.Value.(map[string]any); ok {
					if name, ok := m["displayName"].(string); ok && strings.TrimSpace(name) != "" {
						oldName := team.Name
						team.Name = strings.TrimSpace(name)
						_ = h.store.ConfigStore.UpdateTeam(ctx, team)
						if h.promptLifecycle != nil && oldName != team.Name {
							_ = h.promptLifecycle.OnTeamUpdated(ctx, team, oldName, team.CustomerID)
						}
					}
					if membersVal, ok := m["members"]; ok && ws != nil {
						memberIDs := scimExtractMemberIDs(membersVal)
						for _, rawUID := range memberIDs {
							uid := h.scimResolveUserID(ctx, rawUID)
							_ = ws.AddTeamMember(ctx, team.ID, uid)
							if h.promptLifecycle != nil {
								_ = h.promptLifecycle.OnTeamMemberAdded(ctx, team.ID, uid)
							}
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
	SendJSON(ctx, h.scimGroupResource(ctx, team, members))
}

func (h *WorkspaceHandler) scimDeleteGroup(ctx *fasthttp.RequestCtx) {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return
	}
	id := pathID(ctx, "id")
	team, err := h.store.ConfigStore.GetTeam(ctx, id)
	if err != nil || team == nil {
		SendError(ctx, fasthttp.StatusNotFound, "group not found")
		return
	}
	if err := h.store.ConfigStore.DeleteTeam(ctx, id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete group")
		return
	}
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnTeamDeleted(ctx, team)
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

