package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

// Helper: logs in as admin and returns the session token
func getAdminSessionToken(t *testing.T, handler *SessionHandler) string {
	t.Helper()
	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "admin",
		"password": "AdminPass123!",
	}, "192.168.1.1")
	handler.login(ctx)
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Failed to login as admin: %d - %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
	token, _ := extractResponseCookie(ctx, "token")
	if token == "" {
		t.Fatal("Admin login did not yield token")
	}
	return token
}

// Helper: logs in as regular user and returns session token
func getUserSessionToken(t *testing.T, store *memoryConfigStore, handler *SessionHandler, username, password string) string {
	t.Helper()
	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": username,
		"password": password,
	}, "192.168.1.2")
	handler.login(ctx)
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Failed to login as user: %d - %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}
	token, _ := extractResponseCookie(ctx, "token")
	if token == "" {
		t.Fatal("User login did not yield token")
	}
	return token
}

// 1. Admin successfully creates a new user with full governance parameters
func TestUserManagement_CreateUser_Admin_Success(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	ctx := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username":         "alice_engineer",
		"email":            "alice@rakshatech.io",
		"password":         "ComplexPass2026!",
		"role":             "user",
		"budget":           100.5,
		"rate_limit":       120,
		"allowed_sections": "dashboard,observability,models",
	}, "192.168.1.1")
	ctx.Request.Header.Set("Authorization", "Bearer "+adminToken)

	handler.createUser(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK on user creation, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
	}

	var resp map[string]any
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Security: Password MUST be stripped from response
	if pass, ok := resp["password"]; ok && pass != "" {
		t.Errorf("Password must not be returned in API response, got %v", pass)
	}
	if resp["username"] != "alice_engineer" {
		t.Errorf("Expected username 'alice_engineer', got %v", resp["username"])
	}
	if resp["role"] != "user" {
		t.Errorf("Expected role 'user', got %v", resp["role"])
	}

	// Verify user persisted in database with hashed password and approved status
	user, err := store.GetUserByUsername(context.Background(), "alice_engineer")
	if err != nil || user == nil {
		t.Fatalf("User was not stored in configstore: %v", err)
	}
	if user.Password == "ComplexPass2026!" {
		t.Errorf("Password was stored in plaintext!")
	}
	if user.Status != tables.UserStatusApproved {
		t.Errorf("Admin-created user should be immediately approved, got %s", user.Status)
	}
}

// 2. Non-Admin user is strictly forbidden from creating users
func TestUserManagement_CreateUser_NonAdmin_Forbidden(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// Create and login a standard user
	userToken := getUserSessionToken(t, store, handler, "admin", "AdminPass123!") // get admin first to seed a normal user
	ctxSeed := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username": "bob_regular",
		"email":    "bob@rakshatech.io",
		"password": "Password123!",
		"role":     "user",
	}, "192.168.1.1")
	ctxSeed.Request.Header.Set("Authorization", "Bearer "+userToken)
	handler.createUser(ctxSeed)

	// Now bob logs in
	bobToken := getUserSessionToken(t, store, handler, "bob_regular", "Password123!")

	// Bob attempts to create another user
	ctxAttack := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username": "hacker_sub_user",
		"email":    "hacker@evil.com",
		"password": "EvilPassword123!",
		"role":     "admin",
	}, "192.168.1.5")
	ctxAttack.Request.Header.Set("Authorization", "Bearer "+bobToken)

	handler.createUser(ctxAttack)

	if ctxAttack.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("Non-admin must be forbidden (403) from creating users, got %d", ctxAttack.Response.StatusCode())
	}
}

// 3. User creation rejects weak passwords (Password Policy Enforced)
func TestUserManagement_CreateUser_WeakPassword_Policy(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	weakPasswords := []string{
		"short",          // < 8 chars
		"alllowercase1!", // no uppercase
		"ALLUPPERCASE1!", // no lowercase
		"NoDigitsHere!!", // no digits
		"NoSpecialChar1", // no special chars
	}

	for _, weak := range weakPasswords {
		ctx := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
			"username": "user_" + weak[:4],
			"email":    "test@rakshatech.io",
			"password": weak,
			"role":     "user",
		}, "192.168.1.1")
		ctx.Request.Header.Set("Authorization", "Bearer "+adminToken)

		handler.createUser(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
			t.Errorf("Weak password %q should return 400 Bad Request, got %d", weak, ctx.Response.StatusCode())
		}
	}
}

// 4. User creation rejects invalid email formats
func TestUserManagement_CreateUser_InvalidEmail(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	ctx := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username": "charlie_test",
		"email":    "not-an-email-at-all",
		"password": "ComplexPassword123!",
		"role":     "user",
	}, "192.168.1.1")
	ctx.Request.Header.Set("Authorization", "Bearer "+adminToken)

	handler.createUser(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for invalid email, got %d", ctx.Response.StatusCode())
	}
}

// 5. User creation forbids shadowing built-in admin account
func TestUserManagement_CreateUser_ShadowAdminBlocked(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	ctx := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username": "admin", // Matches built-in administrator
		"email":    "shadow@evil.com",
		"password": "NewAdminPass123!",
		"role":     "admin",
	}, "192.168.1.1")
	ctx.Request.Header.Set("Authorization", "Bearer "+adminToken)

	handler.createUser(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request when shadowing built-in admin, got %d", ctx.Response.StatusCode())
	}
	if !strings.Contains(string(ctx.Response.Body()), "matches the built-in admin account") {
		t.Errorf("Expected shadow warning, got %s", string(ctx.Response.Body()))
	}
}

// 6. Admin can list all users and verify passwords are never exposed
func TestUserManagement_GetUsers_Admin_Success(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	// Seed 2 users
	ctx1 := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username": "engineer_one",
		"email":    "one@rakshatech.io",
		"password": "StrongPassword123!",
		"role":     "user",
	}, "192.168.1.1")
	ctx1.Request.Header.Set("Authorization", "Bearer "+adminToken)
	handler.createUser(ctx1)

	ctx2 := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username": "engineer_two",
		"email":    "two@rakshatech.io",
		"password": "StrongPassword123!",
		"role":     "user",
	}, "192.168.1.1")
	ctx2.Request.Header.Set("Authorization", "Bearer "+adminToken)
	handler.createUser(ctx2)

	// Get users
	listCtx := makeFastHTTPCtx("GET", "/api/session/users", nil, "192.168.1.1")
	listCtx.Request.Header.Set("Authorization", "Bearer "+adminToken)

	handler.getUsers(listCtx)

	if listCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK on listing users, got %d", listCtx.Response.StatusCode())
	}

	var users []map[string]any
	if err := json.Unmarshal(listCtx.Response.Body(), &users); err != nil {
		t.Fatalf("Failed to parse users list JSON: %v", err)
	}

	if len(users) < 2 {
		t.Fatalf("Expected at least 2 users, got %d", len(users))
	}

	// Verify NO passwords leaked in listing
	for _, u := range users {
		if pass, has := u["password"]; has && pass != "" {
			t.Errorf("Password must not be present in user list for user %v", u["username"])
		}
	}
}

// 7. Non-Admin user cannot list users
func TestUserManagement_GetUsers_NonAdmin_Forbidden(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// Unauthenticated request
	anonCtx := makeFastHTTPCtx("GET", "/api/session/users", nil, "192.168.1.9")
	handler.getUsers(anonCtx)

	if anonCtx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("Unauthenticated getUsers should return 403 Forbidden, got %d", anonCtx.Response.StatusCode())
	}
}

// 8. Delete user by ID and verify session purge
func TestUserManagement_DeleteUser_Admin(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	// Create user
	createCtx := makeFastHTTPCtx("POST", "/api/session/users", map[string]any{
		"username": "delete_me_user",
		"email":    "delete@rakshatech.io",
		"password": "StrongPassword123!",
		"role":     "user",
	}, "192.168.1.1")
	createCtx.Request.Header.Set("Authorization", "Bearer "+adminToken)
	handler.createUser(createCtx)

	var created map[string]any
	_ = json.Unmarshal(createCtx.Response.Body(), &created)
	userID := created["id"].(string)

	// User logs in to get active session
	userToken := getUserSessionToken(t, store, handler, "delete_me_user", "StrongPassword123!")

	// Delete user
	delCtx := makeFastHTTPCtx("DELETE", "/api/session/users/"+userID, nil, "192.168.1.1")
	delCtx.SetUserValue("id", userID)
	delCtx.Request.Header.Set("Authorization", "Bearer "+adminToken)

	handler.deleteUser(delCtx)

	if delCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK on user delete, got %d: %s", delCtx.Response.StatusCode(), string(delCtx.Response.Body()))
	}

	// Verify user removed from store
	u, _ := store.GetUserByID(context.Background(), userID)
	if u != nil {
		t.Errorf("User should be deleted from store")
	}

	// Verify active session was immediately revoked (CWE-613)
	sess, _ := store.GetSession(context.Background(), userToken)
	if sess != nil {
		t.Errorf("Deleted user's session should have been invalidated")
	}
}

// 9. Full Approval Workflow: Public Register -> Pending -> Admin Approves -> User Logs In
func TestUserManagement_ApproveUser_Workflow(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	adminToken := getAdminSessionToken(t, handler)

	// Step 1: Self registration
	regCtx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
		"username": "candidate_dev",
		"email":    "candidate@rakshatech.io",
		"password": "StrongPassword123!",
	}, "192.168.1.20")
	handler.register(regCtx)

	if regCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Registration failed: %d", regCtx.Response.StatusCode())
	}
	var regResp map[string]any
	_ = json.Unmarshal(regCtx.Response.Body(), &regResp)
	userID := regResp["id"].(string)

	// Step 2: Login before approval -> 403 Forbidden
	loginFailCtx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "candidate_dev",
		"password": "StrongPassword123!",
	}, "192.168.1.20")
	handler.login(loginFailCtx)
	if loginFailCtx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Fatalf("Expected 403 before approval, got %d", loginFailCtx.Response.StatusCode())
	}

	// Step 3: Admin approves user
	approveCtx := makeFastHTTPCtx("POST", "/api/session/users/"+userID+"/approve", nil, "192.168.1.1")
	approveCtx.SetUserValue("id", userID)
	approveCtx.Request.Header.Set("Authorization", "Bearer "+adminToken)

	handler.approveUser(approveCtx)

	if approveCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK on user approval, got %d: %s", approveCtx.Response.StatusCode(), string(approveCtx.Response.Body()))
	}

	// Step 4: Login after approval -> 200 OK
	loginSuccessCtx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
		"username": "candidate_dev",
		"password": "StrongPassword123!",
	}, "192.168.1.20")
	handler.login(loginSuccessCtx)

	if loginSuccessCtx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("Expected 200 OK after approval, got %d: %s", loginSuccessCtx.Response.StatusCode(), string(loginSuccessCtx.Response.Body()))
	}
}
