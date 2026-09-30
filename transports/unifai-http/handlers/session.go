package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/unifai/unifai/framework/encrypt"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// SessionHandler manages HTTP requests for session operations
type SessionHandler struct {
	configStore    configstore.ConfigStore
	wsTicketStore  *WSTicketStore
	userGovernance UserGovernanceSyncer
}

// NewSessionHandler creates a new session handler instance.
// Optional userGovernance syncs Users.Budget into the live governance meter.
func NewSessionHandler(configStore configstore.ConfigStore, wsTicketStore *WSTicketStore, userGovernance ...UserGovernanceSyncer) *SessionHandler {
	h := &SessionHandler{
		configStore:   configStore,
		wsTicketStore: wsTicketStore,
	}
	if len(userGovernance) > 0 {
		h.userGovernance = userGovernance[0]
	}
	// Validate PASSWORD_RESET_SECRET at startup so operators see a clear warning
	// instead of users hitting a cryptic 500 when they try to reset their password.
	if secret := strings.TrimSpace(os.Getenv("PASSWORD_RESET_SECRET")); len(secret) < 32 {
		logger.Warn("PASSWORD_RESET_SECRET is missing or shorter than 32 characters — password reset via OTP will return 500 errors. Set this env var to enable the feature.")
	}
	return h
}

var (
	resetTokenSecretKey []byte
	resetTokenOnce      sync.Once
	resetTokenInitErr   error
)

const (
	maxOTPAttempts                 = 5
	forgotPasswordCooldown         = 2 * time.Minute
	maxForgotPasswordPerHourTarget = 5
	maxForgotPasswordPerHourIP     = 10
	maxRegistrationsPerHour        = 10
)

type passwordResetTokenClaims struct {
	OTPID    uint   `json:"oid"`
	Username string `json:"usr"`
	Email    string `json:"eml"`
	Exp      int64  `json:"exp"`
}

func getResetTokenSecretKey() ([]byte, error) {
	resetTokenOnce.Do(func() {
		secret := strings.TrimSpace(os.Getenv("PASSWORD_RESET_SECRET"))
		if len(secret) < 32 {
			if guardSecret := strings.TrimSpace(os.Getenv("UNIFAI_GUARD_SECRET")); len(guardSecret) >= 16 {
				secret = guardSecret + "-password-reset-token-fallback-key-32b"
			} else if encKey := strings.TrimSpace(os.Getenv("UNIFAI_ENCRYPTION_KEY")); len(encKey) >= 16 {
				secret = encKey + "-password-reset-token-fallback-key-32b"
			}
		}
		if len(secret) < 32 {
			resetTokenInitErr = errors.New("PASSWORD_RESET_SECRET must be set to at least 32 characters")
			return
		}
		h := sha256.Sum256([]byte(secret + "_pwd_reset_salt_v1"))
		resetTokenSecretKey = h[:]
	})
	if resetTokenInitErr != nil {
		return nil, resetTokenInitErr
	}
	if len(resetTokenSecretKey) == 0 {
		return nil, errors.New("PASSWORD_RESET_SECRET is not configured")
	}
	return resetTokenSecretKey, nil
}

func issuePasswordResetToken(otpID uint, username, email string) (string, error) {
	key, err := getResetTokenSecretKey()
	if err != nil {
		return "", err
	}
	claims := passwordResetTokenClaims{
		OTPID:    otpID,
		Username: username,
		Email:    email,
		Exp:      time.Now().Add(10 * time.Minute).Unix(),
	}
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sigB64, nil
}

func verifyPasswordResetToken(tokenStr string) (*passwordResetTokenClaims, error) {
	key, err := getResetTokenSecretKey()
	if err != nil {
		return nil, err
	}
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, errors.New("malformed reset token")
	}
	payloadB64, sigB64 := parts[0], parts[1]
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payloadB64))
	expectedSig := mac.Sum(nil)
	actualSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("invalid token signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, errors.New("invalid token payload")
	}
	var claims passwordResetTokenClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, errors.New("invalid claims format")
	}
	if time.Now().Unix() > claims.Exp {
		return nil, errors.New("reset token has expired")
	}
	return &claims, nil
}

// normalizeUserRole accepts admin/user or a custom RBAC role that exists in the workspace store.
// Returns ("", false) when the role is invalid so callers can 400 instead of silently coercing to "user".
func (h *SessionHandler) normalizeUserRole(ctx *fasthttp.RequestCtx, role string) (string, bool) {
	role = strings.TrimSpace(role)
	if role == "" {
		return "user", true
	}
	if role == "admin" || role == "user" {
		return role, true
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		return "", false
	}
	_ = ws.EnsureRBACRoles(ctx)
	rows, err := ws.ListRBACRoles(ctx)
	if err != nil {
		return "", false
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Name), role) {
			return row.Name, true
		}
	}
	return "", false
}

// RegisterRoutes registers the session-related routes
func (h *SessionHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.UnifAIHTTPMiddleware) {
	r.POST("/api/session/login", lib.ChainMiddlewares(h.login, middlewares...))
	r.POST("/api/session/logout", lib.ChainMiddlewares(h.logout, middlewares...))
	r.GET("/api/session/is-auth-enabled", lib.ChainMiddlewares(h.isAuthEnabled, middlewares...))
	r.POST("/api/session/ws-ticket", lib.ChainMiddlewares(h.issueWSTicket, middlewares...))
	r.POST("/api/session/register", lib.ChainMiddlewares(h.register, middlewares...))
	r.POST("/api/session/register/verify", lib.ChainMiddlewares(h.verifyRegistration, middlewares...))
	r.POST("/api/session/register/resend", lib.ChainMiddlewares(h.resendRegistrationCode, middlewares...))
	r.POST("/api/session/forgot-password", lib.ChainMiddlewares(h.forgotPassword, middlewares...))
	r.POST("/api/session/verify-otp", lib.ChainMiddlewares(h.verifyOTP, middlewares...))
	r.POST("/api/session/reset-password", lib.ChainMiddlewares(h.resetPassword, middlewares...))
	r.POST("/api/session/forgot-username", lib.ChainMiddlewares(h.forgotUsername, middlewares...))
	r.GET("/api/session/users", lib.ChainMiddlewares(h.getUsers, middlewares...))
	r.POST("/api/session/users", lib.ChainMiddlewares(h.createUser, middlewares...))
	r.PUT("/api/session/users/{id}", lib.ChainMiddlewares(h.updateUser, middlewares...))
	r.DELETE("/api/session/users/{id}", lib.ChainMiddlewares(h.deleteUser, middlewares...))
	r.POST("/api/session/users/{id}/approve", lib.ChainMiddlewares(h.approveUser, middlewares...))
	r.POST("/api/session/users/{id}/reject", lib.ChainMiddlewares(h.rejectUser, middlewares...))
}

// isAuthEnabled handles GET /api/session/is-auth-enabled - Check if auth is enabled
func (h *SessionHandler) isAuthEnabled(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendJSON(ctx, map[string]any{
			"is_auth_enabled": false,
			"has_valid_token": false,
			"auth_type":       "none",
			"role":            "",
			"username":        "",
		})
		return
	}
	authConfig, err := h.configStore.GetAuthConfig(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get auth config: %v", err))
		return
	}
	if authConfig == nil {
		SendJSON(ctx, map[string]any{
			"is_auth_enabled": false,
			"has_valid_token": false,
			"auth_type":       "none",
			"role":            "",
			"username":        "",
		})
		return
	}
	// Check if the header has a token and is valid (Authorization header or cookie)
	token := ""
	if authHeader := string(ctx.Request.Header.Peek("Authorization")); strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		token = string(ctx.Request.Header.Cookie("token"))
	}
	hasValidToken := false
	role := ""
	username := ""
	allowedSections := ""
	if token != "" {
		session, err := h.configStore.GetSession(ctx, token)
		if err == nil && session != nil && session.ExpiresAt.After(time.Now()) {
			hasValidToken = true
			role = session.Role
			username = session.Username
			if username != "" {
				if dbUser, err := h.configStore.GetUserByUsername(ctx, username); err == nil && dbUser != nil {
					if !dbUser.IsApproved() {
						hasValidToken = false
						role = ""
					} else {
						allowedSections = dbUser.AllowedSections
					}
				}
			}
		}
	}
	SendJSON(ctx, map[string]any{
		"is_auth_enabled":  authConfig.IsEnabled,
		"has_valid_token":  hasValidToken,
		"auth_type":        dashboardAuthType(authConfig.IsEnabled),
		"role":             role,
		"username":         username,
		"allowed_sections": allowedSections,
	})
}

// dashboardAuthType reports the dashboard session auth mode for frontend flows.
func dashboardAuthType(isEnabled bool) string {
	if isEnabled {
		return "password"
	}
	return "none"
}

// recordAuthAudit logs authentication lifecycle events (login/logout) to the workspace audit trail.
func (h *SessionHandler) recordAuthAudit(ctx *fasthttp.RequestCtx, action, outcome, username, path string, start time.Time) {
	if ws, ok := configstore.AsWorkspaceStore(h.configStore); ok && ws != nil && username != "" {
		_ = ws.CreateAuditLog(ctx, &tables.TableAuditLog{
			Action:     action,
			Outcome:    outcome,
			Initiator:  username,
			Target:     path,
			Method:     "POST",
			Path:       path,
			IP:         ctx.RemoteIP().String(),
			DurationMs: time.Since(start).Milliseconds(),
			CreatedAt:  time.Now().UTC(),
		})
	}
}

// getAdminCredentials returns the bootstrap super admin username and email.
func (h *SessionHandler) getAdminCredentials(ctx *fasthttp.RequestCtx) (adminUsername, adminEmail string) {
	if h.configStore != nil {
		if authConfig, err := h.configStore.GetAuthConfig(ctx); err == nil && authConfig != nil {
			if authConfig.AdminUserName != nil {
				adminUsername = strings.TrimSpace(authConfig.AdminUserName.GetValue())
			}
			if authConfig.AdminEmail != nil {
				adminEmail = strings.TrimSpace(authConfig.AdminEmail.GetValue())
			}
		}
	}
	if adminUsername == "" {
		adminUsername = "admin"
	}
	if adminEmail == "" && strings.Contains(adminUsername, "@") && isValidEmail(adminUsername) {
		adminEmail = adminUsername
	}
	if adminEmail == "" {
		adminEmail = strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	}
	return adminUsername, adminEmail
}

// isMatchingAdminIdentity checks whether an entered identifier (username or email)
// corresponds to the built-in super admin account.
func isMatchingAdminIdentity(identity, adminUsername, adminEmail string) bool {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return false
	}
	if adminUsername != "" && strings.EqualFold(identity, adminUsername) {
		return true
	}
	if adminEmail != "" && strings.EqualFold(identity, adminEmail) {
		return true
	}
	if strings.EqualFold(identity, "admin") {
		return true
	}
	return false
}

// login handles POST /api/session/login - Login a user
func (h *SessionHandler) login(ctx *fasthttp.RequestCtx) {
	start := time.Now()
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusForbidden, "Authentication is not enabled")
		return
	}
	payload := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}

	// Get auth config
	authConfig, err := h.configStore.GetAuthConfig(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to get auth config: %v", err))
		return
	}

	// Check if auth is enabled
	if authConfig == nil || !authConfig.IsEnabled {
		SendError(ctx, fasthttp.StatusForbidden, "Authentication is not enabled")
		return
	}

	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Username == "" || payload.Password == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Username and password are required")
		return
	}
	if len(payload.Username) > 128 || len(payload.Password) > 128 {
		SendError(ctx, fasthttp.StatusBadRequest, "Username or password exceeds maximum allowed length")
		return
	}
	if blocked, retryAfter := checkLoginIPRateLimit(h.configStore, ctx); blocked {
		mins := lockoutRetryMinutes(ctx, retryAfter)
		SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many login attempts from this network. Try again in about %d minutes", mins))
		return
	}
	if locked, retryAfter, err := checkLoginLockout(h.configStore, ctx, payload.Username); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to check login lockout")
		return
	} else if locked {
		mins := lockoutRetryMinutes(ctx, retryAfter)
		SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many failed login attempts. Try again in about %d minutes", mins))
		return
	}

	// Verify credentials
	sessionRole := "admin"
	sessionUsername := payload.Username
	notifyEmail := ""
	sessionAllowedSections := ""
	authenticated := false

	adminName, adminEmail := h.getAdminCredentials(ctx)
	isBuiltinAdmin := isMatchingAdminIdentity(payload.Username, adminName, adminEmail)

	// Built-in admin ALWAYS wins over any shadowed DB user with the same username
	// (public register must not be able to lock out the bootstrap admin).
	if isBuiltinAdmin && authConfig.AdminPassword != nil {
		compare, cmpErr := encrypt.CompareHash(authConfig.AdminPassword.GetValue(), payload.Password)
		if cmpErr == nil && compare {
			sessionRole = "admin"
			sessionUsername = adminName
			authenticated = true
		} else {
			h.recordAuthAudit(ctx, "login", "failure", payload.Username, "/api/session/login", start)
			if blocked, retryAfter := recordLoginIPFailure(h.configStore, ctx); blocked {
				mins := lockoutRetryMinutes(ctx, retryAfter)
				SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many login attempts from this network. Try again in about %d minutes", mins))
				return
			}
			if locked, retryAfter := recordLoginFailure(h.configStore, ctx, payload.Username); locked {
				mins := lockoutRetryMinutes(ctx, retryAfter)
				SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many failed login attempts. Account locked for about %d minutes", mins))
				return
			}
			SendError(ctx, fasthttp.StatusUnauthorized, "Invalid username or password")
			return
		}
	}

	if !authenticated {
		dbUser, dbErr := h.configStore.GetUserByUsername(ctx, payload.Username)
		if dbErr == nil && dbUser != nil {
			compare, cmpErr := encrypt.CompareHash(dbUser.Password, payload.Password)
			if cmpErr != nil || !compare {
				h.recordAuthAudit(ctx, "login", "failure", payload.Username, "/api/session/login", start)
				if blocked, retryAfter := recordLoginIPFailure(h.configStore, ctx); blocked {
					mins := lockoutRetryMinutes(ctx, retryAfter)
					SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many login attempts from this network. Try again in about %d minutes", mins))
					return
				}
				if locked, retryAfter := recordLoginFailure(h.configStore, ctx, payload.Username); locked {
					mins := lockoutRetryMinutes(ctx, retryAfter)
					SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many failed login attempts. Account locked for about %d minutes", mins))
					return
				}
				SendError(ctx, fasthttp.StatusUnauthorized, "Invalid username or password")
				return
			}
			if !dbUser.IsApproved() {
				h.recordAuthAudit(ctx, "login", "failure", payload.Username, "/api/session/login", start)
				_, _ = recordLoginIPFailure(h.configStore, ctx)
				switch strings.ToLower(strings.TrimSpace(dbUser.Status)) {
				case tables.UserStatusPending:
					SendError(ctx, fasthttp.StatusForbidden, "Your registration is waiting for admin approval")
				case tables.UserStatusRejected:
					SendError(ctx, fasthttp.StatusForbidden, "Admin has not accepted your request")
				case tables.UserStatusEmailUnverified:
					SendError(ctx, fasthttp.StatusForbidden, "Verify your email first — enter the code we emailed you on the Sign Up page")
				default:
					SendError(ctx, fasthttp.StatusUnauthorized, "Invalid username or password")
				}
				return
			}
			sessionRole = dbUser.Role
			notifyEmail = dbUser.Email
			sessionAllowedSections = dbUser.AllowedSections
			authenticated = true
		} else {
			padPasswordCompare(payload.Password)
			h.recordAuthAudit(ctx, "login", "failure", payload.Username, "/api/session/login", start)
			if blocked, retryAfter := recordLoginIPFailure(h.configStore, ctx); blocked {
				mins := lockoutRetryMinutes(ctx, retryAfter)
				SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many login attempts from this network. Try again in about %d minutes", mins))
				return
			}
			if locked, retryAfter := recordLoginFailure(h.configStore, ctx, payload.Username); locked {
				mins := lockoutRetryMinutes(ctx, retryAfter)
				SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many failed login attempts. Account locked for about %d minutes", mins))
				return
			}
			SendError(ctx, fasthttp.StatusUnauthorized, "Invalid username or password")
			return
		}
	}

	clearLoginFailures(h.configStore, ctx, payload.Username)
	clearLoginIPFailures(h.configStore, ctx)

	// Creating a new session with secure lifetime (24 hours default, configurable via SESSION_LIFETIME_HOURS)
	sessionDuration := time.Hour * 24
	if rawHours := strings.TrimSpace(os.Getenv("SESSION_LIFETIME_HOURS")); rawHours != "" {
		if h, err := strconv.Atoi(rawHours); err == nil && h > 0 {
			sessionDuration = time.Duration(h) * time.Hour
		}
	}
	sessionExpiresAt := time.Now().Add(sessionDuration)
	// Invalidate prior sessions for this user (stolen-cookie / fixation mitigation).
	_ = h.configStore.DeleteSessionsByUsername(ctx, sessionUsername)
	if !strings.EqualFold(sessionUsername, payload.Username) {
		_ = h.configStore.DeleteSessionsByUsername(ctx, strings.TrimSpace(payload.Username))
	}
	token := uuid.New().String()
	session := &tables.SessionsTable{
		Token:     token,
		ExpiresAt: sessionExpiresAt,
		Username:  sessionUsername,
		Role:      sessionRole,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := h.configStore.CreateSession(ctx, session); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	// Setting cookies
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey("token")
	cookie.SetValue(token)
	cookie.SetExpire(sessionExpiresAt)
	cookie.SetPath("/")
	cookie.SetHTTPOnly(true)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	if cookieShouldBeSecure(ctx) {
		cookie.SetSecure(true)
	}
	ctx.Response.Header.SetCookie(cookie)

	trySendLoginNoticeEmail(h.configStore, ctx, sessionUsername, notifyEmail)
	h.recordAuthAudit(ctx, "login", "success", sessionUsername, "/api/session/login", start)

	resp := map[string]any{
		"message": "Login successful",
		"role":    sessionRole,
	}
	if sessionAllowedSections != "" {
		resp["allowed_sections"] = sessionAllowedSections
	}
	SendJSON(ctx, resp)
}

// logout handles POST /api/session/logout - Logout a user
func (h *SessionHandler) logout(ctx *fasthttp.RequestCtx) {
	start := time.Now()
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusForbidden, "Authentication is not enabled")
		return
	}
	// Get token from Authorization header
	token := string(ctx.Request.Header.Peek("Authorization"))
	token = strings.TrimPrefix(token, "Bearer ")

	// If no token in header, try to get from cookie
	if token == "" {
		token = string(ctx.Request.Header.Cookie("token"))
	}

	logoutUsername := "unknown"
	if token != "" {
		if sess, err := h.configStore.GetSession(ctx, token); err == nil && sess != nil {
			logoutUsername = sess.Username
		}
	}

	// clear token from cookies
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey("token")
	cookie.SetValue("")
	cookie.SetExpire(time.Now().Add(-time.Hour * 24 * 30))
	cookie.SetMaxAge(0)
	cookie.SetPath("/")
	cookie.SetHTTPOnly(true)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	if cookieShouldBeSecure(ctx) {
		cookie.SetSecure(true)
	}
	ctx.Response.Header.SetCookie(cookie)

	// delete session from database if token exists
	if token != "" {
		err := h.configStore.DeleteSession(ctx, token)
		if err != nil && !errors.Is(err, configstore.ErrNotFound) {
			logger.Error("failed to delete session during logout: %v", err)
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to invalidate session. Please try again.")
			return
		}
	}

	if logoutUsername != "unknown" {
		h.recordAuthAudit(ctx, "logout", "success", logoutUsername, "/api/session/logout", start)
	}

	SendJSON(ctx, map[string]any{
		"message": "Logout successful",
	})
}

// issueWSTicket handles POST /api/session/ws-ticket - Issue a short-lived ticket for WebSocket auth.
// The caller must already be authenticated (via cookie or Authorization header).
// Returns a one-time-use ticket that the frontend passes as ?ticket= when opening the WebSocket.
func (h *SessionHandler) issueWSTicket(ctx *fasthttp.RequestCtx) {
	if h.wsTicketStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "WebSocket tickets are not available")
		return
	}
	sessionToken, ok := ctx.UserValue(schemas.UnifAIContextKeySessionToken).(string)
	if !ok {
		SendError(ctx, fasthttp.StatusUnauthorized, "Unauthorized")
		return
	}
	if sessionToken == "" {
		// This is the case where auth is not configured or not enabled
		sessionToken = "dummy-session"
	}
	ticket, err := h.wsTicketStore.Issue(sessionToken)
	if err != nil {
		logger.Error("failed to issue WS ticket: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to issue WebSocket ticket")
		return
	}
	SendJSON(ctx, map[string]any{
		"ticket": ticket,
	})
}

// extractParam extracts a path param and sends an error if missing.
func (h *SessionHandler) extractParam(ctx *fasthttp.RequestCtx, name string) (string, bool) {
	val := ctx.UserValue(name)
	if val == nil {
		SendError(ctx, fasthttp.StatusBadRequest, name+" is required")
		return "", false
	}
	s, ok := val.(string)
	if !ok || s == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid "+name)
		return "", false
	}
	return s, true
}

func alreadyRegisteredMessage(existing *tables.TableUser) string {
	_ = existing
	// Generic message — do not leak pending/approved/rejected status.
	return "Unable to complete registration with this username. Try a different username or sign in if you already have an account."
}

// userCreateFailureMessage maps DB/create errors into a clear admin-facing toast.
func userCreateFailureMessage(err error) string {
	if err == nil {
		return "Failed to create user"
	}
	errLower := strings.ToLower(err.Error())
	if strings.Contains(errLower, "duplicate") || strings.Contains(errLower, "unique") {
		if strings.Contains(errLower, "email") {
			return "This email is already registered with another account"
		}
		return "Username is already registered"
	}
	if strings.Contains(errLower, "column") || strings.Contains(errLower, "does not exist") {
		return "Database is missing user columns (budget/rate limit); restart the gateway to apply migrations"
	}
	return "Failed to create user"
}

// isValidEmail validates an email address syntax.
func isValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return true
}

// assertEmailAvailable rejects when another user already owns this email.
// exceptUserID allows the same user to keep/update their own email.
// Returns false after sending the HTTP error.
func (h *SessionHandler) assertEmailAvailable(ctx *fasthttp.RequestCtx, email, exceptUserID string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || h.configStore == nil {
		return true
	}
	other, err := h.configStore.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			return true
		}
		logger.Error("failed to validate email availability: %v", err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to validate email — check database connection")
		return false
	}
	if other == nil {
		return true
	}
	if exceptUserID != "" && other.ID == exceptUserID {
		return true
	}
	SendError(ctx, fasthttp.StatusConflict, "Unable to complete registration with this email. Try a different email or sign in if you already have an account.")
	return false
}

// isAdmin checks if the current request session belongs to an admin.
// When dashboard auth is disabled, open admin is allowed only from loopback
// (or ALLOW_OPEN_AUTH=1) so remote callers cannot manage users unauthenticated.
func (h *SessionHandler) isAdmin(ctx *fasthttp.RequestCtx) bool {
	if h.configStore == nil {
		return allowOpenAuthWhenDisabled(ctx)
	}
	authConfig, err := h.configStore.GetAuthConfig(ctx)
	if err != nil {
		return false
	}
	if authConfig == nil || !authConfig.IsEnabled {
		return allowOpenAuthWhenDisabled(ctx)
	}

	token := ""
	if authHeader := string(ctx.Request.Header.Peek("Authorization")); strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		token = string(ctx.Request.Header.Cookie("token"))
	}
	if token == "" {
		return false
	}
	session, err := h.configStore.GetSession(ctx, token)
	if err != nil || session == nil || session.ExpiresAt.Before(time.Now()) {
		return false
	}
	return session.Role == "admin" || session.Role == "sub_admin"
}

func (h *SessionHandler) isSuperAdmin(ctx *fasthttp.RequestCtx) bool {
	if h.configStore == nil {
		return false
	}
	token := ""
	if authHeader := string(ctx.Request.Header.Peek("Authorization")); strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		token = string(ctx.Request.Header.Cookie("token"))
	}
	if token == "" {
		return false
	}
	session, err := h.configStore.GetSession(ctx, token)
	if err != nil || session == nil || session.ExpiresAt.Before(time.Now()) {
		return false
	}
	return session.Role == "admin"
}

// getUsers handles GET /api/session/users - Get all users (Admin only)
func (h *SessionHandler) getUsers(ctx *fasthttp.RequestCtx) {
	if !h.isAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	users, err := h.configStore.GetUsers(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	visible := make([]map[string]any, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		// Denied rows stay in DB for login messaging, but are not listed as users.
		if u.Status == tables.UserStatusRejected {
			continue
		}
		item := map[string]any{
			"id":                   u.ID,
			"username":             u.Username,
			"email":                u.Email,
			"role":                 u.Role,
			"status":               u.Status,
			"budget":               u.Budget,
			"rate_limit":           u.RateLimit,
			"budget_id":            u.BudgetID,
			"rate_limit_id":        u.RateLimitID,
			"allowed_prompt_repos": u.AllowedPromptRepos,
			"allowed_sections":     u.AllowedSections,
			"created_at":           u.CreatedAt,
			"updated_at":           u.UpdatedAt,
		}
		if u.BudgetID != nil && *u.BudgetID != "" {
			if b, err := h.configStore.GetBudget(ctx, *u.BudgetID); err == nil && b != nil {
				item["budget_current_usage"] = b.CurrentUsage
			}
		}
		visible = append(visible, item)
	}
	SendJSON(ctx, visible)
}

// createUser handles POST /api/session/users - Create a new user (Admin only)
func (h *SessionHandler) createUser(ctx *fasthttp.RequestCtx) {
	if !h.isAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	var payload struct {
		Username           string  `json:"username"`
		Email              string  `json:"email"`
		Password           string  `json:"password"`
		Role               string  `json:"role"`
		Budget             float64 `json:"budget"`
		RateLimit          int     `json:"rate_limit"`
		AllowedPromptRepos string  `json:"allowed_prompt_repos"`
		AllowedSections    string  `json:"allowed_sections"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	payload.Email = strings.TrimSpace(strings.ToLower(payload.Email))
	if payload.Username == "" || payload.Password == "" || payload.Email == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Username, email, and password are required")
		return
	}
	if len(payload.Username) > 128 || len(payload.Password) > 128 || len(payload.Email) > 254 {
		SendError(ctx, fasthttp.StatusBadRequest, "Username, email, or password exceeds maximum allowed length")
		return
	}
	if !isValidEmail(payload.Email) {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid email address format")
		return
	}
	if authConfig, err := h.configStore.GetAuthConfig(ctx); err == nil && authConfig != nil && authConfig.AdminUserName != nil {
		if strings.EqualFold(payload.Username, authConfig.AdminUserName.GetValue()) {
			SendError(ctx, fasthttp.StatusBadRequest, "Username matches the built-in admin account. Use a different username.")
			return
		}
	}
	if failures := getPasswordPolicyFailures(payload.Password); len(failures) > 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "Password must include "+strings.Join(failures, ", "))
		return
	}
	role, roleOK := h.normalizeUserRole(ctx, payload.Role)
	if !roleOK {
		SendError(ctx, fasthttp.StatusBadRequest, "Unknown role — create it under Roles & Permissions first, or use admin/user")
		return
	}
	if role == "admin" && !h.isSuperAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Only a super admin can create admin accounts")
		return
	}
	payload.Role = role

	hashedPassword, err := encrypt.Hash(payload.Password)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to hash password")
		return
	}

	now := time.Now()
	existing, err := h.configStore.GetUserByUsername(ctx, payload.Username)
	if err != nil && !errors.Is(err, configstore.ErrNotFound) {
		logger.Error("failed to lookup governance user username=%s: %v", payload.Username, err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to validate username — check database connection")
		return
	}
	if existing != nil {
		if existing.IsApproved() {
			SendError(ctx, fasthttp.StatusConflict, "Username is already registered")
			return
		}
		// Admin create bypasses pending/denied — activate immediately.
		if !h.assertEmailAvailable(ctx, payload.Email, existing.ID) {
			return
		}
		existing.Email = payload.Email
		existing.Password = hashedPassword
		existing.Role = payload.Role
		existing.Status = tables.UserStatusApproved
		existing.Budget = payload.Budget
		existing.RateLimit = payload.RateLimit
		existing.AllowedPromptRepos = payload.AllowedPromptRepos
		existing.AllowedSections = payload.AllowedSections
		existing.ReviewedAt = &now
		existing.UpdatedAt = now
		if err := h.persistUserWithGovernance(ctx, existing, false); err != nil {
			logger.Error("failed to activate pending governance user username=%s: %v", payload.Username, err)
			SendError(ctx, fasthttp.StatusInternalServerError, userCreateFailureMessage(err))
			return
		}
		existing.Password = ""
		emailTo := strings.TrimSpace(payload.Email)
		if emailTo == "" {
			emailTo = strings.TrimSpace(existing.Email)
		}
		emailSent, emailErr := trySendWelcomeEmail(h.configStore, ctx, payload.Username, emailTo, payload.Password)
		SendJSON(ctx, map[string]any{
			"id":                   existing.ID,
			"username":             existing.Username,
			"email":                existing.Email,
			"role":                 existing.Role,
			"status":               existing.Status,
			"budget":               existing.Budget,
			"rate_limit":           existing.RateLimit,
			"budget_id":            existing.BudgetID,
			"rate_limit_id":        existing.RateLimitID,
			"allowed_prompt_repos": existing.AllowedPromptRepos,
			"allowed_sections":     existing.AllowedSections,
			"created_at":           existing.CreatedAt,
			"email_sent":           emailSent,
			"email_error":          emailErr,
		})
		return
	}

	if !h.assertEmailAvailable(ctx, payload.Email, "") {
		return
	}

	user := &tables.TableUser{
		ID:                 uuid.New().String(),
		Username:           payload.Username,
		Email:              payload.Email,
		Password:           hashedPassword,
		Role:               payload.Role,
		Status:             tables.UserStatusApproved,
		Budget:             payload.Budget,
		RateLimit:          payload.RateLimit,
		AllowedPromptRepos: payload.AllowedPromptRepos,
		AllowedSections:    payload.AllowedSections,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := h.persistUserWithGovernance(ctx, user, true); err != nil {
		logger.Error("failed to create governance user username=%s: %v", payload.Username, err)
		SendError(ctx, fasthttp.StatusInternalServerError, userCreateFailureMessage(err))
		return
	}

	user.Password = ""
	emailSent, emailErr := trySendWelcomeEmail(h.configStore, ctx, payload.Username, payload.Email, payload.Password)
	SendJSON(ctx, map[string]any{
		"id":                   user.ID,
		"username":             user.Username,
		"email":                user.Email,
		"role":                 user.Role,
		"status":               user.Status,
		"budget":               user.Budget,
		"rate_limit":           user.RateLimit,
		"budget_id":            user.BudgetID,
		"rate_limit_id":        user.RateLimitID,
		"allowed_prompt_repos": user.AllowedPromptRepos,
		"allowed_sections":     user.AllowedSections,
		"created_at":           user.CreatedAt,
		"email_sent":           emailSent,
		"email_error":          emailErr,
	})
}

// persistUserWithGovernance writes the user + budget/rate-limit rows in one DB transaction.
// create=true inserts the user; create=false updates an existing row.
func (h *SessionHandler) persistUserWithGovernance(ctx *fasthttp.RequestCtx, user *tables.TableUser, create bool) error {
	err := h.configStore.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
		if create {
			if err := h.configStore.CreateUser(ctx, user, tx); err != nil {
				return err
			}
		} else {
			if err := h.configStore.UpdateUser(ctx, user, tx); err != nil {
				return err
			}
		}
		if err := h.materializeUserGovernanceLimits(ctx, user, tx); err != nil {
			return err
		}
		return h.configStore.UpdateUser(ctx, user, tx)
	})
	if err != nil {
		return err
	}
	h.syncUserGovernanceMemory(ctx, user)
	return nil
}

// forgotPassword emails a 6-digit OTP when the account has an email on file.
func (h *SessionHandler) forgotPassword(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store not available")
		return
	}
	var payload struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	payload.Email = strings.TrimSpace(payload.Email)

	// Always return the same message to avoid account enumeration.
	generic := map[string]any{"message": "If an account matches, a one-time code was sent by email"}

	// Consume quota before lookup so unknown targets cannot bypass IP/target limits.
	targetKey := strings.ToLower(strings.TrimSpace(payload.Email))
	if targetKey == "" {
		targetKey = strings.ToLower(strings.TrimSpace(payload.Username))
	}
	if targetKey == "" {
		targetKey = "anonymous"
	}
	if consumeForgotPasswordQuota(h.configStore, ctx, clientIPAddress(ctx), targetKey) {
		SendJSON(ctx, generic)
		return
	}

	var user *tables.TableUser
	if payload.Username != "" {
		if u, err := h.configStore.GetUserByUsername(ctx, payload.Username); err == nil {
			user = u
		}
	}
	if user == nil && payload.Email != "" {
		if u, err := h.configStore.GetUserByEmail(ctx, payload.Email); err == nil {
			user = u
		}
	}

	targetUsername := ""
	targetEmail := ""
	if user != nil {
		if !user.IsApproved() || strings.TrimSpace(user.Email) == "" {
			padPasswordCompare("dummy-otp-pad")
			SendJSON(ctx, generic)
			return
		}
		targetUsername = user.Username
		targetEmail = user.Email
	} else {
		// Check if request matches bootstrap Super Admin
		adminName, adminEmail := h.getAdminCredentials(ctx)
		if adminEmail != "" && (isMatchingAdminIdentity(payload.Username, adminName, adminEmail) || isMatchingAdminIdentity(payload.Email, adminName, adminEmail)) {
			targetUsername = adminName
			targetEmail = adminEmail
		} else {
			padPasswordCompare("dummy-otp-pad")
			SendJSON(ctx, generic)
			return
		}
	}

	otp, err := generateOTP6()
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to generate OTP")
		return
	}
	hash, err := encrypt.Hash(otp)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to store OTP")
		return
	}
	row := &tables.TablePasswordResetOTP{
		Username:  targetUsername,
		Email:     targetEmail,
		OTPHash:   hash,
		ExpiresAt: time.Now().Add(passwordResetOTPTTL),
		CreatedAt: time.Now(),
	}
	if err := h.configStore.CreatePasswordResetOTP(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to store OTP")
		return
	}
	body := fmt.Sprintf(
		"Hello %s,\n\nYour UnifAI password reset code is: %s\n\nIt expires in %d minutes. If you did not request this, ignore this email.\n",
		targetUsername, otp, int(passwordResetOTPTTL.Minutes()),
	)
	if err := sendAuthEmail(h.configStore, ctx, targetEmail, "UnifAI password reset code", body); err != nil {
		logger.Warn("password reset OTP email failed username=%s: %v", targetUsername, err)
		// Always return generic success — do not leak account existence via SMTP errors.
		SendJSON(ctx, generic)
		return
	}
	SendJSON(ctx, generic)
}

// verifyOTP handles POST /api/session/verify-otp - Verify reset OTP before setting a new password.
func (h *SessionHandler) verifyOTP(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store not available")
		return
	}
	// Hourly per-IP cap on OTP guesses (each code also allows only maxOTPAttempts).
	// No per-target cooldown here: it rejected the correct code after a single typo.
	if consumeForgotPasswordQuota(h.configStore, ctx, "otp-verify:"+clientIPAddress(ctx), "") {
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired OTP")
		return
	}
	var payload struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		OTP      string `json:"otp"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	payload.Email = strings.TrimSpace(payload.Email)
	payload.OTP = strings.TrimSpace(payload.OTP)
	if (payload.Username == "" && payload.Email == "") || payload.OTP == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Email and OTP are required")
		return
	}

	var user *tables.TableUser
	if payload.Email != "" {
		if u, err := h.configStore.GetUserByEmail(ctx, payload.Email); err == nil {
			user = u
		}
	}
	if user == nil && payload.Username != "" {
		if u, err := h.configStore.GetUserByUsername(ctx, payload.Username); err == nil {
			user = u
		}
		if user == nil {
			if u, err := h.configStore.GetUserByEmail(ctx, payload.Username); err == nil {
				user = u
			}
		}
	}

	targetUsername := ""
	targetEmail := ""
	if user != nil {
		if !user.IsApproved() {
			padPasswordCompare(payload.OTP)
			SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired OTP")
			return
		}
		targetUsername = user.Username
		targetEmail = user.Email
	} else {
		adminName, adminEmail := h.getAdminCredentials(ctx)
		if adminEmail != "" && (isMatchingAdminIdentity(payload.Username, adminName, adminEmail) || isMatchingAdminIdentity(payload.Email, adminName, adminEmail)) {
			targetUsername = adminName
			targetEmail = adminEmail
		} else {
			padPasswordCompare(payload.OTP)
			SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired OTP")
			return
		}
	}

	otpRow, err := h.configStore.GetLatestPasswordResetOTP(ctx, targetUsername)
	if (err != nil || otpRow == nil) && targetEmail != "" && targetEmail != targetUsername {
		otpRow, err = h.configStore.GetLatestPasswordResetOTP(ctx, targetEmail)
	}
	if err != nil || otpRow == nil {
		padPasswordCompare(payload.OTP)
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired OTP")
		return
	}
	if time.Now().After(otpRow.ExpiresAt) {
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired OTP")
		return
	}
	if otpRow.FailedAttempts >= maxOTPAttempts {
		_ = h.configStore.MarkPasswordResetOTPUsed(ctx, otpRow.ID)
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired OTP")
		return
	}
	ok, err := encrypt.CompareHash(otpRow.OTPHash, payload.OTP)
	if err != nil || !ok {
		fails, _ := h.configStore.IncrementPasswordResetOTPFailures(ctx, otpRow.ID)
		if fails >= maxOTPAttempts {
			_ = h.configStore.MarkPasswordResetOTPUsed(ctx, otpRow.ID)
		}
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired OTP")
		return
	}

	// Burn the raw OTP so it cannot be replayed; short-lived reset_token is required next.
	_ = h.configStore.BurnPasswordResetOTPCode(ctx, otpRow.ID)

	resetToken, err := issuePasswordResetToken(otpRow.ID, targetUsername, targetEmail)
	if err != nil {
		logger.Error("failed to issue password reset token for user=%s: %v (check PASSWORD_RESET_SECRET env var)", targetUsername, err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Password reset is not configured on this server. Please contact your administrator.")
		return
	}

	SendJSON(ctx, map[string]any{
		"valid":       true,
		"reset_token": resetToken,
		"message":     "OTP verified successfully",
	})
}

// forgotUsername handles POST /api/session/forgot-username - Send username to user's email.
func (h *SessionHandler) forgotUsername(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store not available")
		return
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	payload.Email = strings.TrimSpace(strings.ToLower(payload.Email))
	if payload.Email == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Email is required")
		return
	}

	generic := map[string]any{"message": "If an account matches, your username was sent to your email."}

	if checkForgotUsernameCooldown(h.configStore, ctx, payload.Email) || checkForgotUsernameCooldown(h.configStore, ctx, clientIPAddress(ctx)) {
		SendJSON(ctx, generic)
		return
	}

	user, err := h.configStore.GetUserByEmail(ctx, payload.Email)
	if err != nil || user == nil || !user.IsApproved() || strings.TrimSpace(user.Email) == "" {
		adminName, adminEmail := h.getAdminCredentials(ctx)
		if adminEmail != "" && (strings.EqualFold(payload.Email, adminEmail) || isMatchingAdminIdentity(payload.Email, adminName, adminEmail)) {
			body := fmt.Sprintf(
				"Hello,\n\nYour UnifAI administrator username is: %s\n(You can also sign in with username 'admin' or your email: %s)\n\nIf you did not request this, please ignore this email.\n",
				adminName, adminEmail,
			)
			if err := sendAuthEmail(h.configStore, ctx, adminEmail, "Your UnifAI Username", body); err != nil {
				logger.Warn("forgot username email failed for admin email=%s: %v", adminEmail, err)
			}
		}
		SendJSON(ctx, generic)
		return
	}

	body := fmt.Sprintf(
		"Hello,\n\nYour UnifAI username associated with this email address is: %s\n\nIf you did not request this, please ignore this email.\n",
		user.Username,
	)
	if err := sendAuthEmail(h.configStore, ctx, user.Email, "Your UnifAI Username", body); err != nil {
		logger.Warn("forgot username email failed for email=%s: %v", user.Email, err)
		// Always generic — do not leak account existence via SMTP errors.
		SendJSON(ctx, generic)
		return
	}

	SendJSON(ctx, generic)
}

// resetPassword verifies a signed reset_token (from verify-otp) and sets a new password.
// Raw OTP alone is rejected — OTP must be verified first so the code is burned.
func (h *SessionHandler) resetPassword(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store not available")
		return
	}
	var payload struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		OTP         string `json:"otp"`
		ResetToken  string `json:"reset_token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	payload.Email = strings.TrimSpace(payload.Email)
	payload.ResetToken = strings.TrimSpace(payload.ResetToken)
	if (payload.Username == "" && payload.Email == "") || payload.ResetToken == "" || payload.NewPassword == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Email, reset token, and new password are required. Verify OTP first.")
		return
	}
	if len(payload.NewPassword) > 128 {
		SendError(ctx, fasthttp.StatusBadRequest, "Password exceeds maximum allowed length (128 characters)")
		return
	}
	if failures := getPasswordPolicyFailures(payload.NewPassword); len(failures) > 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "Password must include "+strings.Join(failures, ", "))
		return
	}

	var user *tables.TableUser
	if payload.Email != "" {
		if u, err := h.configStore.GetUserByEmail(ctx, payload.Email); err == nil {
			user = u
		}
	}
	if user == nil && payload.Username != "" {
		if u, err := h.configStore.GetUserByUsername(ctx, payload.Username); err == nil {
			user = u
		}
		if user == nil {
			if u, err := h.configStore.GetUserByEmail(ctx, payload.Username); err == nil {
				user = u
			}
		}
	}

	if user != nil {
		if !user.IsApproved() {
			SendError(ctx, fasthttp.StatusUnauthorized, "Invalid reset token or account")
			return
		}
		sameAsCurrent, cmpErr := encrypt.CompareHash(user.Password, payload.NewPassword)
		if cmpErr == nil && sameAsCurrent {
			SendError(ctx, fasthttp.StatusBadRequest, "New password must be different from your current password")
			return
		}

		claims, err := verifyPasswordResetToken(payload.ResetToken)
		if err != nil {
			SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired reset token. Please request a new verification code.")
			return
		}
		if !strings.EqualFold(claims.Username, user.Username) && (claims.Email == "" || !strings.EqualFold(claims.Email, user.Email)) {
			SendError(ctx, fasthttp.StatusUnauthorized, "Reset token does not match this account")
			return
		}
		otpRow, err := h.configStore.GetPasswordResetOTPByID(ctx, claims.OTPID)
		if err != nil || otpRow == nil || otpRow.Used {
			SendError(ctx, fasthttp.StatusUnauthorized, "Reset token has already been consumed or expired. Please request a new code.")
			return
		}
		if time.Now().After(otpRow.ExpiresAt) {
			SendError(ctx, fasthttp.StatusUnauthorized, "Reset token has expired. Please request a new code.")
			return
		}
		if !strings.EqualFold(otpRow.Username, user.Username) {
			SendError(ctx, fasthttp.StatusUnauthorized, "Reset token does not match this account")
			return
		}
		targetOTPID := otpRow.ID

		hashed, err := encrypt.Hash(payload.NewPassword)
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to hash password")
			return
		}
		user.Password = hashed
		user.UpdatedAt = time.Now()
		if err := h.configStore.UpdateUser(ctx, user); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to update password")
			return
		}
		if targetOTPID > 0 {
			_ = h.configStore.MarkPasswordResetOTPUsed(ctx, targetOTPID)
		}
		// Terminate any existing live sessions on password reset (CWE-613)
		_ = h.configStore.DeleteSessionsByUsername(ctx, user.Username)
		// Do not clear login lockout here — failed-login lockout must still apply
		// until the timer expires (forgot-password must not bypass the lock).
		msg := "Password updated. You can sign in now."
		if locked, retryAfter, _ := checkLoginLockout(h.configStore, ctx, user.Username); locked {
			mins := retryMinutes(retryAfter)
			msg = fmt.Sprintf("Password updated. Your account is still locked for about %d minutes after failed logins — wait, then sign in with the new password.", mins)
		}
		SendJSON(ctx, map[string]any{"message": msg})
		return
	}

	// Super Admin flow
	adminName, adminEmail := h.getAdminCredentials(ctx)
	isSuperAdmin := isMatchingAdminIdentity(payload.Username, adminName, adminEmail) || isMatchingAdminIdentity(payload.Email, adminName, adminEmail)
	if !isSuperAdmin {
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid reset token or account")
		return
	}

	authConfig, err := h.configStore.GetAuthConfig(ctx)
	if err != nil || authConfig == nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Auth configuration is not available")
		return
	}

	if authConfig.AdminPassword != nil && authConfig.AdminPassword.GetValue() != "" {
		sameAsCurrent, cmpErr := encrypt.CompareHash(authConfig.AdminPassword.GetValue(), payload.NewPassword)
		if cmpErr == nil && sameAsCurrent {
			SendError(ctx, fasthttp.StatusBadRequest, "New password must be different from your current password")
			return
		}
	}

	claims, err := verifyPasswordResetToken(payload.ResetToken)
	if err != nil {
		SendError(ctx, fasthttp.StatusUnauthorized, "Invalid or expired reset token. Please request a new verification code.")
		return
	}
	if !isMatchingAdminIdentity(claims.Username, adminName, adminEmail) &&
		(claims.Email == "" || !isMatchingAdminIdentity(claims.Email, adminName, adminEmail)) {
		SendError(ctx, fasthttp.StatusUnauthorized, "Reset token does not match this account")
		return
	}

	otpRow, err := h.configStore.GetPasswordResetOTPByID(ctx, claims.OTPID)
	if err != nil || otpRow == nil || otpRow.Used {
		SendError(ctx, fasthttp.StatusUnauthorized, "Reset token has already been consumed or expired. Please request a new code.")
		return
	}
	if time.Now().After(otpRow.ExpiresAt) {
		SendError(ctx, fasthttp.StatusUnauthorized, "Reset token has expired. Please request a new code.")
		return
	}
	if !isMatchingAdminIdentity(otpRow.Username, adminName, adminEmail) &&
		(otpRow.Email == "" || !isMatchingAdminIdentity(otpRow.Email, adminName, adminEmail)) {
		SendError(ctx, fasthttp.StatusUnauthorized, "Reset token does not match this account")
		return
	}

	newHashedPassword, err := encrypt.Hash(payload.NewPassword)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to hash password")
		return
	}

	authConfig.AdminPassword = schemas.NewSecretVar(newHashedPassword)
	if authConfig.AdminEmail == nil && adminEmail != "" {
		authConfig.AdminEmail = schemas.NewSecretVar(adminEmail)
	}
	if err := h.configStore.UpdateAuthConfig(ctx, authConfig); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to update password")
		return
	}

	_ = h.configStore.MarkPasswordResetOTPUsed(ctx, otpRow.ID)
	_ = h.configStore.DeleteSessionsByUsername(ctx, adminName)
	if adminEmail != "" && adminEmail != adminName {
		_ = h.configStore.DeleteSessionsByUsername(ctx, adminEmail)
	}
	_ = h.configStore.FlushSessions(ctx)

	msg := "Password updated. You can sign in now."
	if locked, retryAfter, _ := checkLoginLockout(h.configStore, ctx, adminName); locked {
		mins := retryMinutes(retryAfter)
		msg = fmt.Sprintf("Password updated. Your account is still locked for about %d minutes after failed logins — wait, then sign in with the new password.", mins)
	}
	SendJSON(ctx, map[string]any{"message": msg})
}

// updateUser handles PUT /api/session/users/{id} - Update user (Admin only)
func (h *SessionHandler) updateUser(ctx *fasthttp.RequestCtx) {
	if !h.isAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	id, ok := h.extractParam(ctx, "id")
	if !ok {
		return
	}

	existingUser, err := h.configStore.GetUserByID(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "User not found")
		return
	}

	if existingUser.Role == "admin" && !h.isSuperAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Cannot modify an admin account")
		return
	}

	prevRole := existingUser.Role
	prevUsername := existingUser.Username
	roleChanged := false

	var payload struct {
		Username           string  `json:"username"`
		Password           string  `json:"password"`
		Role               string  `json:"role"`
		Email              *string `json:"email"`
		Status             *string `json:"status"`
		Budget             float64 `json:"budget"`
		RateLimit          int     `json:"rate_limit"`
		AllowedPromptRepos *string `json:"allowed_prompt_repos"`
		AllowedSections    *string `json:"allowed_sections"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}

	if newName := strings.TrimSpace(payload.Username); newName != "" && newName != existingUser.Username {
		if len(newName) > 128 {
			SendError(ctx, fasthttp.StatusBadRequest, "Username exceeds maximum allowed length")
			return
		}
		reserved := strings.EqualFold(newName, "admin")
		if authConfig, err := h.configStore.GetAuthConfig(ctx); err == nil && authConfig != nil && authConfig.AdminUserName != nil &&
			strings.EqualFold(newName, strings.TrimSpace(authConfig.AdminUserName.GetValue())) {
			reserved = true
		}
		if reserved {
			SendError(ctx, fasthttp.StatusBadRequest, "Username matches the built-in admin account. Use a different username.")
			return
		}
		if other, err := h.configStore.GetUserByUsername(ctx, newName); err == nil && other != nil && other.ID != existingUser.ID {
			SendError(ctx, fasthttp.StatusConflict, "Username is already taken")
			return
		}
		existingUser.Username = newName
	}
	if payload.Email != nil {
		newEmail := strings.TrimSpace(strings.ToLower(*payload.Email))
		if newEmail == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "Email cannot be empty")
			return
		}
		if !isValidEmail(newEmail) {
			SendError(ctx, fasthttp.StatusBadRequest, "Invalid email address format")
			return
		}
		if !h.assertEmailAvailable(ctx, newEmail, existingUser.ID) {
			return
		}
		existingUser.Email = newEmail
	}
	if payload.Status != nil && (*payload.Status == tables.UserStatusApproved || *payload.Status == tables.UserStatusPending || *payload.Status == tables.UserStatusRejected) {
		existingUser.Status = *payload.Status
	}
	if payload.Password != "" {
		if failures := getPasswordPolicyFailures(payload.Password); len(failures) > 0 {
			SendError(ctx, fasthttp.StatusBadRequest, "Password must include "+strings.Join(failures, ", "))
			return
		}
		hashedPassword, err := encrypt.Hash(payload.Password)
		if err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to hash password")
			return
		}
		existingUser.Password = hashedPassword
	}
	if payload.Role != "" {
		role, ok := h.normalizeUserRole(ctx, payload.Role)
		if !ok {
			SendError(ctx, fasthttp.StatusBadRequest, "Unknown role — create it under Roles & Permissions first, or use admin/user")
			return
		}
		if role == "admin" && prevRole != "admin" && !h.isSuperAdmin(ctx) {
			SendError(ctx, fasthttp.StatusForbidden, "Only a super admin can promote users to admin")
			return
		}
		if prevRole == "admin" && role != "admin" && !h.isSuperAdmin(ctx) {
			SendError(ctx, fasthttp.StatusForbidden, "Only a super admin can change an admin account role")
			return
		}
		existingUser.Role = role
		roleChanged = role != prevRole
	}
	existingUser.Budget = payload.Budget
	existingUser.RateLimit = payload.RateLimit
	if payload.AllowedPromptRepos != nil {
		existingUser.AllowedPromptRepos = *payload.AllowedPromptRepos
	}
	if payload.AllowedSections != nil {
		existingUser.AllowedSections = *payload.AllowedSections
	}
	existingUser.UpdatedAt = time.Now()

	if err := h.persistUserWithGovernance(ctx, existingUser, false); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, userCreateFailureMessage(err))
		return
	}

	// Terminate sessions on password change / reject; sync or revoke on role change.
	if payload.Password != "" || (payload.Status != nil && *payload.Status == tables.UserStatusRejected) {
		_ = h.configStore.DeleteSessionsByUsername(ctx, existingUser.Username)
		if prevUsername != "" && prevUsername != existingUser.Username {
			_ = h.configStore.DeleteSessionsByUsername(ctx, prevUsername)
		}
	} else if roleChanged {
		// Keep user signed in but refresh RBAC immediately on all live sessions.
		_ = h.configStore.UpdateSessionsRoleByUsername(ctx, existingUser.Username, existingUser.Role)
		if prevUsername != "" && prevUsername != existingUser.Username {
			_ = h.configStore.UpdateSessionsRoleByUsername(ctx, prevUsername, existingUser.Role)
		}
	}

	existingUser.Password = ""
	SendJSON(ctx, existingUser)
}

// deleteUser handles DELETE /api/session/users/{id} - Delete user (Admin only)
func (h *SessionHandler) deleteUser(ctx *fasthttp.RequestCtx) {
	if !h.isAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	id, ok := h.extractParam(ctx, "id")
	if !ok {
		return
	}

	existing, err := h.configStore.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			SendError(ctx, fasthttp.StatusNotFound, "User not found")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to load user: "+err.Error())
		return
	}

	if existing.Role == "admin" && !h.isSuperAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Cannot delete an admin account")
		return
	}

	// Clean related governance rows before deleting the user so DB stays consistent.
	if existing.BudgetID != nil && *existing.BudgetID != "" {
		if err := h.configStore.DeleteBudget(ctx, *existing.BudgetID); err != nil && !errors.Is(err, configstore.ErrNotFound) {
			logger.Error("failed to delete user budget id=%s: %v", *existing.BudgetID, err)
		}
	}
	_ = h.configStore.ExecuteTransaction(ctx, func(tx *gorm.DB) error {
		if tx == nil {
			return nil
		}
		return tx.Where("user_id = ?", id).Delete(&tables.TableBudget{}).Error
	})
	if existing.RateLimitID != nil && *existing.RateLimitID != "" {
		if err := h.configStore.DeleteRateLimit(ctx, *existing.RateLimitID); err != nil && !errors.Is(err, configstore.ErrNotFound) {
			logger.Error("failed to delete user rate limit id=%s: %v", *existing.RateLimitID, err)
		}
	}
	if ws, ok := configstore.AsWorkspaceStore(h.configStore); ok && ws != nil {
		if memberships, err := ws.ListTeamsForUser(ctx, id); err == nil {
			for _, m := range memberships {
				_ = ws.RemoveTeamMember(ctx, m.TeamID, id)
			}
		}
	}

	if err := h.configStore.DeleteUser(ctx, id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to delete user: "+err.Error())
		return
	}
	// Terminate any live sessions for the deleted user
	_ = h.configStore.DeleteSessionsByUsername(ctx, existing.Username)
	if h.userGovernance != nil {
		h.userGovernance.DeleteUserGovernance(ctx, id)
	}

	SendJSON(ctx, map[string]any{
		"message": "User deleted successfully",
	})
}

// register handles POST /api/session/register - public self-registration (pending approval).
func (h *SessionHandler) register(ctx *fasthttp.RequestCtx) {
	if h.configStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "User registration is not available")
		return
	}
	var payload struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	payload.Email = strings.TrimSpace(strings.ToLower(payload.Email))
	if payload.Username == "" || payload.Password == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Username and password are required")
		return
	}
	if len(payload.Username) > 128 || len(payload.Password) > 128 || len(payload.Email) > 254 {
		SendError(ctx, fasthttp.StatusBadRequest, "Username, email, or password exceeds maximum allowed length")
		return
	}
	if payload.Email == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Email is required")
		return
	}
	if !isValidEmail(payload.Email) {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid email address format")
		return
	}

	// Forbid registering the built-in administrator account username to prevent account shadowing
	if authConfig, err := h.configStore.GetAuthConfig(ctx); err == nil && authConfig != nil {
		if authConfig.AdminUserName != nil {
			adminName := strings.TrimSpace(authConfig.AdminUserName.GetValue())
			if adminName != "" && strings.EqualFold(payload.Username, adminName) {
				SendError(ctx, fasthttp.StatusConflict, "Username is unavailable. Try a different username.")
				return
			}
		}
	}
	// Always reserve the literal "admin" username even if AuthConfig admin name differs.
	if strings.EqualFold(payload.Username, "admin") {
		SendError(ctx, fasthttp.StatusConflict, "Username is unavailable. Try a different username.")
		return
	}

	if failures := getPasswordPolicyFailures(payload.Password); len(failures) > 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "Password must include "+strings.Join(failures, ", "))
		return
	}
	// Count only requests that pass validation — weak/invalid payloads must not burn the IP quota.
	if consumeRegistrationQuota(h.configStore, ctx) {
		SendError(ctx, fasthttp.StatusTooManyRequests, "Too many registration requests from this network. Please try again later.")
		return
	}
	// Public registration always assigns the default "user" role.
	// Elevated/Admin privileges must be explicitly granted by an existing administrator.
	payload.Role = "user"

	hashedPassword, err := encrypt.Hash(payload.Password)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to hash password")
		return
	}

	// With SMTP on, prove email ownership before the request reaches the admin queue.
	verifyEmail := smtpEnabled(h.configStore, ctx)
	initialStatus := tables.UserStatusPending
	if verifyEmail {
		initialStatus = tables.UserStatusEmailUnverified
	}

	now := time.Now()
	if existing, err := h.configStore.GetUserByUsername(ctx, payload.Username); err == nil && existing != nil {
		if existing.IsApproved() || existing.Status == tables.UserStatusPending {
			SendError(ctx, fasthttp.StatusConflict, alreadyRegisteredMessage(existing))
			return
		}
		// Denied or never-verified users may request access again.
		if !h.assertEmailAvailable(ctx, payload.Email, existing.ID) {
			return
		}
		existing.Email = payload.Email
		existing.Password = hashedPassword
		existing.Role = "user"
		existing.Status = initialStatus
		existing.ReviewedAt = nil
		existing.UpdatedAt = now
		if err := h.configStore.UpdateUser(ctx, existing); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to submit registration")
			return
		}
		h.respondRegistered(ctx, existing, verifyEmail)
		return
	}

	if !h.assertEmailAvailable(ctx, payload.Email, "") {
		return
	}

	user := &tables.TableUser{
		ID:        uuid.New().String(),
		Username:  payload.Username,
		Email:     payload.Email,
		Password:  hashedPassword,
		Role:      payload.Role,
		Status:    initialStatus,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.configStore.CreateUser(ctx, user); err != nil {
		logger.Error("failed to register user username=%s: %v", payload.Username, err)
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "duplicate") || strings.Contains(errLower, "unique") {
			SendError(ctx, fasthttp.StatusConflict, "Username is already registered")
			return
		}
		if strings.Contains(errLower, "column") && (strings.Contains(errLower, "status") || strings.Contains(errLower, "email") || strings.Contains(errLower, "reviewed_at")) {
			SendError(ctx, fasthttp.StatusInternalServerError, "Database is missing user registration columns; restart the server to apply migrations")
			return
		}
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to submit registration")
		return
	}

	h.respondRegistered(ctx, user, verifyEmail)
}

// approveUser handles POST /api/session/users/{id}/approve
func (h *SessionHandler) approveUser(ctx *fasthttp.RequestCtx) {
	if !h.isAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	id, ok := h.extractParam(ctx, "id")
	if !ok {
		return
	}
	user, err := h.configStore.GetUserByID(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "User not found")
		return
	}
	now := time.Now()
	user.Status = tables.UserStatusApproved
	user.ReviewedAt = &now
	user.UpdatedAt = now
	if err := h.configStore.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to approve user: "+err.Error())
		return
	}
	emailSent, emailErr := trySendRegistrationDecisionEmail(h.configStore, ctx, user.Username, user.Email, "approved")
	user.Password = ""
	SendJSON(ctx, map[string]any{
		"id":          user.ID,
		"username":    user.Username,
		"email":       user.Email,
		"role":        user.Role,
		"status":      user.Status,
		"reviewed_at": user.ReviewedAt,
		"updated_at":  user.UpdatedAt,
		"email_sent":  emailSent,
		"email_error": emailErr,
	})
}

// rejectUser handles POST /api/session/users/{id}/reject
func (h *SessionHandler) rejectUser(ctx *fasthttp.RequestCtx) {
	if !h.isAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Forbidden")
		return
	}
	id, ok := h.extractParam(ctx, "id")
	if !ok {
		return
	}
	user, err := h.configStore.GetUserByID(ctx, id)
	if err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "User not found")
		return
	}
	now := time.Now()
	user.Status = tables.UserStatusRejected
	user.ReviewedAt = &now
	user.UpdatedAt = now
	if err := h.configStore.UpdateUser(ctx, user); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to reject user: "+err.Error())
		return
	}
	// Terminate any live sessions for the rejected user
	_ = h.configStore.DeleteSessionsByUsername(ctx, user.Username)
	emailSent, emailErr := trySendRegistrationDecisionEmail(h.configStore, ctx, user.Username, user.Email, "rejected")
	SendJSON(ctx, map[string]any{
		"message":     "Registration denied",
		"id":          user.ID,
		"status":      user.Status,
		"email_sent":  emailSent,
		"email_error": emailErr,
	})
}
