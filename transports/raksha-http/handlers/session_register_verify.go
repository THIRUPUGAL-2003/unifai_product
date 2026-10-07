package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/raksha/raksha/framework/configstore/tables"
	"github.com/raksha/raksha/framework/encrypt"
	"github.com/valyala/fasthttp"
)

const registrationVerifyGeneric = "If a sign-up is waiting for verification, a new code was sent by email"

// respondRegistered finishes register(): either emails a sign-up code or reports the admin queue.
// discardOnSendFailure removes the just-written sign-up when the code email fails, so the
// username and email are not held by a sign-up nobody can verify.
func (h *SessionHandler) respondRegistered(ctx *fasthttp.RequestCtx, user *tables.TableUser, verifyEmail, discardOnSendFailure bool) {
	if !verifyEmail {
		h.notifyAdminsPendingRegistration(ctx, user)
		SendJSON(ctx, map[string]any{
			"message": "Sent to the admin waiting for approval",
			"id":      user.ID,
			"status":  user.Status,
			"role":    user.Role,
		})
		return
	}
	if err := h.sendRegistrationCode(ctx, user); err != nil {
		logger.Warn("sign-up verification email failed username=%s: %v", user.Username, err)
		if discardOnSendFailure {
			_ = h.configStore.DeleteUser(ctx, user.ID)
		}
		SendError(ctx, fasthttp.StatusServiceUnavailable, "Could not send the verification email. Please try again in a few minutes.")
		return
	}
	SendJSON(ctx, map[string]any{
		"message":               fmt.Sprintf("We emailed a 6-digit code to %s. Enter it to finish sign-up.", maskEmail(user.Email)),
		"id":                    user.ID,
		"status":                user.Status,
		"role":                  user.Role,
		"verification_required": true,
		"expires_in_minutes":    int(passwordResetOTPTTL.Minutes()),
	})
}

// sendRegistrationCode stores a hashed one-time code (same table/limits as password reset)
// and emails the plaintext code. Unverified accounts cannot use password reset, so the rows never mix.
func (h *SessionHandler) sendRegistrationCode(ctx *fasthttp.RequestCtx, user *tables.TableUser) error {
	otp, err := generateOTP6()
	if err != nil {
		return err
	}
	hash, err := encrypt.Hash(otp)
	if err != nil {
		return err
	}
	if err := h.configStore.CreatePasswordResetOTP(ctx, &tables.TablePasswordResetOTP{
		Username:  user.Username,
		Email:     user.Email,
		OTPHash:   hash,
		ExpiresAt: time.Now().Add(passwordResetOTPTTL),
		CreatedAt: time.Now(),
	}); err != nil {
		return err
	}
	body := fmt.Sprintf(
		"Hello %s,\n\nYour Raksha sign-up verification code is: %s\n\nIt expires in %d minutes. After you verify, an administrator must approve your account.\nIf you did not sign up, ignore this email.\n",
		user.Username, otp, int(passwordResetOTPTTL.Minutes()),
	)
	return sendAuthEmail(h.configStore, ctx, user.Email, "Raksha sign-up verification code", body)
}

// verifyRegistration handles POST /api/session/register/verify — email code → admin approval queue.
func (h *SessionHandler) verifyRegistration(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "User registration is not available")
		return
	}
	if consumeForgotPasswordQuota(h.configStore, ctx, "register-verify:"+clientIPAddress(ctx), "") {
		SendError(ctx, fasthttp.StatusTooManyRequests, "Too many verification attempts. Please try again later.")
		return
	}
	var payload struct {
		Username string `json:"username"`
		OTP      string `json:"otp"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	payload.OTP = strings.TrimSpace(payload.OTP)
	if payload.Username == "" || payload.OTP == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Username and verification code are required")
		return
	}
	const invalid = "Invalid or expired verification code"

	user, err := h.configStore.GetUserByUsername(ctx, payload.Username)
	if err != nil || user == nil || user.Status != tables.UserStatusEmailUnverified {
		padPasswordCompare(payload.OTP)
		SendError(ctx, fasthttp.StatusUnauthorized, invalid)
		return
	}
	row, err := h.configStore.GetLatestPasswordResetOTP(ctx, user.Username)
	if err != nil || row == nil || row.Used || time.Now().After(row.ExpiresAt) || row.FailedAttempts >= maxOTPAttempts {
		padPasswordCompare(payload.OTP)
		SendError(ctx, fasthttp.StatusUnauthorized, invalid)
		return
	}
	if ok, cmpErr := encrypt.CompareHash(row.OTPHash, payload.OTP); cmpErr != nil || !ok {
		if fails, _ := h.configStore.IncrementPasswordResetOTPFailures(ctx, row.ID); fails >= maxOTPAttempts {
			_ = h.configStore.MarkPasswordResetOTPUsed(ctx, row.ID)
		}
		SendError(ctx, fasthttp.StatusUnauthorized, invalid)
		return
	}
	_ = h.configStore.MarkPasswordResetOTPUsed(ctx, row.ID)

	user.Status = tables.UserStatusPending
	user.UpdatedAt = time.Now()
	if err := h.configStore.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to submit registration")
		return
	}
	h.notifyAdminsPendingRegistration(ctx, user)
	SendJSON(ctx, map[string]any{
		"message": "Email verified. Sent to the admin waiting for approval",
		"id":      user.ID,
		"status":  user.Status,
	})
}

// resendRegistrationCode handles POST /api/session/register/resend. Always generic (no enumeration).
func (h *SessionHandler) resendRegistrationCode(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "User registration is not available")
		return
	}
	var payload struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	username := strings.TrimSpace(payload.Username)
	generic := map[string]any{"message": registrationVerifyGeneric}
	if username == "" || consumeForgotPasswordQuota(h.configStore, ctx, clientIPAddress(ctx), "register-resend:"+username) {
		SendJSON(ctx, generic)
		return
	}
	user, err := h.configStore.GetUserByUsername(ctx, username)
	if err != nil || user == nil || user.Status != tables.UserStatusEmailUnverified || !smtpEnabled(h.configStore, ctx) {
		SendJSON(ctx, generic)
		return
	}
	if err := h.sendRegistrationCode(ctx, user); err != nil {
		logger.Warn("sign-up verification resend failed username=%s: %v", user.Username, err)
	} else {
		// The username hold follows the newest code's lifetime.
		user.UpdatedAt = time.Now()
		_ = h.configStore.UpdateUser(ctx, user)
	}
	SendJSON(ctx, generic)
}

func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "your email"
	}
	local := email[:at]
	if len(local) <= 2 {
		return local[:1] + "***" + email[at:]
	}
	return local[:2] + "***" + email[at:]
}
