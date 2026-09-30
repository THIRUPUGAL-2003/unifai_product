package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("PASSWORD_RESET_SECRET", "test-password-reset-secret-min-32-chars!!")
	os.Exit(m.Run())
}

func TestPasswordResetTokenLifecycle(t *testing.T) {
	otpID := uint(42)
	username := "test_operator"
	email := "operator@yespanchi.com"

	// 1. Issue token
	token, err := issuePasswordResetToken(otpID, username, email)
	if err != nil {
		t.Fatalf("Failed to issue reset token: %v", err)
	}
	if len(token) < 20 {
		t.Fatalf("Token is suspiciously short: %s", token)
	}

	// 2. Verify valid token
	claims, err := verifyPasswordResetToken(token)
	if err != nil {
		t.Fatalf("Failed to verify valid token: %v", err)
	}
	if claims.OTPID != otpID {
		t.Errorf("Expected OTP ID %d, got %d", otpID, claims.OTPID)
	}
	if claims.Username != username {
		t.Errorf("Expected Username %s, got %s", username, claims.Username)
	}
	if claims.Email != email {
		t.Errorf("Expected Email %s, got %s", email, claims.Email)
	}

	// 3. Reject tampered token
	tampered := token[:len(token)-4] + "xxxx"
	if _, err := verifyPasswordResetToken(tampered); err == nil {
		t.Errorf("Expected signature failure on tampered token, but got nil error")
	}

	// 4. Reject malformed token
	if _, err := verifyPasswordResetToken("invalid-token-no-dots"); err == nil {
		t.Errorf("Expected error for malformed token, got nil")
	}
}

func TestForgotPasswordRateLimiting(t *testing.T) {
	if forgotPasswordCooldown != 2*time.Minute {
		t.Errorf("Expected forgotPasswordCooldown of 2 minutes, got %v", forgotPasswordCooldown)
	}
	if maxForgotPasswordPerHourTarget < 1 || maxForgotPasswordPerHourIP < maxForgotPasswordPerHourTarget {
		t.Errorf("Unexpected forgot-password rate caps: target=%d ip=%d", maxForgotPasswordPerHourTarget, maxForgotPasswordPerHourIP)
	}
}

func TestIsValidEmail(t *testing.T) {
	valid := []string{
		"user@example.com",
		"admin.test@sub.domain.co",
		"first+last@gmail.com",
		"  spaces-trimmed@example.org  ",
	}
	for _, email := range valid {
		if !isValidEmail(email) {
			t.Errorf("Expected %q to be valid email, got false", email)
		}
	}

	invalid := []string{
		"",
		"   ",
		"notanemail",
		"missingdomain@",
		"@missingusername.com",
		"user@localhost",
		"user@.com",
		"user@com.",
		"spaces in@domain.com",
	}
	for _, email := range invalid {
		if isValidEmail(email) {
			t.Errorf("Expected %q to be invalid email, got true", email)
		}
	}
}

func TestPasswordPolicyRules(t *testing.T) {
	valid := "StrongPass123!"
	if fails := getPasswordPolicyFailures(valid); len(fails) > 0 {
		t.Errorf("Expected password %q to be valid, got failures: %v", valid, fails)
	}

	tooShort := "Ab1!"
	if fails := getPasswordPolicyFailures(tooShort); len(fails) == 0 {
		t.Errorf("Expected password %q to fail length check, got 0 failures", tooShort)
	}

	noUpper := "strongpass123!"
	if fails := getPasswordPolicyFailures(noUpper); len(fails) == 0 {
		t.Errorf("Expected password %q to fail uppercase check, got 0 failures", noUpper)
	}

	noLower := "STRONGPASS123!"
	if fails := getPasswordPolicyFailures(noLower); len(fails) == 0 {
		t.Errorf("Expected password %q to fail lowercase check, got 0 failures", noLower)
	}

	noDigit := "StrongPassword!"
	if fails := getPasswordPolicyFailures(noDigit); len(fails) == 0 {
		t.Errorf("Expected password %q to fail digit check, got 0 failures", noDigit)
	}

	noSpecial := "StrongPass1234"
	if fails := getPasswordPolicyFailures(noSpecial); len(fails) == 0 {
		t.Errorf("Expected password %q to fail special char check, got 0 failures", noSpecial)
	}
}

func TestPasswordResetTokenExpiry(t *testing.T) {
	// Directly verify that expired claims are rejected
	key, err := getResetTokenSecretKey()
	if err != nil {
		t.Fatalf("Failed to get secret key: %v", err)
	}
	claims := passwordResetTokenClaims{
		OTPID:    99,
		Username: "expired_user",
		Email:    "expired@example.com",
		Exp:      time.Now().Add(-1 * time.Minute).Unix(), // expired 1 minute ago
	}
	data, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	expiredToken := payloadB64 + "." + sigB64

	if _, err := verifyPasswordResetToken(expiredToken); err == nil {
		t.Errorf("Expected error for expired reset token, got nil")
	}
}


