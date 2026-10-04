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
	"github.com/raksha/raksha/core/schemas"
	"github.com/raksha/raksha/framework/configstore"
	"github.com/raksha/raksha/framework/configstore/tables"
	"github.com/raksha/raksha/framework/encrypt"
	"github.com/raksha/raksha/transports/raksha-http/lib"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
)

// SessionHandler manages HTTP requests for session operations
type SessionHandler struct {
	configStore     configstore.ConfigStore
	wsTicketStore   *WSTicketStore
	userGovernance  UserGovernanceSyncer
	promptLifecycle *PromptLifecycleManager
}

// NewSessionHandler creates a new session handler instance.
// Optional userGovernance syncs Users.Budget into the live governance meter.
func NewSessionHandler(configStore configstore.ConfigStore, wsTicketStore *WSTicketStore, userGovernance ...UserGovernanceSyncer) *SessionHandler {
	h := &SessionHandler{
		configStore:     configStore,
		wsTicketStore:   wsTicketStore,
		promptLifecycle: NewPromptLifecycleManager(configStore),
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
			if guardSecret := strings.TrimSpace(os.Getenv("RAKSHA_GUARD_SECRET")); len(guardSecret) >= 16 {
				secret = guardSecret + "-password-reset-token-fallback-key-32b"
			} else if encKey := strings.TrimSpace(os.Getenv("RAKSHA_ENCRYPTION_KEY")); len(encKey) >= 16 {
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
func (h *SessionHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.RakshaHTTPMiddleware) {
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
	email := ""
	userID := ""
	allowedSections := ""
	userBudget := 0.0
	budgetUsage := 0.0
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
						allowedSections = effectiveAllowedSections(ctx, h.configStore, dbUser)
						email = dbUser.Email
						userID = dbUser.ID
						userBudget = dbUser.Budget
						if dbUser.BudgetID != nil && *dbUser.BudgetID != "" {
							if h.userGovernance != nil {
								if usage, ok := h.userGovernance.GetBudgetUsage(ctx, *dbUser.BudgetID); ok {
									budgetUsage = usage
								}
							}
							if budgetUsage == 0 {
								if b, err := h.configStore.GetBudget(ctx, *dbUser.BudgetID); err == nil && b != nil {
									budgetUsage = b.CurrentUsage
								}
							}
						}
					}
				}
			}
		}
	}
	SendJSON(ctx, map[string]any{
		"is_auth_enabled":      authConfig.IsEnabled,
		"has_valid_token":      hasValidToken,
		"auth_type":            dashboardAuthType(authConfig.IsEnabled),
		"role":                 role,
		"username":             username,
		"email":                email,
		"user_id":              userID,
		"allowed_sections":     allowedSections,
		"budget":               userBudget,
		"budget_current_usage": budgetUsage,
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
	return adminCredentialsFromStore(ctx, h.configStore)
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

	adminName, adminEmail := h.getAdminCredentials(ctx)
	isBuiltinAdmin := isMatchingAdminIdentity(payload.Username, adminName, adminEmail)
	// The built-in admin signs in as "admin", its username or its email; all of them
	// must share one failure counter or each alias would grant a fresh set of guesses.
	lockoutKey := payload.Username
	if isBuiltinAdmin {
		lockoutKey = adminName
	}

	if locked, retryAfter, err := checkLoginLockout(h.configStore, ctx, lockoutKey); err != nil {
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
	var activeDBUser *tables.TableUser

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
			if locked, retryAfter := recordLoginFailure(h.configStore, ctx, lockoutKey); locked {
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
				if locked, retryAfter := recordLoginFailure(h.configStore, ctx, lockoutKey); locked {
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
				case tables.UserStatusDisabled:
					SendError(ctx, fasthttp.StatusForbidden, "Your account is disabled. Contact your administrator.")
				case tables.UserStatusEmailUnverified:
					SendError(ctx, fasthttp.StatusForbidden, "Verify your email first — enter the code we emailed you on the Sign Up page")
				default:
					SendError(ctx, fasthttp.StatusUnauthorized, "Invalid username or password")
				}
				return
			}
			sessionRole = dbUser.Role
			notifyEmail = dbUser.Email
			sessionAllowedSections = effectiveAllowedSections(ctx, h.configStore, dbUser)
			authenticated = true
			activeDBUser = dbUser
		} else {
			padPasswordCompare(payload.Password)
			h.recordAuthAudit(ctx, "login", "failure", payload.Username, "/api/session/login", start)
			if blocked, retryAfter := recordLoginIPFailure(h.configStore, ctx); blocked {
				mins := lockoutRetryMinutes(ctx, retryAfter)
				SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many login attempts from this network. Try again in about %d minutes", mins))
				return
			}
			if locked, retryAfter := recordLoginFailure(h.configStore, ctx, lockoutKey); locked {
				mins := lockoutRetryMinutes(ctx, retryAfter)
				SendError(ctx, fasthttp.StatusTooManyRequests, fmt.Sprintf("Too many failed login attempts. Account locked for about %d minutes", mins))
				return
			}
			SendError(ctx, fasthttp.StatusUnauthorized, "Invalid username or password")
			return
		}
	}

	// The per-IP counter is deliberately left alone: if a success reset it, one valid
	// account would let an attacker spray passwords at every other account indefinitely.
	clearLoginFailures(h.configStore, ctx, lockoutKey)

	// Creating a new session with secure lifetime (24 hours default, configurable via SESSION_LIFETIME_HOURS)
	sessionDuration := time.Hour * 24
	if rawHours := strings.TrimSpace(os.Getenv("SESSION_LIFETIME_HOURS")); rawHours != "" {
		if h, err := strconv.Atoi(rawHours); err == nil && h > 0 {
			sessionDuration = time.Duration(h) * time.Hour
		}
	}
	sessionExpiresAt := time.Now().Add(sessionDuration)
	// NOTE: We intentionally do NOT wipe prior sessions on new login.
	// Deleting all sessions on every login caused automatic logouts when the same
	// user had multiple browser tabs or devices open simultaneously — the new login
	// would silently invalidate the other sessions, and those tabs would be redirected
	// to /login on the next API call.
	// Sessions are still revoked on password change/reset (CWE-613 still covered) and
	// they expire naturally after SESSION_LIFETIME_HOURS (default 24 h).
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

	if activeDBUser != nil && h.promptLifecycle != nil {
		h.promptLifecycle.OnUserLogin(ctx, activeDBUser)
	}

	resp := map[string]any{
		"message":          "Login successful",
		"role":             sessionRole,
		"allowed_sections": sessionAllowedSections,
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
	sessionToken, ok := ctx.UserValue(schemas.RakshaContextKeySessionToken).(string)
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
	// An abandoned sign-up whose code has expired must not hold the email forever.
	if other.Status == tables.UserStatusEmailUnverified && time.Since(other.UpdatedAt) > passwordResetOTPTTL {
		if err := h.configStore.DeleteUser(ctx, other.ID); err == nil {
			return true
		}
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
			var currentUsage float64
			found := false
			if h.userGovernance != nil {
				if usage, ok := h.userGovernance.GetBudgetUsage(ctx, *u.BudgetID); ok {
					currentUsage = usage
					found = true
				}
			}
			if !found {
				if b, err := h.configStore.GetBudget(ctx, *u.BudgetID); err == nil && b != nil {
					currentUsage = b.CurrentUsage
				}
			}
			item["budget_current_usage"] = currentUsage
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
		AutoCreatePrompt   *bool   `json:"auto_create_prompt"`
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
	if isBuiltinAdminIdentity(ctx, h.configStore, payload.Username) {
		SendError(ctx, fasthttp.StatusBadRequest, "Username matches the built-in admin account. Use a different username.")
		return
	}
	if isBuiltinAdminIdentity(ctx, h.configStore, payload.Email) {
		SendError(ctx, fasthttp.StatusConflict, "This email belongs to the built-in admin account. Use a different email.")
		return
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
	if !h.callerMayManageRole(ctx, role) {
		SendError(ctx, fasthttp.StatusForbidden, roleBeyondCallerMessage)
		return
	}
	if msg := scopedAdminTargetGuard(ctx, h.configStore, "", role); msg != "" {
		SendError(ctx, fasthttp.StatusForbidden, msg)
		return
	}
	if !callerSectionsCover(ctx, h.configStore, payload.AllowedSections) {
		SendError(ctx, fasthttp.StatusForbidden, "You can only grant sections you have access to yourself")
		return
	}
	payload.Role = role

	hashedPassword, err := encrypt.Hash(payload.Password)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to hash password")
		return
	}

	now := time.Now()
	autoCreatePrompt := false
	if payload.AutoCreatePrompt != nil {
		autoCreatePrompt = *payload.AutoCreatePrompt
	}

	existing, err := h.configStore.GetUserByUsername(ctx, payload.Username)
	if err != nil && !errors.Is(err, configstore.ErrNotFound) {
		logger.Error("failed to lookup governance user username=%s: %v", payload.Username, err)
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to validate username — check database connection")
		return
	}
	if usernameCaseConflict(ctx, h.configStore, payload.Username, "") {
		SendError(ctx, fasthttp.StatusConflict, "Username is already registered (usernames are not case-sensitive)")
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
		if payload.AutoCreatePrompt != nil {
			setPromptAutoCreatePreference(ctx, h.configStore, existing.ID, autoCreatePrompt)
		}
		if h.promptLifecycle != nil {
			_ = h.promptLifecycle.OnUserCreated(ctx, existing, autoCreatePrompt)
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

	if payload.AutoCreatePrompt != nil {
		setPromptAutoCreatePreference(ctx, h.configStore, user.ID, autoCreatePrompt)
	}
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnUserCreated(ctx, user, autoCreatePrompt)
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
		"Hello %s,\n\nYour Raksha password reset code is: %s\n\nIt expires in %d minutes. If you did not request this, ignore this email.\n",
		targetUsername, otp, int(passwordResetOTPTTL.Minutes()),
	)
	if err := sendAuthEmail(h.configStore, ctx, targetEmail, "Raksha password reset code", body); err != nil {
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
		SendError(ctx, fasthttp.StatusTooManyRequests, "Too many verification attempts. Please try again later.")
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
				"Hello,\n\nYour Raksha administrator username is: %s\n(You can also sign in with username 'admin' or your email: %s)\n\nIf you did not request this, please ignore this email.\n",
				adminName, adminEmail,
			)
			if err := sendAuthEmail(h.configStore, ctx, adminEmail, "Your Raksha Username", body); err != nil {
				logger.Warn("forgot username email failed for admin email=%s: %v", adminEmail, err)
			}
		}
		SendJSON(ctx, generic)
		return
	}

	body := fmt.Sprintf(
		"Hello,\n\nYour Raksha username associated with this email address is: %s\n\nIf you did not request this, please ignore this email.\n",
		user.Username,
	)
	if err := sendAuthEmail(h.configStore, ctx, user.Email, "Your Raksha Username", body); err != nil {
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

	// Every token/account failure returns the same message, and the "same as current
	// password" check only runs after the token is proven valid, so this unauthenticated
	// endpoint reveals neither which accounts exist nor whether a guessed password is right.
	const invalidResetMsg = "Invalid or expired reset token. Please request a new verification code."
	claims, err := verifyPasswordResetToken(payload.ResetToken)
	if err != nil {
		SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
		return
	}
	otpRow, err := h.configStore.GetPasswordResetOTPByID(ctx, claims.OTPID)
	if err != nil || otpRow == nil || otpRow.Used || time.Now().After(otpRow.ExpiresAt) {
		SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
		return
	}

	if user != nil {
		if !user.IsApproved() {
			SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
			return
		}
		if !strings.EqualFold(claims.Username, user.Username) && (claims.Email == "" || !strings.EqualFold(claims.Email, user.Email)) {
			SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
			return
		}
		if !strings.EqualFold(otpRow.Username, user.Username) {
			SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
			return
		}
		sameAsCurrent, cmpErr := encrypt.CompareHash(user.Password, payload.NewPassword)
		if cmpErr == nil && sameAsCurrent {
			SendError(ctx, fasthttp.StatusBadRequest, "New password must be different from your current password")
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
		// The emailed code proves ownership, so the failed-login lock (which only guards
		// the old password) is lifted. The per-IP limit still applies.
		clearAccountLockouts(h.configStore, ctx, user.Username, user.Email)
		SendJSON(ctx, map[string]any{"message": "Password updated. You can sign in now."})
		return
	}

	// Super Admin flow
	adminName, adminEmail := h.getAdminCredentials(ctx)
	isSuperAdmin := isMatchingAdminIdentity(payload.Username, adminName, adminEmail) || isMatchingAdminIdentity(payload.Email, adminName, adminEmail)
	if !isSuperAdmin {
		SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
		return
	}
	if !isMatchingAdminIdentity(claims.Username, adminName, adminEmail) &&
		(claims.Email == "" || !isMatchingAdminIdentity(claims.Email, adminName, adminEmail)) {
		SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
		return
	}
	if !isMatchingAdminIdentity(otpRow.Username, adminName, adminEmail) &&
		(otpRow.Email == "" || !isMatchingAdminIdentity(otpRow.Email, adminName, adminEmail)) {
		SendError(ctx, fasthttp.StatusUnauthorized, invalidResetMsg)
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
	if !strings.EqualFold(adminName, "admin") {
		_ = h.configStore.DeleteSessionsByUsername(ctx, "admin")
	}

	clearAccountLockouts(h.configStore, ctx, adminName, adminEmail, "admin")
	SendJSON(ctx, map[string]any{"message": "Password updated. You can sign in now."})
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
	if !h.callerMayManageRole(ctx, existingUser.Role) {
		SendError(ctx, fasthttp.StatusForbidden, roleBeyondCallerMessage)
		return
	}

	prevRole := existingUser.Role
	prevUsername := existingUser.Username
	prevEmail := existingUser.Email
	roleChanged := false

	var payload struct {
		Username           string   `json:"username"`
		Password           string   `json:"password"`
		Role               string   `json:"role"`
		Email              *string  `json:"email"`
		Status             *string  `json:"status"`
		Budget             *float64 `json:"budget"`
		RateLimit          *int     `json:"rate_limit"`
		AllowedPromptRepos *string  `json:"allowed_prompt_repos"`
		AllowedSections    *string  `json:"allowed_sections"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if msg := scopedAdminTargetGuard(ctx, h.configStore, existingUser.Username, existingUser.Role); msg != "" {
		SendError(ctx, fasthttp.StatusForbidden, msg)
		return
	}
	if session, scoped := callerIsScopedAdmin(ctx, h.configStore); scoped && strings.EqualFold(session.Username, existingUser.Username) {
		changesOwnGrants := (payload.AllowedSections != nil && *payload.AllowedSections != existingUser.AllowedSections) ||
			(payload.Budget != nil && *payload.Budget != existingUser.Budget) ||
			(payload.RateLimit != nil && *payload.RateLimit != existingUser.RateLimit) ||
			(payload.Role != "" && !strings.EqualFold(payload.Role, existingUser.Role)) ||
			(payload.Status != nil && *payload.Status != existingUser.Status)
		if changesOwnGrants {
			SendError(ctx, fasthttp.StatusForbidden, "You cannot change your own role, status, sections, budget or rate limit")
			return
		}
	}
	if payload.AllowedSections != nil && *payload.AllowedSections != existingUser.AllowedSections &&
		!callerSectionsCover(ctx, h.configStore, *payload.AllowedSections) {
		SendError(ctx, fasthttp.StatusForbidden, "You can only grant sections you have access to yourself")
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
		if usernameCaseConflict(ctx, h.configStore, newName, existingUser.ID) {
			SendError(ctx, fasthttp.StatusConflict, "Username is already taken (usernames are not case-sensitive)")
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
		if !h.callerMayManageRole(ctx, role) {
			SendError(ctx, fasthttp.StatusForbidden, roleBeyondCallerMessage)
			return
		}
		if msg := scopedAdminTargetGuard(ctx, h.configStore, existingUser.Username, role); msg != "" && role != prevRole {
			SendError(ctx, fasthttp.StatusForbidden, msg)
			return
		}
		existingUser.Role = role
		roleChanged = role != prevRole
	}
	// Partial updates (e.g. Roles & Permissions only sends role/allowed_sections)
	// must not reset budget or rate limit to zero.
	if payload.Budget != nil {
		existingUser.Budget = *payload.Budget
	}
	if payload.RateLimit != nil {
		existingUser.RateLimit = *payload.RateLimit
	}
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
	if payload.Password != "" || (payload.Status != nil && (*payload.Status == tables.UserStatusRejected || *payload.Status == tables.UserStatusPending)) {
		_ = h.configStore.DeleteSessionsByUsername(ctx, existingUser.Username)
		if prevUsername != "" && prevUsername != existingUser.Username {
			_ = h.configStore.DeleteSessionsByUsername(ctx, prevUsername)
		}
		if payload.Password != "" {
			clearAccountLockouts(h.configStore, ctx, existingUser.Username, existingUser.Email)
		}
	} else if roleChanged {
		// Keep user signed in but refresh RBAC immediately on all live sessions.
		_ = h.configStore.UpdateSessionsRoleByUsername(ctx, existingUser.Username, existingUser.Role)
		if prevUsername != "" && prevUsername != existingUser.Username {
			_ = h.configStore.UpdateSessionsRoleByUsername(ctx, prevUsername, existingUser.Role)
		}
	}

	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnUserUpdated(ctx, existingUser, prevEmail, prevUsername)
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
	if !h.callerMayManageRole(ctx, existing.Role) {
		SendError(ctx, fasthttp.StatusForbidden, roleBeyondCallerMessage)
		return
	}
	if msg := scopedAdminTargetGuard(ctx, h.configStore, existing.Username, existing.Role); msg != "" {
		SendError(ctx, fasthttp.StatusForbidden, msg)
		return
	}

	// Clean related governance rows before deleting the user so DB stays consistent.
	purgeUserRelations(ctx, h.configStore, h.promptLifecycle, existing)

	if err := h.configStore.DeleteUser(ctx, id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to delete user: "+err.Error())
		return
	}
	// Terminate any live sessions for the deleted user
	_ = h.configStore.DeleteSessionsByUsername(ctx, existing.Username)
	if h.userGovernance != nil {
		h.userGovernance.DeleteUserGovernance(ctx, id)
		if evicter, ok := h.userGovernance.(userModelConfigEvicter); ok {
			evicter.DeleteUserModelConfigs(ctx, id)
		}
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

	// The built-in admin's identifiers ("admin", its username and its email) are reserved:
	// a shadow account could otherwise capture the admin's forgot-password / forgot-username mail.
	adminName, adminEmail := h.getAdminCredentials(ctx)
	if isMatchingAdminIdentity(payload.Username, adminName, adminEmail) {
		SendError(ctx, fasthttp.StatusConflict, "Username is unavailable. Try a different username.")
		return
	}
	if isMatchingAdminIdentity(payload.Email, adminName, adminEmail) {
		SendError(ctx, fasthttp.StatusConflict, "This email is already registered with another account")
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
	if usernameCaseConflict(ctx, h.configStore, payload.Username, "") {
		SendError(ctx, fasthttp.StatusConflict, "This username is already registered. Please choose a different username.")
		return
	}
	if existing, err := h.configStore.GetUserByUsername(ctx, payload.Username); err == nil && existing != nil {
		if existing.IsApproved() || existing.Status == tables.UserStatusPending {
			SendError(ctx, fasthttp.StatusConflict, alreadyRegisteredMessage(existing))
			return
		}
		// An unverified sign-up stays reserved while its emailed code is still valid.
		// Overwriting it (even with the same email) would swap in a new password that the
		// real owner then unknowingly activates by entering their code.
		if existing.Status == tables.UserStatusEmailUnverified {
			if wait := time.Until(existing.UpdatedAt.Add(passwordResetOTPTTL)); wait > 0 {
				// The owner resubmitting (page refresh, "Edit details") proves it with the same
				// password: keep the hold and email a fresh code instead of a dead-end 409.
				if ok, cmpErr := encrypt.CompareHash(existing.Password, payload.Password); cmpErr == nil && ok {
					if !strings.EqualFold(existing.Email, payload.Email) {
						if !h.assertEmailAvailable(ctx, payload.Email, existing.ID) {
							return
						}
						existing.Email = payload.Email
					}
					existing.Status = initialStatus
					existing.UpdatedAt = now
					if err := h.configStore.UpdateUser(ctx, existing); err != nil {
						SendError(ctx, fasthttp.StatusInternalServerError, "Failed to submit registration")
						return
					}
					h.respondRegistered(ctx, existing, verifyEmail, false)
					return
				}
				SendError(ctx, fasthttp.StatusConflict, fmt.Sprintf(
					"This username has a sign-up waiting for email verification. Enter the emailed code (or use Resend Code), or try again in about %d minutes.",
					retryMinutes(wait)))
				return
			}
		}
		// Denied or never-verified users may request access again — but never over an
		// account the identity provider manages or one that still owns teams/keys, or the
		// new sign-up would inherit a real employee's access once it is re-activated.
		if !h.registrationMayReuse(ctx, existing) {
			SendError(ctx, fasthttp.StatusConflict, "Username is already registered")
			return
		}
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
		h.respondRegistered(ctx, existing, verifyEmail, true)
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

	h.respondRegistered(ctx, user, verifyEmail, true)
}

// registrationMayReuse reports whether a public sign-up may overwrite an existing row.
func (h *SessionHandler) registrationMayReuse(ctx *fasthttp.RequestCtx, existing *tables.TableUser) bool {
	if existing.Status != tables.UserStatusRejected && existing.Status != tables.UserStatusEmailUnverified {
		return false
	}
	if strings.TrimSpace(existing.ExternalID) != "" {
		return false
	}
	if r := strings.ToLower(strings.TrimSpace(existing.Role)); r != "" && r != "user" {
		return false
	}
	if ws, ok := configstore.AsWorkspaceStore(h.configStore); ok && ws != nil {
		if teams, err := ws.ListTeamsForUser(ctx, existing.ID); err != nil || len(teams) > 0 {
			return false
		}
		if vks, err := ws.ListVirtualKeysForUser(ctx, existing.ID); err != nil || len(vks) > 0 {
			return false
		}
	}
	return true
}

// reviewTargetGuard applies the same role guards as edit/delete to approve/reject and
// limits both to sign-ups still awaiting review. Returns false after sending an error.
func (h *SessionHandler) reviewTargetGuard(ctx *fasthttp.RequestCtx, user *tables.TableUser) bool {
	if user.Status != tables.UserStatusPending && user.Status != tables.UserStatusEmailUnverified {
		SendError(ctx, fasthttp.StatusConflict, "Only pending sign-ups can be approved or denied. Use Edit to change an existing account.")
		return false
	}
	if user.Role == "admin" && !h.isSuperAdmin(ctx) {
		SendError(ctx, fasthttp.StatusForbidden, "Cannot modify an admin account")
		return false
	}
	if !h.callerMayManageRole(ctx, user.Role) {
		SendError(ctx, fasthttp.StatusForbidden, roleBeyondCallerMessage)
		return false
	}
	if msg := scopedAdminTargetGuard(ctx, h.configStore, user.Username, user.Role); msg != "" {
		SendError(ctx, fasthttp.StatusForbidden, msg)
		return false
	}
	return true
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
	if user.Status == tables.UserStatusDisabled {
		SendError(ctx, fasthttp.StatusConflict, "This account was disabled by the identity provider. Re-activate it there.")
		return
	}
	if !h.reviewTargetGuard(ctx, user) {
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
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnUserCreated(ctx, user, true)
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
	if !h.reviewTargetGuard(ctx, user) {
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
