package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/unifai/unifai/framework/encrypt"
	"github.com/unifai/unifai/framework/mailer"
)

var sixDigits = regexp.MustCompile(`\b\d{6}\b`)

func captureAuthMail(t *testing.T) *[]mailer.Message {
	t.Helper()
	sent := &[]mailer.Message{}
	prev := authMailSend
	authMailSend = func(_ mailer.Config, msg mailer.Message) error {
		*sent = append(*sent, msg)
		return nil
	}
	t.Cleanup(func() { authMailSend = prev })
	return sent
}

func enableTestSMTP(store *memoryConfigStore) {
	store.mu.Lock()
	store.smtpConfig = &tables.TableSMTPConfig{Enabled: true, Host: "smtp.test", Port: 587, FromEmail: "noreply@test.local"}
	store.mu.Unlock()
}

func TestSignupEmailVerificationFlow(t *testing.T) {
	store := setupTestStore(t)
	enableTestSMTP(store)
	sent := captureAuthMail(t)
	handler := NewSessionHandler(store, nil)
	const ip = "192.168.7.10"

	ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
		"username": "carol", "email": "carol@example.com", "password": "StrongPassword123!",
	}, ip)
	handler.register(ctx)
	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("register: %d %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	var resp map[string]any
	_ = json.Unmarshal(ctx.Response.Body(), &resp)
	if resp["verification_required"] != true || resp["status"] != tables.UserStatusEmailUnverified {
		t.Fatalf("expected email verification step, got %v", resp)
	}
	if len(*sent) != 1 || (*sent)[0].To != "carol@example.com" {
		t.Fatalf("expected one code email to carol, got %+v", *sent)
	}
	code := sixDigits.FindString((*sent)[0].Body)
	if code == "" {
		t.Fatal("verification email has no 6-digit code")
	}

	login := makeFastHTTPCtx("POST", "/api/session/login", map[string]string{"username": "carol", "password": "StrongPassword123!"}, "192.168.7.11")
	handler.login(login)
	if login.Response.StatusCode() != http.StatusForbidden || !strings.Contains(string(login.Response.Body()), "Verify your email") {
		t.Fatalf("unverified login must be blocked with a verify hint, got %d %s", login.Response.StatusCode(), login.Response.Body())
	}

	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	bad := makeFastHTTPCtx("POST", "/api/session/register/verify", map[string]string{"username": "carol", "otp": wrong}, ip)
	handler.verifyRegistration(bad)
	if bad.Response.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("wrong code must be rejected, got %d", bad.Response.StatusCode())
	}

	good := makeFastHTTPCtx("POST", "/api/session/register/verify", map[string]string{"username": "carol", "otp": code}, ip)
	handler.verifyRegistration(good)
	if good.Response.StatusCode() != http.StatusOK {
		t.Fatalf("correct code right after a typo must work, got %d %s", good.Response.StatusCode(), good.Response.Body())
	}
	if u, _ := store.GetUserByUsername(nil, "carol"); u == nil || u.Status != tables.UserStatusPending {
		t.Fatalf("verified user must wait for admin approval, got %+v", u)
	}

	replay := makeFastHTTPCtx("POST", "/api/session/register/verify", map[string]string{"username": "carol", "otp": code}, ip)
	handler.verifyRegistration(replay)
	if replay.Response.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("code must not be reusable, got %d", replay.Response.StatusCode())
	}

	resend := makeFastHTTPCtx("POST", "/api/session/register/resend", map[string]string{"username": "nobody-here"}, ip)
	handler.resendRegistrationCode(resend)
	if resend.Response.StatusCode() != http.StatusOK || !strings.Contains(string(resend.Response.Body()), registrationVerifyGeneric) {
		t.Fatalf("resend must answer generically, got %d %s", resend.Response.StatusCode(), resend.Response.Body())
	}
}

func TestSignupWithoutSMTPGoesStraightToApproval(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	ctx := makeFastHTTPCtx("POST", "/api/session/register", map[string]string{
		"username": "dave", "email": "dave@example.com", "password": "StrongPassword123!",
	}, "192.168.7.20")
	handler.register(ctx)
	var resp map[string]any
	_ = json.Unmarshal(ctx.Response.Body(), &resp)
	if ctx.Response.StatusCode() != http.StatusOK || resp["status"] != tables.UserStatusPending || resp["verification_required"] != nil {
		t.Fatalf("without SMTP sign-up must go to the admin queue, got %d %v", ctx.Response.StatusCode(), resp)
	}
}

func TestForgotPasswordOTPAcceptsCorrectCodeAfterTypo(t *testing.T) {
	store := setupTestStore(t)
	enableTestSMTP(store)
	sent := captureAuthMail(t)
	handler := NewSessionHandler(store, nil)
	hash, _ := encrypt.Hash("InitialPassword123!")
	now := time.Now()
	_ = store.CreateUser(nil, &tables.TableUser{ID: "u-erin", Username: "erin", Email: "erin@example.com", Password: hash,
		Role: "user", Status: tables.UserStatusApproved, CreatedAt: now, UpdatedAt: now})

	const ip = "192.168.7.30"
	fp := makeFastHTTPCtx("POST", "/api/session/forgot-password", map[string]string{"email": "erin@example.com"}, ip)
	handler.forgotPassword(fp)
	if len(*sent) != 1 {
		t.Fatalf("expected reset code email, got %d", len(*sent))
	}
	code := sixDigits.FindString((*sent)[0].Body)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	bad := makeFastHTTPCtx("POST", "/api/session/verify-otp", map[string]string{"email": "erin@example.com", "otp": wrong}, ip)
	handler.verifyOTP(bad)
	good := makeFastHTTPCtx("POST", "/api/session/verify-otp", map[string]string{"email": "erin@example.com", "otp": code}, ip)
	handler.verifyOTP(good)
	if good.Response.StatusCode() != http.StatusOK {
		t.Fatalf("correct OTP after one typo (same IP) must be accepted, got %d %s", good.Response.StatusCode(), good.Response.Body())
	}
}

func TestAdminCannotRenameUserToAdminOrTakenName(t *testing.T) {
	store := setupTestStore(t)
	handler := NewSessionHandler(store, nil)
	hash, _ := encrypt.Hash("InitialPassword123!")
	now := time.Now()
	for _, u := range []*tables.TableUser{
		{ID: "u-frank", Username: "frank", Email: "frank@example.com"},
		{ID: "u-gina", Username: "gina", Email: "gina@example.com"},
	} {
		u.Password, u.Role, u.Status, u.CreatedAt, u.UpdatedAt = hash, "user", tables.UserStatusApproved, now, now
		_ = store.CreateUser(nil, u)
	}
	token := getAdminSessionToken(t, handler)
	for name, want := range map[string]int{"admin": http.StatusBadRequest, "gina": http.StatusConflict, "frank2": http.StatusOK} {
		ctx := makeFastHTTPCtx("PUT", "/api/session/users/u-frank", map[string]any{"username": name}, "192.168.1.1")
		ctx.Request.Header.Set("Authorization", "Bearer "+token)
		ctx.SetUserValue("id", "u-frank")
		handler.updateUser(ctx)
		if ctx.Response.StatusCode() != want {
			t.Errorf("rename to %q: got %d want %d (%s)", name, ctx.Response.StatusCode(), want, ctx.Response.Body())
		}
	}
}
