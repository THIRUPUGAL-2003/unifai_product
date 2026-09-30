package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// mockWorkspaceStore implements WorkspaceStore for SCIM tests.
type scimTestWorkspaceStore struct {
	configstore.WorkspaceStore
	*memoryConfigStore
	settings map[string]string
	teams    map[string]*tables.TableTeam
	members  map[string]map[string]bool // teamID -> set of userIDs
}

func newSCIMTestStore() *scimTestWorkspaceStore {
	mem := newMemoryConfigStore()
	return &scimTestWorkspaceStore{
		memoryConfigStore: mem,
		settings:          make(map[string]string),
		teams:             make(map[string]*tables.TableTeam),
		members:           make(map[string]map[string]bool),
	}
}

func (s *scimTestWorkspaceStore) GetWorkspaceSetting(ctx context.Context, key string) (*tables.TableWorkspaceSetting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.settings[key]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	return &tables.TableWorkspaceSetting{
		Key:       key,
		Data:      val,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (s *scimTestWorkspaceStore) UpsertWorkspaceSetting(ctx context.Context, key, data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = data
	return nil
}

func (s *scimTestWorkspaceStore) DeleteWorkspaceSetting(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.settings, key)
	return nil
}

func (s *scimTestWorkspaceStore) GetTeams(ctx context.Context, orgID string) ([]tables.TableTeam, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]tables.TableTeam, 0, len(s.teams))
	for _, t := range s.teams {
		out = append(out, *t)
	}
	return out, nil
}

func (s *scimTestWorkspaceStore) GetTeam(ctx context.Context, id string) (*tables.TableTeam, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.teams[id]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	return t, nil
}

func (s *scimTestWorkspaceStore) GetTeamByName(ctx context.Context, name, orgID string) (*tables.TableTeam, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.teams {
		if t.Name == name {
			return t, nil
		}
	}
	return nil, configstore.ErrNotFound
}

func (s *scimTestWorkspaceStore) CreateTeam(ctx context.Context, team *tables.TableTeam, tx ...*gorm.DB) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teams[team.ID] = team
	return nil
}

func (s *scimTestWorkspaceStore) UpdateTeam(ctx context.Context, team *tables.TableTeam, tx ...*gorm.DB) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teams[team.ID] = team
	return nil
}

func (s *scimTestWorkspaceStore) DeleteTeam(ctx context.Context, id string, tx ...*gorm.DB) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.teams, id)
	delete(s.members, id)
	return nil
}

func (s *scimTestWorkspaceStore) ListTeamMembers(ctx context.Context, teamID string) ([]tables.TableTeamMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uids, ok := s.members[teamID]
	if !ok {
		return []tables.TableTeamMember{}, nil
	}
	out := make([]tables.TableTeamMember, 0, len(uids))
	for uid := range uids {
		out = append(out, tables.TableTeamMember{
			TeamID: teamID,
			UserID: uid,
		})
	}
	return out, nil
}

func (s *scimTestWorkspaceStore) AddTeamMember(ctx context.Context, teamID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.members[teamID] == nil {
		s.members[teamID] = make(map[string]bool)
	}
	s.members[teamID][userID] = true
	return nil
}

func (s *scimTestWorkspaceStore) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.members[teamID] != nil {
		delete(s.members[teamID], userID)
	}
	return nil
}

// TestMicrosoftEntraSCIMFlow simulates Microsoft Entra ID connecting to UnifAI SCIM 2.0.
func TestMicrosoftEntraSCIMFlow(t *testing.T) {
	store := newSCIMTestStore()
	handler := &WorkspaceHandler{
		workspace: store,
		store: &lib.Config{
			ConfigStore: store,
		},
	}

	testBearer := "entra-secret-bearer-token-12345"
	scimCfg := scimConfigPayload{
		Enabled:     true,
		Provider:    "entra",
		BearerToken: testBearer,
		Config: map[string]any{
			"tenantId":     "test-tenant-id",
			"clientId":     "test-client-id",
			"clientSecret": "test-client-secret",
			"defaultRole":  "user",
		},
	}
	rawCfg, _ := json.Marshal(scimCfg)
	_ = store.UpsertWorkspaceSetting(nil, configstore.WorkspaceSettingSCIM, string(rawCfg))

	authMiddleware := handler.scimMiddleware()

	// Helper to run a handler wrapped in SCIM auth
	runWithAuth := func(h fasthttp.RequestHandler, method, uri, token string, body any) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(method)
		ctx.Request.SetRequestURI(uri)
		if token != "" {
			ctx.Request.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			raw, _ := json.Marshal(body)
			ctx.Request.SetBody(raw)
			ctx.Request.Header.Set("Content-Type", "application/json")
		}
		authMiddleware(h)(ctx)
		return ctx
	}

	// 1. Entra Connection Test: Missing Token -> 401
	{
		ctx := runWithAuth(handler.scimServiceProviderConfig, "GET", "/scim/v2/ServiceProviderConfig", "", nil)
		if ctx.Response.StatusCode() != http.StatusUnauthorized {
			t.Fatalf("Expected 401 for missing token, got %d", ctx.Response.StatusCode())
		}
	}

	// 2. Entra Connection Test: Invalid Token -> 401
	{
		ctx := runWithAuth(handler.scimServiceProviderConfig, "GET", "/scim/v2/ServiceProviderConfig", "wrong-token", nil)
		if ctx.Response.StatusCode() != http.StatusUnauthorized {
			t.Fatalf("Expected 401 for wrong token, got %d", ctx.Response.StatusCode())
		}
	}

	// 3. Entra Connection Test: Valid Token -> ServiceProviderConfig 200
	{
		ctx := runWithAuth(handler.scimServiceProviderConfig, "GET", "/scim/v2/ServiceProviderConfig", testBearer, nil)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for ServiceProviderConfig, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		patch, ok := resp["patch"].(map[string]any)
		if !ok || patch["supported"] != true {
			t.Errorf("Expected patch.supported = true, got %v", resp["patch"])
		}
	}

	// 4. Entra Connection Test: Schemas endpoint -> 200
	{
		ctx := runWithAuth(handler.scimSchemas, "GET", "/scim/v2/Schemas", testBearer, nil)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for Schemas, got %d", ctx.Response.StatusCode())
		}
	}

	// 5. Entra probes for user existence before provisioning:
	// GET /scim/v2/Users?filter=userName eq "testuser@entra.com"
	{
		ctx := runWithAuth(handler.scimListUsers, "GET", `/scim/v2/Users?filter=userName%20eq%20"testuser@entra.com"`, testBearer, nil)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for list users query, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if int(resp["totalResults"].(float64)) != 0 {
			t.Errorf("Expected 0 results for non-existent user, got %v", resp["totalResults"])
		}
	}

	// 6. Entra provisions new user: POST /scim/v2/Users
	var createdUserID string
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "alex.smith@contoso.com",
			"externalId": "entra-guid-1111-2222",
			"active":     true,
			"emails": []map[string]any{
				{"value": "alex.smith@contoso.com", "primary": true},
			},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Expected 201 for SCIM user creation, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var created map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &created)
		createdUserID = created["id"].(string)
		if created["userName"] != "alex.smith@contoso.com" {
			t.Errorf("Expected userName alex.smith@contoso.com, got %v", created["userName"])
		}
		if created["externalId"] != "entra-guid-1111-2222" {
			t.Errorf("Expected externalId entra-guid-1111-2222, got %v", created["externalId"])
		}
		if created["active"] != true {
			t.Errorf("Expected active=true, got %v", created["active"])
		}
	}

	// 7. Entra checks user by single-quoted query:
	// GET /scim/v2/Users?filter=userName eq 'alex.smith@contoso.com'
	{
		ctx := runWithAuth(handler.scimListUsers, "GET", "/scim/v2/Users?filter=userName%20eq%20'alex.smith@contoso.com'", testBearer, nil)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for single-quoted user query, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if int(resp["totalResults"].(float64)) != 1 {
			t.Fatalf("Expected 1 result for single-quoted filter, got %v", resp["totalResults"])
		}
	}

	// 8. Entra checks user by schema-prefixed query:
	// GET /scim/v2/Users?filter=urn:ietf:params:scim:schemas:core:2.0:User:userName eq "alex.smith@contoso.com"
	{
		ctx := runWithAuth(handler.scimListUsers, "GET", `/scim/v2/Users?filter=urn:ietf:params:scim:schemas:core:2.0:User:userName%20eq%20"alex.smith@contoso.com"`, testBearer, nil)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for schema-prefixed query, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if int(resp["totalResults"].(float64)) != 1 {
			t.Fatalf("Expected 1 result for schema-prefixed filter, got %v", resp["totalResults"])
		}
	}

	// 9. Entra checks user by externalId:
	// GET /scim/v2/Users?filter=externalId eq "entra-guid-1111-2222"
	{
		ctx := runWithAuth(handler.scimListUsers, "GET", `/scim/v2/Users?filter=externalId%20eq%20"entra-guid-1111-2222"`, testBearer, nil)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for externalId query, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if int(resp["totalResults"].(float64)) != 1 {
			t.Fatalf("Expected 1 result for externalId filter, got %v", resp["totalResults"])
		}
	}

	// 10. Entra deactivates user via PATCH with string "False" & schema prefix
	{
		patchPayload := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":    "Replace",
					"path":  "urn:ietf:params:scim:schemas:core:2.0:User:active",
					"value": "False",
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", createdUserID)
			handler.scimPatchUser(c)
		}, "PATCH", "/scim/v2/Users/"+createdUserID, testBearer, patchPayload)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for SCIM user deactivation, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var patched map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &patched)
		if patched["active"] != false {
			t.Errorf("Expected active=false after Entra string deactivation, got %v", patched["active"])
		}
	}

	// 11. Entra provisions a group/team: POST /scim/v2/Groups
	var createdGroupID string
	{
		groupPayload := map[string]any{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"displayName": "AI Engineers",
			"externalId":  "entra-group-guid-9999",
			"members": []map[string]any{
				{"value": createdUserID, "display": "alex.smith@contoso.com"},
			},
		}
		ctx := runWithAuth(handler.scimCreateGroup, "POST", "/scim/v2/Groups", testBearer, groupPayload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Expected 201 for group creation, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var groupResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &groupResp)
		createdGroupID = groupResp["id"].(string)
		if groupResp["displayName"] != "AI Engineers" {
			t.Errorf("Expected group displayName AI Engineers, got %v", groupResp["displayName"])
		}
		members := groupResp["members"].([]any)
		if len(members) != 1 {
			t.Errorf("Expected 1 member in group, got %d", len(members))
		}
	}

	// 12. Entra removes member from group via PATCH: op=Remove, path=members[value eq "userID"]
	{
		groupPatch := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":   "Remove",
					"path": "members[value eq \"" + createdUserID + "\"]",
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", createdGroupID)
			handler.scimPatchGroup(c)
		}, "PATCH", "/scim/v2/Groups/"+createdGroupID, testBearer, groupPatch)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for group patch remove member, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var groupResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &groupResp)
		members := groupResp["members"].([]any)
		if len(members) != 0 {
			t.Errorf("Expected 0 members after Entra removal, got %d", len(members))
		}
	}

	// 13. Entra deletes user: DELETE /scim/v2/Users/{id} -> 204 No Content
	{
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", createdUserID)
			handler.scimDeleteUser(c)
		}, "DELETE", "/scim/v2/Users/"+createdUserID, testBearer, nil)

		if ctx.Response.StatusCode() != http.StatusNoContent {
			t.Fatalf("Expected 204 for SCIM delete user, got %d", ctx.Response.StatusCode())
		}
	}
}

// TestOktaSCIMFlow verifies Okta provisioning, role assignment, and user deactivation.
func TestOktaSCIMFlow(t *testing.T) {
	store := newSCIMTestStore()
	handler := &WorkspaceHandler{
		workspace: store,
		store: &lib.Config{
			ConfigStore: store,
		},
	}

	testBearer := "okta-provisioning-token-xyz987"
	// Save Okta config via updateSCIMConfig handler (like the UI does)
	oktaCfg := scimConfigPayload{
		Enabled:     true,
		Provider:    "okta",
		BearerToken: testBearer,
		Config: map[string]any{
			"issuerUrl":    "https://dev-12345.okta.com/oauth2/default",
			"clientId":     "okta-client-id",
			"clientSecret": "okta-client-secret",
			"apiToken":     "okta-api-token",
			"defaultRole":  "developer",
		},
	}
	putCtx := &fasthttp.RequestCtx{}
	putCtx.Request.Header.SetMethod("PUT")
	putCtx.Request.SetRequestURI("/api/scim/config")
	rawBody, _ := json.Marshal(oktaCfg)
	putCtx.Request.SetBody(rawBody)
	handler.updateSCIMConfig(putCtx)
	if putCtx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("Failed to save Okta config, got %d", putCtx.Response.StatusCode())
	}

	// Verify getSCIMConfig returns enabled Okta config
	getCtx := &fasthttp.RequestCtx{}
	getCtx.Request.Header.SetMethod("GET")
	getCtx.Request.SetRequestURI("/api/scim/config")
	handler.getSCIMConfig(getCtx)
	var loaded scimConfigPayload
	_ = json.Unmarshal(getCtx.Response.Body(), &loaded)
	if !loaded.Enabled || loaded.Provider != "okta" {
		t.Fatalf("Expected enabled Okta config, got %+v", loaded)
	}

	authMiddleware := handler.scimMiddleware()
	runWithAuth := func(h fasthttp.RequestHandler, method, uri, token string, body any) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(method)
		ctx.Request.SetRequestURI(uri)
		if token != "" {
			ctx.Request.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			raw, _ := json.Marshal(body)
			ctx.Request.SetBody(raw)
			ctx.Request.Header.Set("Content-Type", "application/json")
		}
		authMiddleware(h)(ctx)
		return ctx
	}

	// Okta creates user with explicit role "admin"
	var oktaUserID string
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "sam.admin@enterprise.com",
			"externalId": "okta-user-999000",
			"active":     true,
			"roles": []map[string]any{
				{"value": "admin"},
			},
			"emails": []map[string]any{
				{"value": "sam.admin@enterprise.com", "primary": true},
			},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Expected 201 for Okta user create, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var userResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &userResp)
		oktaUserID = userResp["id"].(string)
		roles := userResp["roles"].([]any)
		if len(roles) == 0 || roles[0].(map[string]any)["value"] != "admin" {
			t.Errorf("Expected role admin, got %v", userResp["roles"])
		}
	}

	// Okta deactivates user using standard replace format: {op: "replace", path: "active", value: false}
	{
		patchPayload := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":    "replace",
					"path":  "active",
					"value": false,
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", oktaUserID)
			handler.scimPatchUser(c)
		}, "PATCH", "/scim/v2/Users/"+oktaUserID, testBearer, patchPayload)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for Okta user deactivate, got %d", ctx.Response.StatusCode())
		}
		var patched map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &patched)
		if patched["active"] != false {
			t.Errorf("Expected active=false, got %v", patched["active"])
		}
	}
}

// TestKeycloakSCIMFlow verifies Keycloak provisioning, groups, and role default fallback.
func TestKeycloakSCIMFlow(t *testing.T) {
	store := newSCIMTestStore()
	handler := &WorkspaceHandler{
		workspace: store,
		store: &lib.Config{
			ConfigStore: store,
		},
	}

	testBearer := "keycloak-bearer-token-abc123"
	keycloakCfg := scimConfigPayload{
		Enabled:     true,
		Provider:    "keycloak",
		BearerToken: testBearer,
		Config: map[string]any{
			"issuerUrl":    "https://auth.company.internal/realms/unifai",
			"realm":        "unifai",
			"clientId":     "unifai-scim-client",
			"clientSecret": "keycloak-secret",
			"defaultRole":  "analyst",
		},
	}
	putCtx := &fasthttp.RequestCtx{}
	putCtx.Request.Header.SetMethod("PUT")
	putCtx.Request.SetRequestURI("/api/scim/config")
	rawBody, _ := json.Marshal(keycloakCfg)
	putCtx.Request.SetBody(rawBody)
	handler.updateSCIMConfig(putCtx)
	if putCtx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("Failed to save Keycloak config, got %d", putCtx.Response.StatusCode())
	}

	authMiddleware := handler.scimMiddleware()
	runWithAuth := func(h fasthttp.RequestHandler, method, uri, token string, body any) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(method)
		ctx.Request.SetRequestURI(uri)
		if token != "" {
			ctx.Request.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			raw, _ := json.Marshal(body)
			ctx.Request.SetBody(raw)
			ctx.Request.Header.Set("Content-Type", "application/json")
		}
		authMiddleware(h)(ctx)
		return ctx
	}

	// Keycloak creates user without specifying role -> should get "analyst" (defaultRole)
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "david.analyst@company.internal",
			"externalId": "keycloak-uuid-5555",
			"active":     true,
			"emails": []map[string]any{
				{"value": "david.analyst@company.internal", "primary": true},
			},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Expected 201 for Keycloak user create, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var userResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &userResp)
		roles := userResp["roles"].([]any)
		if len(roles) == 0 || roles[0].(map[string]any)["value"] != "analyst" {
			t.Errorf("Expected fallback default role analyst, got %v", userResp["roles"])
		}
	}
}

// TestTeamCreationAndUserAssignmentFlow explicitly tests that when an IdP (Entra/Okta)
// provisions a Team/Group and assigns users, the team is properly created and users
// are accurately linked to that team inside UnifAI.
func TestTeamCreationAndUserAssignmentFlow(t *testing.T) {
	store := newSCIMTestStore()
	handler := &WorkspaceHandler{
		workspace: store,
		store: &lib.Config{
			ConfigStore: store,
		},
	}

	testBearer := "entra-teams-sync-token-777"
	scimCfg := scimConfigPayload{
		Enabled:     true,
		Provider:    "entra",
		BearerToken: testBearer,
		Config: map[string]any{
			"defaultRole": "developer",
		},
	}
	rawCfg, _ := json.Marshal(scimCfg)
	_ = store.UpsertWorkspaceSetting(nil, configstore.WorkspaceSettingSCIM, string(rawCfg))

	authMiddleware := handler.scimMiddleware()
	runWithAuth := func(h fasthttp.RequestHandler, method, uri, token string, body any) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(method)
		ctx.Request.SetRequestURI(uri)
		if token != "" {
			ctx.Request.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			raw, _ := json.Marshal(body)
			ctx.Request.SetBody(raw)
			ctx.Request.Header.Set("Content-Type", "application/json")
		}
		authMiddleware(h)(ctx)
		return ctx
	}

	// 1. Provision User 1: Alice
	var aliceID string
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "alice@contoso.com",
			"externalId": "entra-user-alice-01",
			"active":     true,
			"emails":     []map[string]any{{"value": "alice@contoso.com", "primary": true}},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Failed to create user Alice, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		aliceID = resp["id"].(string)
	}

	// 2. Provision User 2: Bob
	var bobID string
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "bob@contoso.com",
			"externalId": "entra-user-bob-02",
			"active":     true,
			"emails":     []map[string]any{{"value": "bob@contoso.com", "primary": true}},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Failed to create user Bob, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		bobID = resp["id"].(string)
	}

	// 3. Entra pushes Group "AI Platform Engineers" with Alice and Bob as members
	var teamID string
	{
		groupPayload := map[string]any{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"displayName": "AI Platform Engineers",
			"externalId":  "entra-group-id-ai-team",
			"members": []map[string]any{
				{"value": aliceID, "display": "alice@contoso.com"},
				{"value": bobID, "display": "bob@contoso.com"},
			},
		}
		ctx := runWithAuth(handler.scimCreateGroup, "POST", "/scim/v2/Groups", testBearer, groupPayload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Failed to create Group, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		teamID = resp["id"].(string)
		if resp["displayName"] != "AI Platform Engineers" {
			t.Fatalf("Expected team name 'AI Platform Engineers', got %v", resp["displayName"])
		}

		// Verify members inside the created group
		members := resp["members"].([]any)
		if len(members) != 2 {
			t.Fatalf("Expected 2 members in team, got %d", len(members))
		}
	}

	// 4. Verify in the workspace store that both users are assigned to the team
	members, err := store.ListTeamMembers(nil, teamID)
	if err != nil {
		t.Fatalf("ListTeamMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("Expected 2 team members in database, got %d", len(members))
	}
	hasAlice, hasBob := false, false
	for _, m := range members {
		if m.UserID == aliceID {
			hasAlice = true
		}
		if m.UserID == bobID {
			hasBob = true
		}
	}
	if !hasAlice || !hasBob {
		t.Fatalf("Expected both Alice and Bob in team, got hasAlice=%v hasBob=%v", hasAlice, hasBob)
	}

	// 5. Provision User 3: Charlie
	var charlieID string
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "charlie@contoso.com",
			"externalId": "entra-user-charlie-03",
			"active":     true,
			"emails":     []map[string]any{{"value": "charlie@contoso.com", "primary": true}},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Failed to create user Charlie, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		charlieID = resp["id"].(string)
	}

	// 6. Entra adds Charlie to team via PATCH (op: "add", path: "members")
	{
		patch := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":   "add",
					"path": "members",
					"value": []map[string]any{
						{"value": charlieID, "display": "charlie@contoso.com"},
					},
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", teamID)
			handler.scimPatchGroup(c)
		}, "PATCH", "/scim/v2/Groups/"+teamID, testBearer, patch)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Failed to add Charlie via PATCH, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 7. Verify team now has 3 members in database
	membersAfterAdd, _ := store.ListTeamMembers(nil, teamID)
	if len(membersAfterAdd) != 3 {
		t.Fatalf("Expected 3 members after adding Charlie, got %d", len(membersAfterAdd))
	}

	// 8. Entra removes Bob from team via PATCH (op: "remove", path: "members[value eq 'bobID']")
	{
		removePatch := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":   "remove",
					"path": "members[value eq \"" + bobID + "\"]",
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", teamID)
			handler.scimPatchGroup(c)
		}, "PATCH", "/scim/v2/Groups/"+teamID, testBearer, removePatch)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Failed to remove Bob via PATCH, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 9. Verify team now has exactly Alice and Charlie, but NOT Bob
	finalMembers, _ := store.ListTeamMembers(nil, teamID)
	if len(finalMembers) != 2 {
		t.Fatalf("Expected 2 members after removing Bob, got %d", len(finalMembers))
	}
	for _, m := range finalMembers {
		if m.UserID == bobID {
			t.Fatalf("Bob should have been removed from the team, but was still present")
		}
	}

	// 10. GET /scim/v2/Groups/{id} reflects updated members list
	{
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", teamID)
			handler.scimGetGroup(c)
		}, "GET", "/scim/v2/Groups/"+teamID, testBearer, nil)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Failed to get Group, got %d", ctx.Response.StatusCode())
		}
		var groupResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &groupResp)
		membersList := groupResp["members"].([]any)
		if len(membersList) != 2 {
			t.Fatalf("Expected 2 members in SCIM group response, got %d", len(membersList))
		}
	}
}

// TestOktaAdvancedProvisioningAndKeycloakFullFlow verifies:
// 1. Okta user provisioning with Name object & complex email query (emails[type eq "work"].value eq "...")
// 2. Pre-existing user adoption by Okta without 500 collision
// 3. Okta group push with schema prefix (urn:ietf:params:scim:schemas:core:2.0:Group:members)
// 4. Okta group member removal with schema prefix
// 5. Okta group deletion (DELETE /scim/v2/Groups/{id})
// 6. Keycloak group creation and role assignment
func TestOktaAdvancedProvisioningAndKeycloakFullFlow(t *testing.T) {
	store := newSCIMTestStore()
	handler := &WorkspaceHandler{
		workspace: store,
		store: &lib.Config{
			ConfigStore: store,
		},
	}

	testBearer := "okta-secret-bearer-token-12345"
	oktaCfg := scimConfigPayload{
		Enabled:     true,
		Provider:    "okta",
		BearerToken: testBearer,
		Config: map[string]any{
			"issuerUrl":   "https://dev-okta.example.com",
			"defaultRole": "developer",
		},
	}
	rawCfg, _ := json.Marshal(oktaCfg)
	_ = store.UpsertWorkspaceSetting(nil, configstore.WorkspaceSettingSCIM, string(rawCfg))

	authMiddleware := handler.scimMiddleware()
	runWithAuth := func(h fasthttp.RequestHandler, method, uri, token string, body any) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(method)
		ctx.Request.SetRequestURI(uri)
		if token != "" {
			ctx.Request.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			raw, _ := json.Marshal(body)
			ctx.Request.SetBody(raw)
			ctx.Request.Header.Set("Content-Type", "application/json")
		}
		authMiddleware(h)(ctx)
		return ctx
	}

	// 1. Pre-register local admin user into UnifAI
	preExistingAdmin := &tables.TableUser{
		ID:        "pre-existing-local-admin-uuid",
		Username:  "admin.local@company.com",
		Email:     "admin.local@company.com",
		Role:      "admin",
		Status:    tables.UserStatusApproved,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = store.CreateUser(nil, preExistingAdmin)

	// 2. Okta attempts to provision the same user (e.g. initial sync of existing users)
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "admin.local@company.com",
			"externalId": "okta-admin-id-001",
			"active":     true,
			"name": map[string]any{
				"formatted":  "Local Administrator",
				"familyName": "Administrator",
				"givenName":  "Local",
			},
			"emails": []map[string]any{
				{"value": "admin.local@company.com", "primary": true, "type": "work"},
			},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		// Should succeed (200 OK) and adopt externalId, NOT fail with 500 error!
		if ctx.Response.StatusCode() != http.StatusOK && ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Expected 200 or 201 for existing user adoption, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if resp["externalId"] != "okta-admin-id-001" {
			t.Errorf("Expected externalId okta-admin-id-001, got %v", resp["externalId"])
		}
		if resp["name"] == nil {
			t.Errorf("Expected name object in SCIM user response, got nil")
		}
	}

	// 3. Okta queries user with complex email filter: emails[type eq "work"].value eq "admin.local@company.com"
	{
		filterURI := `/scim/v2/Users?filter=emails[type%20eq%20"work"].value%20eq%20"admin.local@company.com"`
		ctx := runWithAuth(handler.scimListUsers, "GET", filterURI, testBearer, nil)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for Okta complex email filter query, got %d", ctx.Response.StatusCode())
		}
		var listResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &listResp)
		if int(listResp["totalResults"].(float64)) != 1 {
			t.Fatalf("Expected totalResults=1 for Okta work email filter, got %v", listResp["totalResults"])
		}
	}

	// 4. Okta provisions a new user: Sarah
	var sarahID string
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "sarah.connor@company.com",
			"externalId": "okta-sarah-999",
			"active":     true,
			"emails": []map[string]any{
				{"value": "sarah.connor@company.com", "primary": true, "type": "work"},
			},
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Expected 201 for Sarah creation, got %d", ctx.Response.StatusCode())
		}
		var userResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &userResp)
		sarahID = userResp["id"].(string)
	}

	// 5. Okta creates a Group "Data Science"
	var groupID string
	{
		payload := map[string]any{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
			"displayName": "Data Science",
			"externalId":  "okta-group-data-sci-77",
			"members": []map[string]any{
				{"value": preExistingAdmin.ID},
			},
		}
		ctx := runWithAuth(handler.scimCreateGroup, "POST", "/scim/v2/Groups", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Expected 201 for group create, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var gResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &gResp)
		groupID = gResp["id"].(string)
	}

	// 6. Okta adds Sarah to "Data Science" using schema-prefixed path
	{
		patch := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":   "add",
					"path": "urn:ietf:params:scim:schemas:core:2.0:Group:members",
					"value": []map[string]any{
						{"value": sarahID},
					},
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", groupID)
			handler.scimPatchGroup(c)
		}, "PATCH", "/scim/v2/Groups/"+groupID, testBearer, patch)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for Okta schema-prefixed group add, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}

		// Verify 2 members in team
		members, _ := store.ListTeamMembers(nil, groupID)
		if len(members) != 2 {
			t.Fatalf("Expected 2 members after Okta add, got %d", len(members))
		}
	}

	// 7. Okta removes local admin using schema-prefixed path filter:
	// urn:ietf:params:scim:schemas:core:2.0:Group:members[value eq "adminID"]
	{
		patch := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":   "remove",
					"path": "urn:ietf:params:scim:schemas:core:2.0:Group:members[value eq \"" + preExistingAdmin.ID + "\"]",
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", groupID)
			handler.scimPatchGroup(c)
		}, "PATCH", "/scim/v2/Groups/"+groupID, testBearer, patch)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for Okta schema-prefixed group remove, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}

		// Verify only Sarah remains in team
		members, _ := store.ListTeamMembers(nil, groupID)
		if len(members) != 1 || members[0].UserID != sarahID {
			t.Fatalf("Expected only Sarah in team after removal, got %+v", members)
		}
	}

	// 8. Okta deletes group: DELETE /scim/v2/Groups/{id} -> 204 No Content
	{
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", groupID)
			handler.scimDeleteGroup(c)
		}, "DELETE", "/scim/v2/Groups/"+groupID, testBearer, nil)

		if ctx.Response.StatusCode() != http.StatusNoContent {
			t.Fatalf("Expected 204 for group delete, got %d", ctx.Response.StatusCode())
		}

		// Confirm group is deleted
		_, err := store.GetTeam(nil, groupID)
		if err == nil {
			t.Fatalf("Expected group to be deleted, but it was found")
		}
	}
}

// TestSCIMEmailAndDefaultRoleConfiguration verifies:
// 1. When defaultRole is set to "sub_admin" in SCIM config, provisioned users without roles get "sub_admin"
// 2. When defaultRole is set to "admin" or "user", provisioned users receive that exact role
// 3. When IdP provides userName as email without an emails array, email is automatically populated
// 4. When email is patched via SCIM PATCH, user.Email updates correctly
func TestSCIMEmailAndDefaultRoleConfiguration(t *testing.T) {
	store := newSCIMTestStore()
	handler := &WorkspaceHandler{
		workspace: store,
		store: &lib.Config{
			ConfigStore: store,
		},
	}

	testBearer := "test-role-token-888"

	authMiddleware := handler.scimMiddleware()
	runWithAuth := func(h fasthttp.RequestHandler, method, uri, token string, body any) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(method)
		ctx.Request.SetRequestURI(uri)
		if token != "" {
			ctx.Request.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			raw, _ := json.Marshal(body)
			ctx.Request.SetBody(raw)
			ctx.Request.Header.Set("Content-Type", "application/json")
		}
		authMiddleware(h)(ctx)
		return ctx
	}

	// 1. Configure SCIM with defaultRole = "sub_admin" (like selected in UI dropdown)
	{
		cfg := scimConfigPayload{
			Enabled:     true,
			Provider:    "okta",
			BearerToken: testBearer,
			Config: map[string]any{
				"defaultRole": "sub_admin",
			},
		}
		raw, _ := json.Marshal(cfg)
		_ = store.UpsertWorkspaceSetting(nil, configstore.WorkspaceSettingSCIM, string(raw))
	}

	// 2. IdP provisions user without explicit roles and without emails array (only userName)
	var subAdminID string
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "manager.dev@enterprise.com",
			"externalId": "idp-user-subadmin-1",
			"active":     true,
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Failed to create user with defaultRole sub_admin, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		subAdminID = resp["id"].(string)

		// Verify role is sub_admin
		roles := resp["roles"].([]any)
		if len(roles) == 0 || roles[0].(map[string]any)["value"] != "sub_admin" {
			t.Fatalf("Expected role sub_admin from defaultRole setting, got %+v", roles)
		}

		// Verify email was populated from userName
		emails := resp["emails"].([]any)
		if len(emails) == 0 || emails[0].(map[string]any)["value"] != "manager.dev@enterprise.com" {
			t.Fatalf("Expected email manager.dev@enterprise.com, got %+v", emails)
		}

		// Verify in DB directly
		dbUser, _ := store.GetUserByID(nil, subAdminID)
		if dbUser == nil || dbUser.Role != "sub_admin" || dbUser.Email != "manager.dev@enterprise.com" {
			t.Fatalf("Database user state mismatch: role=%v email=%v", dbUser.Role, dbUser.Email)
		}
	}

	// 3. Patch email using SCIM PATCH path: emails[type eq 'work'].value
	{
		patch := map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
			"Operations": []map[string]any{
				{
					"op":    "replace",
					"path":  "emails[type eq 'work'].value",
					"value": "manager.updated@enterprise.com",
				},
			},
		}
		ctx := runWithAuth(func(c *fasthttp.RequestCtx) {
			c.SetUserValue("id", subAdminID)
			handler.scimPatchUser(c)
		}, "PATCH", "/scim/v2/Users/"+subAdminID, testBearer, patch)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Failed to patch user email, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		dbUser, _ := store.GetUserByID(nil, subAdminID)
		if dbUser.Email != "manager.updated@enterprise.com" {
			t.Fatalf("Expected patched email manager.updated@enterprise.com, got %s", dbUser.Email)
		}
	}

	// 4. Change defaultRole in UI to "admin"
	{
		cfg := scimConfigPayload{
			Enabled:     true,
			Provider:    "entra",
			BearerToken: testBearer,
			Config: map[string]any{
				"defaultRole": "admin",
			},
		}
		raw, _ := json.Marshal(cfg)
		_ = store.UpsertWorkspaceSetting(nil, configstore.WorkspaceSettingSCIM, string(raw))
	}

	// 5. Provision next user -> should receive "admin"
	{
		payload := map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":   "new.admin@enterprise.com",
			"externalId": "idp-user-admin-2",
			"active":     true,
		}
		ctx := runWithAuth(handler.scimCreateUser, "POST", "/scim/v2/Users", testBearer, payload)
		if ctx.Response.StatusCode() != http.StatusCreated {
			t.Fatalf("Failed to create user with defaultRole admin, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		roles := resp["roles"].([]any)
		if len(roles) == 0 || roles[0].(map[string]any)["value"] != "admin" {
			t.Fatalf("Expected role admin, got %+v", roles)
		}
	}
}




