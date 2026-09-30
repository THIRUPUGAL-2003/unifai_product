package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/unifai/unifai/framework/encrypt"
)

// TestRegistrationWorkflow tests user account creation, validations, and security boundaries.
func TestRegistrationWorkflow(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// 1. Weak password rejected
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
			"username": "newuser1",
			"email":    "user1@example.com",
			"password": "weak",
		}, "192.168.1.50")
		handler.register(ctx)
		if ctx.Response.StatusCode() != http.StatusBadRequest {
			t.Errorf("Expected 400 for weak password, got %d", ctx.Response.StatusCode())
		}
	}

	// 2. Invalid email format rejected
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
			"username": "newuser2",
			"email":    "invalid-email-no-at",
			"password": "StrongPassword123!",
		}, "192.168.1.50")
		handler.register(ctx)
		if ctx.Response.StatusCode() != http.StatusBadRequest {
			t.Errorf("Expected 400 for invalid email, got %d", ctx.Response.StatusCode())
		}
	}

	// 3. Forbid registering "admin" username (Shadowing prevention)
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
			"username": "admin",
			"email":    "fakeadmin@example.com",
			"password": "StrongPassword123!",
		}, "192.168.1.50")
		handler.register(ctx)
		if ctx.Response.StatusCode() != http.StatusConflict {
			t.Errorf("Expected 409 Conflict for registering 'admin' username, got %d", ctx.Response.StatusCode())
		}
	}

	// 4. Successful registration -> Enters UserStatusPending
	var createdUserID string
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
			"username": "alice",
			"email":    "alice@example.com",
			"password": "StrongPassword123!",
		}, "192.168.1.50")
		handler.register(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for valid registration, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if resp["status"] != tables.UserStatusPending {
			t.Errorf("Expected status %s, got %v", tables.UserStatusPending, resp["status"])
		}
		if resp["role"] != "user" {
			t.Errorf("Expected role 'user', got %v", resp["role"])
		}
		createdUserID = fmt.Sprintf("%v", resp["id"])
	}

	// 5. Duplicate registration rejected with 409 Conflict
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
			"username": "alice",
			"email":    "alice_different@example.com",
			"password": "StrongPassword123!",
		}, "192.168.1.50")
		handler.register(ctx)
		if ctx.Response.StatusCode() != http.StatusConflict {
			t.Errorf("Expected 409 Conflict for duplicate username, got %d", ctx.Response.StatusCode())
		}
	}

	// 6. Login as Pending user MUST FAIL with 403 Forbidden Pending Approval
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": "alice",
			"password": "StrongPassword123!",
		}, "192.168.1.51")
		handler.login(ctx)
		if ctx.Response.StatusCode() != http.StatusForbidden {
			t.Errorf("Expected 403 for pending user login, got %d", ctx.Response.StatusCode())
		}
		if !strings.Contains(string(ctx.Response.Body()), "waiting for admin approval") {
			t.Errorf("Expected waiting for admin approval message, got %s", string(ctx.Response.Body()))
		}
	}

	// 7. Admin Approves Alice -> Login MUST SUCCEED
	{
		adminToken := getAdminSessionToken(t, handler)
		ctx := makeFastHTTPCtx("POST", "/api/session/users/"+createdUserID+"/approve", nil, "192.168.1.1")
		ctx.Request.Header.Set("Authorization", "Bearer "+adminToken)
		ctx.SetUserValue("id", createdUserID)
		handler.approveUser(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for admin approval, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}

		// Now Alice can log in successfully
		loginCtx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": "alice",
			"password": "StrongPassword123!",
		}, "192.168.1.51")
		handler.login(loginCtx)
		if loginCtx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for approved user login, got %d: %s", loginCtx.Response.StatusCode(), string(loginCtx.Response.Body()))
		}
		var loginResp map[string]any
		_ = json.Unmarshal(loginCtx.Response.Body(), &loginResp)
		if loginResp["message"] != "Login successful" {
			t.Errorf("Expected message 'Login successful', got %v", loginResp["message"])
		}
		token, _ := extractResponseCookie(loginCtx, "token")
		if token == "" {
			t.Errorf("Expected session token cookie to be set")
		}
	}
}

// TestForgotPasswordAndResetFlow tests OTP generation, OTP verification, signed reset token issuance, and password update.
func TestForgotPasswordAndResetFlow(t *testing.T) {
	_ = os.Setenv("PASSWORD_RESET_SECRET", "super-secret-key-that-is-at-least-32-chars-long!")
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// Create an approved user "bob"
	hashedPwd, _ := encrypt.Hash("InitialPassword123!")
	bobUser := &tables.TableUser{
		ID:        uuid.New().String(),
		Username:  "bob",
		Email:     "bob@example.com",
		Password:  hashedPwd,
		Role:      "user",
		Status:    tables.UserStatusApproved,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateUser(nil, bobUser)

	// 1. Request Forgot Password for bob -> returns generic success message (no user enumeration)
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/forgot-password", map[string]string{
			"email": "bob@example.com",
		}, "192.168.1.60")
		handler.forgotPassword(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for forgot password, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if !strings.Contains(fmt.Sprintf("%v", resp["message"]), "code was sent") {
			t.Errorf("Expected generic code sent message, got %v", resp["message"])
		}
	}

	// 2. Forgot Password for non-existent user ALSO returns identical generic success (Anti-enumeration security)
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/forgot-password", map[string]string{
			"email": "unknown-ghost-user@example.com",
		}, "192.168.1.61")
		handler.forgotPassword(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for non-existent user forgot password, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if !strings.Contains(fmt.Sprintf("%v", resp["message"]), "code was sent") {
			t.Errorf("Expected anti-enumeration generic message, got %v", resp["message"])
		}
	}

	// 3. Inspect the created OTP for bob in mock store
	latestOTP, err := store.GetLatestPasswordResetOTP(nil, "bob")
	if err != nil || latestOTP == nil {
		t.Fatalf("Expected OTP record in store for bob, got %v", err)
	}

	// Emulate knowing the OTP: Generate known OTP hash and insert
	testOTP := "123456"
	testOTPHash, _ := encrypt.Hash(testOTP)
	latestOTP.OTPHash = testOTPHash
	_ = store.CreatePasswordResetOTP(nil, latestOTP)

	// 4. Verify OTP with WRONG OTP -> fails with 401
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/verify-otp", map[string]string{
			"username": "bob",
			"otp":      "999999",
		}, "192.168.1.62")
		handler.verifyOTP(ctx)
		if ctx.Response.StatusCode() != http.StatusUnauthorized {
			t.Errorf("Expected 401 for incorrect OTP, got %d", ctx.Response.StatusCode())
		}
	}

	// 5. Verify OTP with CORRECT OTP -> returns valid signed reset_token and burns raw OTP
	var signedResetToken string
	{
		// Use fresh OTP for bob
		latestOTP, _ = store.GetLatestPasswordResetOTP(nil, "bob")
		latestOTP.OTPHash = testOTPHash
		latestOTP.FailedAttempts = 0
		_ = store.CreatePasswordResetOTP(nil, latestOTP)

		ctx := makeFastHTTPCtx("POST", "/api/session/verify-otp", map[string]string{
			"username": "bob",
			"otp":      testOTP,
		}, "192.168.1.63")
		handler.verifyOTP(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for valid OTP verify, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if resp["valid"] != true {
			t.Errorf("Expected valid=true, got %v", resp["valid"])
		}
		signedResetToken = fmt.Sprintf("%v", resp["reset_token"])
		if signedResetToken == "" || !strings.Contains(signedResetToken, ".") {
			t.Fatalf("Expected signed token with dot format, got %s", signedResetToken)
		}
	}

	// 6. Attempting to RE-USE the same raw OTP must fail (Replay attack prevention)
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/verify-otp", map[string]string{
			"username": "bob",
			"otp":      testOTP,
		}, "192.168.1.64")
		handler.verifyOTP(ctx)
		if ctx.Response.StatusCode() != http.StatusUnauthorized {
			t.Errorf("Expected 401 when replaying burned OTP, got %d", ctx.Response.StatusCode())
		}
	}

	// 7. Reset password with REUSED current password -> rejected (Must be different)
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/reset-password", map[string]string{
			"username":     "bob",
			"reset_token":  signedResetToken,
			"new_password": "InitialPassword123!",
		}, "192.168.1.65")
		handler.resetPassword(ctx)
		if ctx.Response.StatusCode() != http.StatusBadRequest {
			t.Errorf("Expected 400 when reusing current password, got %d", ctx.Response.StatusCode())
		}
	}

	// 8. Reset password with TAMPERED reset_token -> rejected with 401
	{
		tamperedToken := signedResetToken[:len(signedResetToken)-5] + "aaaaa"
		ctx := makeFastHTTPCtx("POST", "/api/session/reset-password", map[string]string{
			"username":     "bob",
			"reset_token":  tamperedToken,
			"new_password": "BrandNewPassword123!",
		}, "192.168.1.66")
		handler.resetPassword(ctx)
		if ctx.Response.StatusCode() != http.StatusUnauthorized {
			t.Errorf("Expected 401 for tampered reset token, got %d", ctx.Response.StatusCode())
		}
	}

	// 9. Reset password with VALID reset_token and strong new password -> SUCCESS
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/reset-password", map[string]string{
			"username":     "bob",
			"reset_token":  signedResetToken,
			"new_password": "BrandNewPassword123!",
		}, "192.168.1.67")
		handler.resetPassword(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for successful password reset, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 10. Login with OLD password -> MUST FAIL
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": "bob",
			"password": "InitialPassword123!",
		}, "192.168.1.68")
		handler.login(ctx)
		if ctx.Response.StatusCode() != http.StatusUnauthorized {
			t.Errorf("Expected 401 when logging in with old password, got %d", ctx.Response.StatusCode())
		}
	}

	// 11. Login with NEW password -> MUST SUCCEED
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": "bob",
			"password": "BrandNewPassword123!",
		}, "192.168.1.68")
		handler.login(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 when logging in with new password, got %d: %s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var loginResp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &loginResp)
		if loginResp["message"] != "Login successful" {
			t.Errorf("Expected message 'Login successful', got %v", loginResp["message"])
		}
		token, _ := extractResponseCookie(ctx, "token")
		if token == "" {
			t.Errorf("Expected session token cookie to be set")
		}
	}
}

// TestForgotUsernameFlow tests recovery of username via registered email.
func TestForgotUsernameFlow(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	// Create user "charlie"
	charlieUser := &tables.TableUser{
		ID:        uuid.New().String(),
		Username:  "charlie",
		Email:     "charlie@example.com",
		Password:  "hashed",
		Role:      "user",
		Status:    tables.UserStatusApproved,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = store.CreateUser(nil, charlieUser)

	// 1. Empty email -> 400 Bad Request
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/forgot-username", map[string]string{
			"email": "",
		}, "192.168.1.70")
		handler.forgotUsername(ctx)
		if ctx.Response.StatusCode() != http.StatusBadRequest {
			t.Errorf("Expected 400 for empty email, got %d", ctx.Response.StatusCode())
		}
	}

	// 2. Valid email -> 200 OK with generic response (Anti-enumeration)
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/forgot-username", map[string]string{
			"email": "charlie@example.com",
		}, "192.168.1.71")
		handler.forgotUsername(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Errorf("Expected 200 for valid email, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if !strings.Contains(fmt.Sprintf("%v", resp["message"]), "your username was sent") {
			t.Errorf("Expected username sent message, got %v", resp["message"])
		}
	}

	// 3. Unknown email -> 200 OK with identical generic response (Anti-enumeration)
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/forgot-username", map[string]string{
			"email": "unknown@example.com",
		}, "192.168.1.72")
		handler.forgotUsername(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Errorf("Expected 200 for unknown email, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if !strings.Contains(fmt.Sprintf("%v", resp["message"]), "your username was sent") {
			t.Errorf("Expected identical generic response, got %v", resp["message"])
		}
	}
}

// TestSuperAdminForgotPasswordAndResetFlow tests the end-to-end forgot password,
// OTP verification, and password reset flow for the built-in bootstrap Super Admin.
func TestSuperAdminForgotPasswordAndResetFlow(t *testing.T) {
	_ = os.Setenv("PASSWORD_RESET_SECRET", "super-secret-key-that-is-at-least-32-chars-long!")
	_ = os.Setenv("ADMIN_EMAIL", "admin@yespanchi.com")
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	initialAdminPwd, _ := encrypt.Hash("OldSuperAdminPass123!")
	_ = store.UpdateAuthConfig(nil, &configstore.AuthConfig{
		AdminUserName: schemas.NewSecretVar("admin"),
		AdminPassword: schemas.NewSecretVar(initialAdminPwd),
		AdminEmail:    schemas.NewSecretVar("admin@yespanchi.com"),
		IsEnabled:     true,
	})

	// 1. Forgot password using admin email
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/forgot-password", map[string]string{
			"email": "admin@yespanchi.com",
		}, "192.168.1.80")
		handler.forgotPassword(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for super admin forgot password, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if !strings.Contains(fmt.Sprintf("%v", resp["message"]), "code was sent") {
			t.Errorf("Expected code sent message, got %v", resp["message"])
		}
	}

	// 2. Fetch the created OTP for Super Admin
	latestOTP, err := store.GetLatestPasswordResetOTP(nil, "admin")
	if err != nil || latestOTP == nil {
		latestOTP, err = store.GetLatestPasswordResetOTP(nil, "admin@yespanchi.com")
	}
	if err != nil || latestOTP == nil {
		t.Fatalf("Expected OTP record in store for super admin, got %v", err)
	}

	testOTP := "987654"
	testOTPHash, _ := encrypt.Hash(testOTP)
	latestOTP.OTPHash = testOTPHash
	_ = store.CreatePasswordResetOTP(nil, latestOTP)

	// 3. Verify OTP using email
	var resetToken string
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/verify-otp", map[string]string{
			"email": "admin@yespanchi.com",
			"otp":   testOTP,
		}, "192.168.1.81")
		handler.verifyOTP(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 verifying super admin OTP, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if resp["valid"] != true {
			t.Fatalf("Expected valid=true, got %v", resp["valid"])
		}
		resetToken = fmt.Sprintf("%v", resp["reset_token"])
		if resetToken == "" {
			t.Fatalf("Expected non-empty reset token")
		}
	}

	// 4. Reset password
	newAdminPassword := "BrandNewAdminPass456!"
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/reset-password", map[string]string{
			"email":        "admin@yespanchi.com",
			"reset_token":  resetToken,
			"new_password": newAdminPassword,
		}, "192.168.1.82")
		handler.resetPassword(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 resetting super admin password, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 5. Verify AuthConfig has the updated password
	authConfig, err := store.GetAuthConfig(nil)
	if err != nil || authConfig == nil {
		t.Fatalf("Failed to fetch auth config after reset: %v", err)
	}
	match, err := encrypt.CompareHash(authConfig.AdminPassword.GetValue(), newAdminPassword)
	if err != nil || !match {
		t.Fatalf("Expected authConfig password hash to match new password, match=%v err=%v", match, err)
	}

	// 6. Verify Super Admin can log in with new password using "admin"
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": "admin",
			"password": newAdminPassword,
		}, "192.168.1.83")
		handler.login(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 logging in with new admin password, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 7. Verify Super Admin can ALSO log in with email "admin@yespanchi.com"
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{
			"username": "admin@yespanchi.com",
			"password": newAdminPassword,
		}, "192.168.1.84")
		handler.login(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 logging in with admin email, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 8. Verify forgotUsername works for Super Admin email
	{
		ctx := makeFastHTTPCtx("POST", "/api/session/forgot-username", map[string]string{
			"email": "admin@yespanchi.com",
		}, "192.168.1.85")
		handler.forgotUsername(ctx)
		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for super admin forgot-username, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if !strings.Contains(fmt.Sprintf("%v", resp["message"]), "your username was sent") {
			t.Errorf("Expected username sent message, got %v", resp["message"])
		}
	}
}

