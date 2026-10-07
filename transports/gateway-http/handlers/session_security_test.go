package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/encrypt"
	"github.com/valyala/fasthttp"
)

func loginStatus(h *SessionHandler, username, password, ip string) int {
	ctx := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{"username": username, "password": password}, ip)
	h.login(ctx)
	return ctx.Response.StatusCode()
}

// A locked-out user who proves email ownership via the reset code must be able to
// sign in with the new password immediately.
func TestResetPassword_LiftsLoginLockout(t *testing.T) {
	_ = os.Setenv("PASSWORD_RESET_SECRET", "super-secret-key-that-is-at-least-32-chars-long!")
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)

	hashed, _ := encrypt.Hash("InitialPassword123!")
	_ = store.CreateUser(nil, &tables.TableUser{
		ID: uuid.New().String(), Username: "dave", Email: "dave@example.com", Password: hashed,
		Role: "user", Status: tables.UserStatusApproved, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	for i := 0; i < loginMaxFailedAttempts; i++ {
		loginStatus(handler, "dave", "WrongPassword1!", fmt.Sprintf("10.20.0.%d", i+1))
	}
	if code := loginStatus(handler, "dave", "InitialPassword123!", "10.20.0.50"); code != http.StatusTooManyRequests {
		t.Fatalf("expected account to be locked (429), got %d", code)
	}

	fp := makeFastHTTPCtx("POST", "/api/session/forgot-password", map[string]string{"email": "dave@example.com"}, "10.20.1.1")
	handler.forgotPassword(fp)
	otpRow, err := store.GetLatestPasswordResetOTP(nil, "dave")
	if err != nil || otpRow == nil {
		t.Fatalf("forgot-password must still issue a code while the account is locked: %v", err)
	}
	otpRow.OTPHash, _ = encrypt.Hash("246810")
	_ = store.CreatePasswordResetOTP(nil, otpRow)

	verify := makeFastHTTPCtx("POST", "/api/session/verify-otp", map[string]string{"email": "dave@example.com", "otp": "246810"}, "10.20.1.2")
	handler.verifyOTP(verify)
	if verify.Response.StatusCode() != http.StatusOK {
		t.Fatalf("verify-otp failed: %d %s", verify.Response.StatusCode(), verify.Response.Body())
	}
	var vr map[string]any
	_ = json.Unmarshal(verify.Response.Body(), &vr)

	reset := makeFastHTTPCtx("POST", "/api/session/reset-password", map[string]string{
		"email": "dave@example.com", "reset_token": fmt.Sprint(vr["reset_token"]), "new_password": "BrandNewPass456!",
	}, "10.20.1.3")
	handler.resetPassword(reset)
	if reset.Response.StatusCode() != http.StatusOK {
		t.Fatalf("reset failed: %d %s", reset.Response.StatusCode(), reset.Response.Body())
	}

	if code := loginStatus(handler, "dave", "BrandNewPass456!", "10.20.0.60"); code != http.StatusOK {
		t.Fatalf("expected login with new password right after reset, got %d", code)
	}
}

// "admin", the admin username and the admin email are one account and must share a
// single failure counter.
func TestLogin_AdminAliasesShareLockout(t *testing.T) {
	store := setupTestStore(t)
	cfg, _ := store.GetAuthConfig(nil)
	cfg.AdminEmail = schemas.NewSecretVar("owner@example.com")
	_ = store.UpdateAuthConfig(nil, cfg)
	handler := NewSessionHandler(store, nil)

	aliases := []string{"admin", "owner@example.com", "ADMIN"}
	for i := 0; i < loginMaxFailedAttempts; i++ {
		loginStatus(handler, aliases[i%len(aliases)], "WrongPassword1!", fmt.Sprintf("10.30.0.%d", i+1))
	}
	if code := loginStatus(handler, "owner@example.com", "AdminPass123!", "10.30.0.50"); code != http.StatusTooManyRequests {
		t.Fatalf("expected admin to be locked after %d failures across aliases, got %d", loginMaxFailedAttempts, code)
	}
}

func TestRegister_RejectsAdminIdentifiers(t *testing.T) {
	store := setupTestStore(t)
	cfg, _ := store.GetAuthConfig(nil)
	cfg.AdminEmail = schemas.NewSecretVar("owner@example.com")
	_ = store.UpdateAuthConfig(nil, cfg)
	handler := NewSessionHandler(store, nil)

	cases := []map[string]string{
		{"username": "owner@example.com", "email": "someone@example.com", "password": "StrongPassword123!"},
		{"username": "mallory", "email": "OWNER@example.com", "password": "StrongPassword123!"},
	}
	for i, body := range cases {
		ctx := makeFastHTTPCtx("POST", "/api/session/register", body, fmt.Sprintf("10.40.0.%d", i+1))
		handler.register(ctx)
		if ctx.Response.StatusCode() != http.StatusConflict {
			t.Errorf("case %d: expected 409 for admin identifier, got %d %s", i, ctx.Response.StatusCode(), ctx.Response.Body())
		}
	}
}

// A sign-up still waiting for its email code must not be overwritten by another
// registration of the same username; once the code window has passed it can be reclaimed.
func TestRegister_UnverifiedSignupIsReservedWhileCodeIsValid(t *testing.T) {
	store := setupTestStore(t)
	enableTestSMTP(store)
	captureAuthMail(t)
	handler := NewSessionHandler(store, nil)

	register := func(email, password, ip string) int {
		ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
			"username": "frank", "email": email, "password": password,
		}, ip)
		handler.register(ctx)
		return ctx.Response.StatusCode()
	}

	if code := register("frank@example.com", "OwnerPassword123!", "10.60.0.1"); code != http.StatusOK {
		t.Fatalf("first sign-up: expected 200, got %d", code)
	}
	for i, email := range []string{"attacker@example.com", "frank@example.com"} {
		if code := register(email, "AttackerPassword123!", fmt.Sprintf("10.60.1.%d", i+1)); code != http.StatusConflict {
			t.Fatalf("overwrite with %s while code is valid: expected 409, got %d", email, code)
		}
	}
	user, _ := store.GetUserByUsername(nil, "frank")
	if ok, _ := encrypt.CompareHash(user.Password, "OwnerPassword123!"); !ok || user.Email != "frank@example.com" {
		t.Fatalf("pending sign-up was modified: email=%s", user.Email)
	}

	user.UpdatedAt = time.Now().Add(-passwordResetOTPTTL - time.Minute)
	_ = store.UpdateUser(nil, user)
	if code := register("newowner@example.com", "NewOwnerPassword123!", "10.60.2.1"); code != http.StatusOK {
		t.Fatalf("abandoned sign-up should be reclaimable after the code window, got %d", code)
	}
}

func TestClientIPAddress_IgnoresSpoofedForwardedFor(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "1")
	newCtx := func(xff string) *fasthttp.RequestCtx {
		ctx := &fasthttp.RequestCtx{}
		ctx.Init(&fasthttp.Request{}, &net.TCPAddr{IP: net.ParseIP("172.18.0.2"), Port: 443}, nil)
		ctx.Request.Header.Set("X-Forwarded-For", xff)
		return ctx
	}
	cases := map[string]string{
		"203.0.113.9":                         "203.0.113.9",
		"127.0.0.1, 203.0.113.9":              "203.0.113.9",
		"1.2.3.4, 203.0.113.9":                "203.0.113.9",
		"127.0.0.1, 10.0.0.5":                 "10.0.0.5",
		"198.51.100.7, 203.0.113.9, 10.0.0.1": "203.0.113.9",
	}
	for xff, want := range cases {
		if got := clientIPAddress(newCtx(xff)); got != want {
			t.Errorf("XFF %q: got %q, want %q", xff, got, want)
		}
	}
}

// The bootstrap admin session must survive the session check when no explicit admin
// username is configured (login names that session "admin").
func TestCheckSession_BootstrapAdminWithoutUsername(t *testing.T) {
	store := setupTestStore(t)
	pw, _ := encrypt.Hash("AdminPass123!")
	_ = store.UpdateAuthConfig(nil, &configstore.AuthConfig{IsEnabled: true, AdminPassword: schemas.NewSecretVar(pw)})
	_ = store.CreateSession(nil, &tables.SessionsTable{
		Token: "boot-token", Username: "admin", Role: "admin",
		ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	ctx := makeFastHTTPCtx("GET", "/api/config", nil, "10.50.0.1")
	if got := checkSession(ctx, store, "boot-token"); got != sessionOK {
		t.Fatalf("expected bootstrap admin session to be valid, got %v", got)
	}
}
