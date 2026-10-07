package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"
	"time"

	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

const registrationReviewTokenTTL = 48 * time.Hour

type registrationReviewClaims struct {
	UserID string `json:"uid"`
	Action string `json:"act"` // "approve" | "reject"
	Exp    int64  `json:"exp"`
}

func issueRegistrationReviewToken(userID, action string) (string, error) {
	key, err := getResetTokenSecretKey()
	if err != nil {
		return "", err
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "reject" {
		return "", fmt.Errorf("invalid review action")
	}
	claims := registrationReviewClaims{
		UserID: strings.TrimSpace(userID),
		Action: action,
		Exp:    time.Now().Add(registrationReviewTokenTTL).Unix(),
	}
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("reg_review_v1|" + payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sigB64, nil
}

func verifyRegistrationReviewToken(tokenStr string) (*registrationReviewClaims, error) {
	key, err := getResetTokenSecretKey()
	if err != nil {
		return nil, err
	}
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed review token")
	}
	payloadB64, sigB64 := parts[0], parts[1]
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("reg_review_v1|" + payloadB64))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || !hmac.Equal(expected, actual) {
		return nil, fmt.Errorf("invalid review token")
	}
	data, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid review token")
	}
	var claims registrationReviewClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("invalid review token")
	}
	if strings.TrimSpace(claims.UserID) == "" || (claims.Action != "approve" && claims.Action != "reject") {
		return nil, fmt.Errorf("invalid review token")
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("review link has expired")
	}
	return &claims, nil
}

func publicRequestOrigin(ctx *fasthttp.RequestCtx) string {
	proto := strings.TrimSpace(string(ctx.Request.Header.Peek("X-Forwarded-Proto")))
	if proto == "" {
		if ctx.IsTLS() {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(string(ctx.Request.Header.Peek("X-Forwarded-Host")))
	if host == "" {
		host = strings.TrimSpace(string(ctx.Host()))
	}
	if host == "" {
		return ""
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func publicUIOrigin(ctx *fasthttp.RequestCtx) string {
	for _, key := range []string{"GATEWAY_UI_URL", "GATEWAY_UI_URL", "GATEWAY_PUBLIC_URL", "GATEWAY_PUBLIC_URL"} {
		if u := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); u != "" {
			return u
		}
	}
	return publicRequestOrigin(ctx)
}

func publicAPIOrigin(ctx *fasthttp.RequestCtx) string {
	if u := strings.TrimRight(strings.TrimSpace(gatewayEnv("API_URL")), "/"); u != "" {
		return u
	}
	return publicRequestOrigin(ctx)
}

// notifyAdminsPendingRegistration emails admins/sub-admins Accept/Deny links and a Governance deep link.
func (h *SessionHandler) notifyAdminsPendingRegistration(ctx *fasthttp.RequestCtx, user *tables.TableUser) {
	if h == nil || h.configStore == nil || user == nil || user.Status != tables.UserStatusPending {
		return
	}
	if !smtpEnabled(h.configStore, ctx) {
		return
	}
	approveTok, err1 := issueRegistrationReviewToken(user.ID, "approve")
	rejectTok, err2 := issueRegistrationReviewToken(user.ID, "reject")
	if err1 != nil || err2 != nil {
		logger.Warn("registration review tokens unavailable user=%s: approve=%v reject=%v", user.Username, err1, err2)
		return
	}
	apiOrigin := publicAPIOrigin(ctx)
	uiOrigin := publicUIOrigin(ctx)
	if apiOrigin == "" {
		logger.Warn("registration admin notify skipped: no public API origin (set GATEWAY_API_URL)")
		return
	}
	acceptURL := apiOrigin + "/api/session/users/review?token=" + approveTok
	denyURL := apiOrigin + "/api/session/users/review?token=" + rejectTok
	governanceURL := "/workspace/governance/users"
	if uiOrigin != "" {
		governanceURL = uiOrigin + governanceURL
	}

	users, err := h.configStore.GetUsers(ctx)
	if err != nil {
		return
	}
	body := fmt.Sprintf(
		"A new Gateway sign-up is waiting for approval.\n\n"+
			"Username: %s\nEmail: %s\n\n"+
			"Accept (activate account):\n%s\n\n"+
			"Deny (block access):\n%s\n\n"+
			"Or open Governance → Users:\n%s\n\n"+
			"These links expire in %d hours.\n",
		user.Username, user.Email, acceptURL, denyURL, governanceURL, int(registrationReviewTokenTTL.Hours()),
	)
	subject := "Gateway registration pending: " + user.Username
	seen := map[string]bool{}
	for _, u := range users {
		if u == nil || !u.IsApproved() {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(u.Role))
		if role != "admin" && role != "sub_admin" {
			continue
		}
		email := strings.TrimSpace(strings.ToLower(u.Email))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		if err := sendAuthEmail(h.configStore, ctx, email, subject, body); err != nil {
			logger.Warn("pending registration admin notify failed to=%s: %v", email, err)
		}
	}
	// Built-in admin may not have a governance_users row — include configured admin email.
	if adminName, adminEmail := h.getAdminCredentials(ctx); adminEmail != "" && !seen[strings.ToLower(adminEmail)] {
		_ = adminName
		if err := sendAuthEmail(h.configStore, ctx, adminEmail, subject, body); err != nil {
			logger.Warn("pending registration admin notify failed to built-in admin: %v", err)
		}
	}
}

// reviewRegistrationByToken handles GET /api/session/users/review?token=...
// Signed Accept/Deny links from the admin notification email (no session cookie required).
func (h *SessionHandler) reviewRegistrationByToken(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		h.writeReviewHTML(ctx, fasthttp.StatusServiceUnavailable, "Unavailable", "User registration is not available.")
		return
	}
	token := strings.TrimSpace(string(ctx.QueryArgs().Peek("token")))
	if token == "" {
		h.writeReviewHTML(ctx, fasthttp.StatusBadRequest, "Invalid link", "This review link is missing a token.")
		return
	}
	claims, err := verifyRegistrationReviewToken(token)
	if err != nil {
		h.writeReviewHTML(ctx, fasthttp.StatusUnauthorized, "Link expired or invalid", "Request a fresh notification from Governance → Users, or open the Users page while signed in as an admin.")
		return
	}
	user, err := h.configStore.GetUserByID(ctx, claims.UserID)
	if err != nil || user == nil {
		h.writeReviewHTML(ctx, fasthttp.StatusNotFound, "User not found", "This registration is no longer available.")
		return
	}

	switch claims.Action {
	case "approve":
		if user.Status == tables.UserStatusApproved {
			h.writeReviewHTML(ctx, fasthttp.StatusOK, "Already approved", html.EscapeString(user.Username)+" can already sign in.")
			return
		}
		if user.Status == tables.UserStatusEmailUnverified {
			h.writeReviewHTML(ctx, fasthttp.StatusConflict, "Email not verified", "The applicant must enter the email verification code before you can accept this account. You can Deny the request from Governance → Users.")
			return
		}
		if user.Status != tables.UserStatusPending {
			h.writeReviewHTML(ctx, fasthttp.StatusConflict, "Cannot approve", "Only pending sign-ups can be approved from this link.")
			return
		}
		now := time.Now()
		user.Status = tables.UserStatusApproved
		user.ReviewedAt = &now
		user.UpdatedAt = now
		if err := h.configStore.UpdateUser(ctx, user); err != nil {
			h.writeReviewHTML(ctx, fasthttp.StatusInternalServerError, "Failed", "Could not approve this registration. Try again from Governance → Users.")
			return
		}
		if h.promptLifecycle != nil {
			_ = h.promptLifecycle.OnUserCreated(ctx, user, true)
		}
		_, _ = trySendRegistrationDecisionEmail(h.configStore, ctx, user.Username, user.Email, "approved")
		h.writeReviewHTML(ctx, fasthttp.StatusOK, "Accepted", html.EscapeString(user.Username)+" is now active and can sign in. They also appear under Governance → Users.")
	case "reject":
		if user.Status == tables.UserStatusRejected {
			h.writeReviewHTML(ctx, fasthttp.StatusOK, "Already denied", html.EscapeString(user.Username)+" was already denied.")
			return
		}
		if user.Status != tables.UserStatusPending && user.Status != tables.UserStatusEmailUnverified {
			h.writeReviewHTML(ctx, fasthttp.StatusConflict, "Cannot deny", "Only pending sign-ups can be denied from this link.")
			return
		}
		now := time.Now()
		user.Status = tables.UserStatusRejected
		user.ReviewedAt = &now
		user.UpdatedAt = now
		if err := h.configStore.UpdateUser(ctx, user); err != nil {
			h.writeReviewHTML(ctx, fasthttp.StatusInternalServerError, "Failed", "Could not deny this registration. Try again from Governance → Users.")
			return
		}
		_ = h.configStore.DeleteSessionsByUsername(ctx, user.Username)
		_, _ = trySendRegistrationDecisionEmail(h.configStore, ctx, user.Username, user.Email, "rejected")
		h.writeReviewHTML(ctx, fasthttp.StatusOK, "Denied", html.EscapeString(user.Username)+" cannot sign in. The request is removed from the Governance pending list.")
	default:
		h.writeReviewHTML(ctx, fasthttp.StatusBadRequest, "Invalid link", "Unknown review action.")
	}
}

func (h *SessionHandler) writeReviewHTML(ctx *fasthttp.RequestCtx, status int, title, message string) {
	ui := publicUIOrigin(ctx)
	usersLink := "/workspace/governance/users"
	if ui != "" {
		usersLink = ui + usersLink
	}
	ctx.SetStatusCode(status)
	ctx.SetContentType("text/html; charset=utf-8")
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>%s · Gateway</title>
<style>
body{font-family:system-ui,sans-serif;background:#0b1220;color:#e2e8f0;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;padding:24px}
.card{max-width:480px;background:#111827;border:1px solid #334155;border-radius:16px;padding:28px;box-shadow:0 20px 50px #0008}
h1{font-size:1.25rem;margin:0 0 12px;color:#5eead4}
p{margin:0 0 20px;line-height:1.5;color:#cbd5e1}
a{color:#2dd4bf}
</style></head><body><div class="card"><h1>%s</h1><p>%s</p>
<p><a href="%s">Open Governance → Users</a></p></div></body></html>`,
		html.EscapeString(title), html.EscapeString(title), message, html.EscapeString(usersLink))
	ctx.SetBodyString(page)
}
