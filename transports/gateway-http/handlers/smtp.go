package handlers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/encrypt"
	"github.com/gateway/gateway/framework/mailer"
	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/valyala/fasthttp"
)

const (
	loginMaxFailedAttempts = 3
	loginLockoutDuration   = 20 * time.Minute
	passwordResetOTPTTL    = 15 * time.Minute
)

// SMTPHandler manages SMTP settings used for auth emails.
type SMTPHandler struct {
	store *lib.Config
}

func NewSMTPHandler(store *lib.Config) *SMTPHandler {
	return &SMTPHandler{store: store}
}

func (h *SMTPHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.GatewayHTTPMiddleware) {
	r.GET("/api/smtp-config", lib.ChainMiddlewares(h.getSMTPConfig, middlewares...))
	r.PUT("/api/smtp-config", lib.ChainMiddlewares(h.updateSMTPConfig, middlewares...))
	r.POST("/api/smtp-config/test", lib.ChainMiddlewares(h.testSMTPConfig, middlewares...))
}

type smtpConfigPayload struct {
	Enabled             bool   `json:"enabled"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	FromEmail           string `json:"from_email"`
	FromName            string `json:"from_name"`
	UseTLS              bool   `json:"use_tls"`
	NotifyOnLogin       bool   `json:"notify_on_login"`
	NotifyOnUserCreate  bool   `json:"notify_on_user_create"`
	EmailProvider       string `json:"email_provider"` // "smtp" or "outlook"
	OutlookEnabled      bool   `json:"outlook_enabled"`
	OutlookClientID     string `json:"outlook_client_id"`
	OutlookClientSecret string `json:"outlook_client_secret"`
	OutlookTenantID     string `json:"outlook_tenant_id"`
	OutlookEmail        string `json:"outlook_email"`
}

func (h *SMTPHandler) requireStore(ctx *fasthttp.RequestCtx) configstore.ConfigStore {
	if h.store == nil || h.store.ConfigStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store not available")
		return nil
	}
	return h.store.ConfigStore
}

func (h *SMTPHandler) getSMTPConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetSMTPConfig(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	envClientID := strings.TrimSpace(os.Getenv("OUTLOOK_CLIENT_ID"))
	envTenantID := strings.TrimSpace(os.Getenv("OUTLOOK_TENANT_ID"))
	envEmail := strings.TrimSpace(os.Getenv("OUTLOOK_EMAIL"))
	envSecret := strings.TrimSpace(os.Getenv("OUTLOOK_CLIENT_SECRET"))
	if envSecret == "" {
		envSecret = strings.TrimSpace(os.Getenv("OUTLOOK_SECRET_ID"))
	}

	if row == nil {
		secretPlaceholder := ""
		if envSecret != "" {
			secretPlaceholder = "<redacted>"
		}
		SendJSON(ctx, smtpConfigPayload{
			Port:                587,
			UseTLS:              true,
			NotifyOnUserCreate:  true,
			EmailProvider:       "smtp",
			OutlookClientID:     envClientID,
			OutlookClientSecret: secretPlaceholder,
			OutlookTenantID:     envTenantID,
			OutlookEmail:        envEmail,
		})
		return
	}

	clientID := row.OutlookClientID
	if clientID == "" {
		clientID = envClientID
	}
	tenantID := row.OutlookTenantID
	if tenantID == "" {
		tenantID = envTenantID
	}
	outlookEmail := row.OutlookEmail
	if outlookEmail == "" {
		outlookEmail = envEmail
	}
	secretPlaceholder := ""
	if row.OutlookClientSecret != "" || envSecret != "" {
		secretPlaceholder = "<redacted>"
	}
	provider := row.EmailProvider
	if provider == "" {
		if row.OutlookEnabled {
			provider = "outlook"
		} else {
			provider = "smtp"
		}
	}

	SendJSON(ctx, smtpConfigPayload{
		Enabled:             row.Enabled,
		Host:                row.Host,
		Port:                row.Port,
		Username:            row.Username,
		Password:            "<redacted>",
		FromEmail:           row.FromEmail,
		FromName:            row.FromName,
		UseTLS:              row.UseTLS,
		NotifyOnLogin:       row.NotifyOnLogin,
		NotifyOnUserCreate:  row.NotifyOnUserCreate,
		EmailProvider:       provider,
		OutlookEnabled:      row.OutlookEnabled,
		OutlookClientID:     clientID,
		OutlookClientSecret: secretPlaceholder,
		OutlookTenantID:     tenantID,
		OutlookEmail:        outlookEmail,
	})
}

func (h *SMTPHandler) updateSMTPConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload smtpConfigPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid payload")
		return
	}

	// Mutual exclusivity: enabling one disables the other
	if payload.OutlookEnabled {
		payload.Enabled = false
		payload.EmailProvider = "outlook"
	} else if payload.Enabled {
		payload.OutlookEnabled = false
		payload.EmailProvider = "smtp"
	} else if payload.EmailProvider == "outlook" {
		payload.OutlookEnabled = true
		payload.Enabled = false
	} else if payload.EmailProvider == "smtp" {
		payload.Enabled = true
		payload.OutlookEnabled = false
	}

	existing, err := store.GetSMTPConfig(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	envSecret := strings.TrimSpace(os.Getenv("OUTLOOK_CLIENT_SECRET"))
	if envSecret == "" {
		envSecret = strings.TrimSpace(os.Getenv("OUTLOOK_SECRET_ID"))
	}
	outlookSecret := payload.OutlookClientSecret
	if outlookSecret == "" || strings.EqualFold(outlookSecret, "<redacted>") || strings.Contains(outlookSecret, "*") {
		if existing != nil && existing.OutlookClientSecret != "" {
			outlookSecret = existing.OutlookClientSecret
		} else {
			outlookSecret = envSecret
		}
	}

	if payload.OutlookEnabled {
		cid := strings.TrimSpace(payload.OutlookClientID)
		if cid == "" {
			cid = strings.TrimSpace(os.Getenv("OUTLOOK_CLIENT_ID"))
		}
		if cid == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "Outlook Client ID is required")
			return
		}
		tid := strings.TrimSpace(payload.OutlookTenantID)
		if tid == "" {
			tid = strings.TrimSpace(os.Getenv("OUTLOOK_TENANT_ID"))
		}
		if tid == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "Outlook Tenant ID is required")
			return
		}
		oEmail := strings.TrimSpace(payload.OutlookEmail)
		if oEmail == "" {
			oEmail = strings.TrimSpace(os.Getenv("OUTLOOK_EMAIL"))
		}
		if oEmail == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "Outlook From email is required")
			return
		}
		if strings.TrimSpace(outlookSecret) == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "Outlook Client Secret (secret ID) is required when Outlook is enabled")
			return
		}
	} else if payload.Enabled {
		if strings.TrimSpace(payload.Host) == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "SMTP host is required")
			return
		}
		if strings.TrimSpace(payload.FromEmail) == "" && strings.TrimSpace(payload.Username) == "" {
			SendError(ctx, fasthttp.StatusBadRequest, "From email or SMTP username is required")
			return
		}
	}

	if payload.Port <= 0 {
		payload.Port = 587
	}

	password := payload.Password
	if password == "" || strings.EqualFold(password, "<redacted>") || strings.Contains(password, "*") {
		if existing != nil {
			password = existing.Password
		} else {
			password = ""
		}
	}
	if payload.Enabled && strings.TrimSpace(password) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "SMTP password is required when SMTP is enabled")
		return
	}
	if strings.TrimSpace(password) != "" && !encrypt.IsEnabled() {
		SendError(ctx, fasthttp.StatusBadRequest, "Set GATEWAY_ENCRYPTION_KEY before saving SMTP passwords so credentials are encrypted at rest")
		return
	}

	row := &tables.TableSMTPConfig{
		Enabled:             payload.Enabled,
		Host:                strings.TrimSpace(payload.Host),
		Port:                payload.Port,
		Username:            strings.TrimSpace(payload.Username),
		Password:            password,
		FromEmail:           strings.TrimSpace(payload.FromEmail),
		FromName:            strings.TrimSpace(payload.FromName),
		UseTLS:              payload.UseTLS,
		NotifyOnLogin:       payload.NotifyOnLogin,
		NotifyOnUserCreate:  payload.NotifyOnUserCreate,
		EmailProvider:       payload.EmailProvider,
		OutlookEnabled:      payload.OutlookEnabled,
		OutlookClientID:     strings.TrimSpace(payload.OutlookClientID),
		OutlookClientSecret: outlookSecret,
		OutlookTenantID:     strings.TrimSpace(payload.OutlookTenantID),
		OutlookEmail:        strings.TrimSpace(payload.OutlookEmail),
		EncryptionStatus:    tables.EncryptionStatusPlainText,
	}
	if err := store.UpdateSMTPConfig(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "ok", "provider": payload.EmailProvider})
}

func (h *SMTPHandler) testSMTPConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload struct {
		To string `json:"to"`
	}
	_ = json.Unmarshal(ctx.PostBody(), &payload)
	row, err := store.GetSMTPConfig(ctx)
	if err != nil || row == nil || (!row.Enabled && !row.OutlookEnabled) {
		SendError(ctx, fasthttp.StatusBadRequest, "enable and save email settings first (SMTP or Outlook)")
		return
	}
	to := strings.TrimSpace(payload.To)
	if to == "" {
		if row.OutlookEnabled || row.EmailProvider == "outlook" {
			to = row.OutlookEmail
			if to == "" {
				to = strings.TrimSpace(os.Getenv("OUTLOOK_EMAIL"))
			}
		} else {
			to = row.FromEmail
			if to == "" {
				to = row.Username
			}
		}
	}
	if to == "" {
		to = "test@example.com"
	}

	providerName := "SMTP"
	if row.OutlookEnabled || row.EmailProvider == "outlook" {
		providerName = "Microsoft Outlook"
	}
	subject := fmt.Sprintf("Gateway %s test", providerName)
	body := fmt.Sprintf("This is a test email from Gateway Security → %s settings.", providerName)

	err = sendConfiguredEmail(ctx, store, row, to, subject, body)
	recordEmailAudit(store, ctx, to, subject, err)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("%s test failed: %v", providerName, err))
		return
	}
	SendJSON(ctx, map[string]any{"status": "ok", "to": to, "provider": providerName})
}

func smtpToMailer(row *tables.TableSMTPConfig) mailer.Config {
	if row == nil {
		return mailer.Config{}
	}
	fromName := strings.TrimSpace(row.FromName)
	if fromName == "" {
		brand := strings.TrimSpace(os.Getenv("GATEWAY_BRAND_NAME"))
		if brand == "" {
			brand = "Raksha"
		}
		fromName = brand + " Security System"
	}
	return mailer.Config{
		Enabled:   row.Enabled,
		Host:      row.Host,
		Port:      row.Port,
		Username:  row.Username,
		Password:  row.Password,
		FromEmail: row.FromEmail,
		FromName:  fromName,
		UseTLS:    row.UseTLS,
	}
}

func rowToOutlook(row *tables.TableSMTPConfig) mailer.OutlookConfig {
	if row == nil {
		return mailer.OutlookConfig{}
	}
	fromName := strings.TrimSpace(row.FromName)
	if fromName == "" {
		brand := strings.TrimSpace(os.Getenv("GATEWAY_BRAND_NAME"))
		if brand == "" {
			brand = "Raksha"
		}
		fromName = brand + " Security System"
	}
	clientID := strings.TrimSpace(row.OutlookClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(os.Getenv("OUTLOOK_CLIENT_ID"))
	}
	clientSecret := strings.TrimSpace(row.OutlookClientSecret)
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(os.Getenv("OUTLOOK_CLIENT_SECRET"))
		if clientSecret == "" {
			clientSecret = strings.TrimSpace(os.Getenv("OUTLOOK_SECRET_ID"))
		}
	}
	tenantID := strings.TrimSpace(row.OutlookTenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(os.Getenv("OUTLOOK_TENANT_ID"))
	}
	email := strings.TrimSpace(row.OutlookEmail)
	if email == "" {
		email = strings.TrimSpace(os.Getenv("OUTLOOK_EMAIL"))
	}
	return mailer.OutlookConfig{
		Enabled:      row.OutlookEnabled || row.EmailProvider == "outlook",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TenantID:     tenantID,
		Email:        email,
		FromName:     fromName,
	}
}

func sendConfiguredEmail(ctx context.Context, store configstore.ConfigStore, row *tables.TableSMTPConfig, to, subject, body string) error {
	if row == nil {
		return fmt.Errorf("email configuration not found")
	}
	if row.OutlookEnabled || row.EmailProvider == "outlook" {
		return mailer.SendOutlookWithContext(ctx, rowToOutlook(row), mailer.Message{To: to, Subject: subject, Body: body})
	}
	if row.Enabled || row.EmailProvider == "smtp" {
		return authMailSend(smtpToMailer(row), mailer.Message{To: to, Subject: subject, Body: body})
	}
	return fmt.Errorf("email is not enabled; configure SMTP or Outlook in Settings → Security")
}

// authMailSend is swapped in tests to capture auth emails without an SMTP server.
var authMailSend = mailer.Send

func sendAuthEmail(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, to, subject, body string) error {
	err := sendSMTPEmail(ctx, store, to, subject, body)
	recordEmailAudit(store, ctx, to, subject, err)
	return err
}

// sendSMTPEmail sends through the email configuration (SMTP or Outlook) saved under Security settings
// without needing an HTTP request, so background dispatchers can use it.
func sendSMTPEmail(ctx context.Context, store configstore.ConfigStore, to, subject, body string) error {
	if store == nil || strings.TrimSpace(to) == "" {
		return fmt.Errorf("no recipient")
	}
	row, err := store.GetSMTPConfig(ctx)
	if err != nil {
		return err
	}
	if row == nil || (!row.Enabled && !row.OutlookEnabled) {
		return fmt.Errorf("email is not enabled; configure SMTP or Outlook in Settings → Security")
	}
	return sendConfiguredEmail(ctx, store, row, to, subject, body)
}

// recordEmailAudit writes one audit_logs row per outgoing email. The body is never
// stored: auth emails carry one-time codes.
func recordEmailAudit(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, to, subject string, sendErr error) {
	if store == nil || isAuditDisabled() {
		return
	}
	ws, _ := configstore.AsWorkspaceStore(store)
	if ws == nil {
		return
	}
	outcome, detail := "success", "subject: "+subject
	if sendErr != nil {
		outcome = "failure"
		detail += "\nerror: " + sendErr.Error()
	}
	_ = ws.CreateAuditLog(ctx, &tables.TableAuditLog{
		Action:    "email_sent",
		Outcome:   outcome,
		Initiator: auditInitiator(store, ctx),
		Target:    strings.TrimSpace(to),
		Method:    string(ctx.Method()),
		Path:      string(ctx.Path()),
		IP:        ctx.RemoteIP().String(),
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	})
}

func emailServiceConfigured(row *tables.TableSMTPConfig) bool {
	if row == nil {
		return false
	}
	if row.OutlookEnabled || row.EmailProvider == "outlook" {
		cid := strings.TrimSpace(row.OutlookClientID)
		if cid == "" {
			cid = strings.TrimSpace(os.Getenv("OUTLOOK_CLIENT_ID"))
		}
		return cid != ""
	}
	return row.Enabled && strings.TrimSpace(row.Host) != ""
}

func smtpEnabled(store configstore.ConfigStore, ctx *fasthttp.RequestCtx) bool {
	if store == nil {
		return false
	}
	row, err := store.GetSMTPConfig(ctx)
	return err == nil && emailServiceConfigured(row)
}

func welcomeAccountEmailBody(username, email, tempPassword string) string {
	var passInfo string
	if tempPassword != "" {
		passInfo = fmt.Sprintf("\nTemporary Password: %s\n\nImportant: You will be required to set a new permanent password upon your first sign in.\n", tempPassword)
	} else {
		passInfo = "\nSign in with the temporary password provided by your administrator. You will be required to set a new permanent password upon your first sign in.\n"
	}
	loginLine := "Sign in on the login page."
	if loginURL := publicLoginURL(); loginURL != "" && loginURL != "/login" {
		loginLine = "Sign in: " + loginURL
	}
	return fmt.Sprintf(
		"Hello %s,\n\nYour Gateway Enterprise account has been created.\n\n%s\n\nUsername: %s\nEmail: %s%s\nPlease keep your credentials confidential.\n",
		username, loginLine, username, email, passInfo,
	)
}

func publicLoginURL() string {
	domain := strings.TrimRight(strings.TrimSpace(os.Getenv("SERVER_DOMAIN")), "/")
	if domain == "" {
		domain = strings.TrimRight(strings.TrimSpace(os.Getenv("GATEWAY_UI_URL")), "/")
	}
	if domain == "" {
		domain = strings.TrimRight(strings.TrimSpace(os.Getenv("UI_URL")), "/")
	}
	if domain == "" {
		domain = strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_URL")), "/")
	}
	if domain == "" {
		return "/login"
	}
	if strings.HasSuffix(domain, "/login") {
		return domain
	}
	return domain + "/login"
}

func requestLoginURL(ctx *fasthttp.RequestCtx) string {
	domain := strings.TrimRight(strings.TrimSpace(os.Getenv("SERVER_DOMAIN")), "/")
	if domain != "" {
		if strings.HasSuffix(domain, "/login") {
			return domain
		}
		return domain + "/login"
	}
	if ctx != nil {
		// Prefer Origin header if present (standard CORS header sent by browsers on API requests)
		if origin := strings.TrimRight(strings.TrimSpace(string(ctx.Request.Header.Peek("Origin"))), "/"); origin != "" {
			return origin + "/login"
		}

		// Check Referer header if present
		if referer := strings.TrimSpace(string(ctx.Request.Header.Peek("Referer"))); referer != "" {
			if parsed, err := url.Parse(referer); err == nil && parsed.Host != "" {
				scheme := parsed.Scheme
				if scheme == "" {
					scheme = "http"
				}
				return scheme + "://" + parsed.Host + "/login"
			}
		}

		// Fallback to request Host header, dynamically replacing backend port with UI port from env
		host := string(ctx.Host())
		if host != "" {
			scheme := "http"
			if ctx.IsTLS() || string(ctx.Request.Header.Peek("X-Forwarded-Proto")) == "https" {
				scheme = "https"
			}
			appPort := strings.TrimSpace(os.Getenv("APP_PORT"))
			uiPort := strings.TrimSpace(os.Getenv("UI_PORT"))
			if appPort != "" && uiPort != "" && strings.Contains(host, ":"+appPort) {
				host = strings.Replace(host, ":"+appPort, ":"+uiPort, 1)
			}
			return scheme + "://" + host + "/login"
		}
	}
	return publicLoginURL()
}

func provisionedAccountEmailBody(username, email, tempPassword string) string {
	loginLine := "Sign in on the login page."
	if loginURL := publicLoginURL(); loginURL != "" && loginURL != "/login" {
		loginLine = "Sign in: " + loginURL
	}
	return fmt.Sprintf(
		"Hello %s,\n\nYour account has been created.\n\n%s\n\nUsername: %s\nEmail: %s\nTemporary Password: %s\n\nImportant: You will be required to set a new permanent password upon your first sign in.\n",
		username, loginLine, username, email, tempPassword,
	)
}

// trySendProvisionedEmail emails the temporary password and login link.
// SCIM user creation still succeeds when SMTP is off; the failure is only logged.
func trySendProvisionedEmail(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username, email, tempPassword string) {
	email = strings.TrimSpace(email)
	if email == "" || store == nil || strings.TrimSpace(tempPassword) == "" {
		return
	}
	smtpRow, err := store.GetSMTPConfig(ctx)
	if err != nil || !emailServiceConfigured(smtpRow) {
		if logger != nil {
			logger.Warn("scim welcome email skipped username=%s: email service is not enabled", username)
		}
		return
	}
	if err := sendAuthEmail(store, ctx, email, "Your account - login details", provisionedAccountEmailBody(username, email, tempPassword)); err != nil && logger != nil {
		logger.Warn("scim welcome email failed username=%s: %v", username, err)
	}
}

// temporaryPasswordMailReady reports why a generated password cannot be emailed yet.
func temporaryPasswordMailReady(store configstore.ConfigStore, ctx *fasthttp.RequestCtx) string {
	if store == nil {
		return "Email service is not configured. Turn it on under Security settings so the temporary password can be emailed."
	}
	smtpRow, err := store.GetSMTPConfig(ctx)
	if err != nil {
		return err.Error()
	}
	if !emailServiceConfigured(smtpRow) {
		return "Email service is not enabled. Turn on SMTP or Outlook under Security settings so the temporary password can be emailed."
	}
	return ""
}

// trySendTemporaryPasswordEmail always attempts delivery when SMTP is enabled.
// A generated password is useless if the mail never leaves the server.
func trySendTemporaryPasswordEmail(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username, email, password string) (bool, string) {
	email = strings.TrimSpace(email)
	if email == "" || store == nil {
		return false, "No email address — temporary password was not sent"
	}
	smtpRow, err := store.GetSMTPConfig(ctx)
	if err != nil {
		return false, err.Error()
	}
	if !emailServiceConfigured(smtpRow) {
		return false, "Email service is not enabled. Turn on SMTP or Outlook under Security settings so the temporary password can be emailed."
	}
	if err := sendAuthEmail(store, ctx, email, "Your Gateway Account - Login Credentials", welcomeAccountEmailBody(username, email, password)); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// trySendWelcomeEmail sends create-user mail when SMTP notify-on-create is on.
// Returns (sent, errorMessage). Missing email / disabled SMTP is not an error.
func trySendWelcomeEmail(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username, email, password string) (bool, string) {
	email = strings.TrimSpace(email)
	if email == "" || store == nil {
		return false, ""
	}
	smtpRow, err := store.GetSMTPConfig(ctx)
	if err != nil {
		return false, err.Error()
	}
	if !emailServiceConfigured(smtpRow) || !smtpRow.NotifyOnUserCreate {
		return false, ""
	}
	if err := sendAuthEmail(store, ctx, email, "Your Gateway Account - Login Credentials", welcomeAccountEmailBody(username, email, password)); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func accountApprovedEmailBody(username string) string {
	loginLine := "Sign in on the login page."
	if loginURL := publicLoginURL(); loginURL != "" && loginURL != "/login" {
		loginLine = "Sign in: " + loginURL
	}
	return fmt.Sprintf(
		"Hello %s,\n\nYour Gateway registration was approved by an administrator.\n\n%s\n\nYou can sign in now with the username and password you registered with.\n\nIf you forgot your password, use Forgot password on the login page.\n",
		username, loginLine,
	)
}

func accountRejectedEmailBody(username string) string {
	return fmt.Sprintf(
		"Hello %s,\n\nYour Gateway registration was reviewed and was not approved.\n\nYou will not be able to sign in with this account. Contact your administrator if you believe this is a mistake.\n",
		username,
	)
}

// trySendAccountBlockStatusEmail notifies the user when their account is blocked or unblocked.
func trySendAccountBlockStatusEmail(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username, email string, blocked bool) (bool, string) {
	email = strings.TrimSpace(email)
	if email == "" && strings.Contains(username, "@") {
		email = strings.TrimSpace(username)
	}
	if email == "" || store == nil {
		return false, "User email address is missing"
	}
	smtpRow, err := store.GetSMTPConfig(ctx)
	if err != nil {
		return false, err.Error()
	}
	if !emailServiceConfigured(smtpRow) {
		return false, "Email service is not enabled. Please configure SMTP or Outlook in Security settings to dispatch notification emails."
	}
	var subject, body string
	if blocked {
		subject = "Security Notice: Your Account Has Been Blocked"
		body = fmt.Sprintf(
			"Hello %s,\n\nSecurity Notice: Your Gateway account has been blocked by an administrator.\n\nYou will not be able to sign in while your account is blocked. Please contact your administrator if you believe this is an error.\n",
			username,
		)
	} else {
		subject = "Account Notice: Your Account Has Been Unblocked"
		loginURL := requestLoginURL(ctx)
		loginLine := ""
		if loginURL != "" && loginURL != "/login" {
			loginLine = fmt.Sprintf("\n\nSign in link: %s", loginURL)
		}
		body = fmt.Sprintf(
			"Hello %s,\n\nYour Gateway account has been unblocked by an administrator.\n\nYou can now sign in normally with your credentials.%s\n\nIf you have any questions, please contact your administrator.\n",
			username, loginLine,
		)
	}
	if err := sendAuthEmail(store, ctx, email, subject, body); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// trySendRegistrationDecisionEmail notifies the user after admin accept/reject.
// Sends when SMTP is enabled and the user has an email address.
func trySendRegistrationDecisionEmail(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username, email, decision string) (bool, string) {
	email = strings.TrimSpace(email)
	if email == "" || store == nil {
		return false, ""
	}
	smtpRow, err := store.GetSMTPConfig(ctx)
	if err != nil {
		return false, err.Error()
	}
	if !emailServiceConfigured(smtpRow) {
		return false, ""
	}
	var subject, body string
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approved", "approve", "accepted", "accept":
		subject = "Gateway account approved"
		body = accountApprovedEmailBody(username)
	case "rejected", "reject", "denied", "deny":
		subject = "Gateway registration not approved"
		body = accountRejectedEmailBody(username)
	default:
		return false, "unknown registration decision"
	}
	if err := sendAuthEmail(store, ctx, email, subject, body); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func loginUsernameKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// usernameCaseConflict reports whether another user's name differs from name only by case.
// Login lockout is keyed on the lowercased name, so such pairs would share one counter.
func usernameCaseConflict(ctx context.Context, store configstore.ConfigStore, name, excludeID string) bool {
	name = strings.TrimSpace(name)
	if store == nil || name == "" {
		return false
	}
	users, err := store.GetUsers(ctx)
	if err != nil {
		return false
	}
	for _, u := range users {
		if u == nil || u.ID == excludeID || u.Username == name {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(u.Username), name) {
			return true
		}
	}
	return false
}

func checkLoginLockout(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username string) (locked bool, retryAfter time.Duration, err error) {
	key := loginUsernameKey(username)
	if isLocked, retry, found := GetAuthCache().GetLockout(key); found {
		if isLocked {
			return true, retry, nil
		}
	}
	row, err := store.GetLoginLockout(ctx, key)
	if err != nil || row == nil || row.LockedUntil == nil {
		return false, 0, err
	}
	if time.Now().Before(*row.LockedUntil) {
		dur := time.Until(*row.LockedUntil)
		GetAuthCache().SetLockout(key, *row.LockedUntil)
		return true, dur, nil
	}
	return false, 0, nil
}

func recordLoginFailure(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username string) (locked bool, retryAfter time.Duration) {
	key := loginUsernameKey(username)
	now := time.Now()
	row, _ := store.GetLoginLockout(ctx, key)
	if row == nil {
		row = &tables.TableLoginLockout{UsernameKey: key}
	}
	if row.LockedUntil != nil && now.Before(*row.LockedUntil) {
		dur := time.Until(*row.LockedUntil)
		GetAuthCache().SetLockout(key, *row.LockedUntil)
		return true, dur
	}
	// Fresh window after lockout expired.
	if row.LockedUntil != nil && now.After(*row.LockedUntil) {
		row.FailedCount = 0
		row.LockedUntil = nil
	}
	row.FailedCount++
	row.LastFailedAt = &now
	if row.FailedCount >= loginMaxFailedAttempts {
		until := now.Add(loginLockoutDuration)
		row.LockedUntil = &until
		row.FailedCount = 0
		GetAuthCache().SetLockout(key, until)
		_ = store.UpsertLoginLockout(ctx, row)
		return true, loginLockoutDuration
	}
	_ = store.UpsertLoginLockout(ctx, row)
	return false, 0
}

func clearLoginFailures(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username string) {
	key := loginUsernameKey(username)
	GetAuthCache().ClearLockout(key)
	_ = store.ClearLoginLockout(ctx, key)
}

// clearAccountLockouts lifts the failed-login lock under every identifier an account
// can sign in with (username, email, aliases).
func clearAccountLockouts(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, identifiers ...string) {
	if store == nil {
		return
	}
	for _, id := range identifiers {
		if strings.TrimSpace(id) != "" {
			clearLoginFailures(store, ctx, id)
		}
	}
}

// lockAccountIdentifiers enforces the lockout window across all specified identifiers
// (username, email, login input) so they share the exact same lockout timing.
func lockAccountIdentifiers(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, until time.Time, identifiers ...string) {
	if store == nil {
		return
	}
	seen := make(map[string]bool)
	cache := GetAuthCache()
	for _, id := range identifiers {
		key := loginUsernameKey(id)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		cache.SetLockout(key, until)
		row := &tables.TableLoginLockout{
			UsernameKey: key,
			LockedUntil: &until,
			FailedCount: 0,
		}
		_ = store.UpsertLoginLockout(ctx, row)
	}
}

// retryMinutes rounds a remaining lockout up to whole minutes (never below 1).
func retryMinutes(retryAfter time.Duration) int {
	mins := int((retryAfter + time.Minute - 1) / time.Minute)
	if mins < 1 {
		return 1
	}
	return mins
}

// lockoutRetryMinutes sets Retry-After (seconds) so the UI can show a live countdown,
// and returns the remaining wait in whole minutes for the error message.
func lockoutRetryMinutes(ctx *fasthttp.RequestCtx, retryAfter time.Duration) int {
	secs := int((retryAfter + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	ctx.Response.Header.Set("Retry-After", strconv.Itoa(secs))
	return retryMinutes(retryAfter)
}

// trustProxyHeaders returns true only when TRUST_PROXY_HEADERS is explicitly enabled
// in .env / environment, indicating the gateway is behind a trusted reverse proxy
// (e.g. Cloudflare / Nginx) that sanitizes and overwrites client headers.
func trustProxyHeaders() bool {
	v := strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// directPeerIsTrustedProxy is true when the TCP peer may safely set X-Forwarded-*.
// With TRUSTED_PROXIES set, the peer must match that list; otherwise only
// loopback/private peers are trusted (same-host reverse proxy).
func directPeerIsTrustedProxy(ctx *fasthttp.RequestCtx) bool {
	if !trustProxyHeaders() || ctx == nil {
		return false
	}
	return ipIsTrustedProxy(ctx.RemoteIP())
}

// ipIsTrustedProxy: with TRUSTED_PROXIES set only those networks count; otherwise
// any loopback/private hop is treated as our own reverse proxy.
func ipIsTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if cidrs := trustedProxyCIDRs(); len(cidrs) > 0 {
		for _, n := range cidrs {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

var (
	trustedProxyCIDRsOnce sync.Once
	trustedProxyCIDRList  []*net.IPNet
)

func trustedProxyCIDRs() []*net.IPNet {
	trustedProxyCIDRsOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
		if raw == "" {
			return
		}
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !strings.Contains(part, "/") {
				if ip := net.ParseIP(part); ip != nil {
					if ip.To4() != nil {
						part += "/32"
					} else {
						part += "/128"
					}
				}
			}
			_, network, err := net.ParseCIDR(part)
			if err == nil && network != nil {
				trustedProxyCIDRList = append(trustedProxyCIDRList, network)
			}
		}
	})
	return trustedProxyCIDRList
}

func clientIPAddress(ctx *fasthttp.RequestCtx) string {
	if directPeerIsTrustedProxy(ctx) {
		if xff := string(ctx.Request.Header.Peek("X-Forwarded-For")); xff != "" {
			// Proxies append, so only the right-hand hops are trustworthy; the left-most
			// entry is whatever the client sent. Take the first hop (from the right) that
			// is not one of our own proxies.
			// When every hop is internal, the right-most one (written by our proxy) is used:
			// anything further left could be a forged "127.0.0.1".
			parts := strings.Split(xff, ",")
			rightmost := ""
			for i := len(parts) - 1; i >= 0; i-- {
				candidate := strings.TrimSpace(parts[i])
				ip := net.ParseIP(candidate)
				if ip == nil {
					continue
				}
				if rightmost == "" {
					rightmost = candidate
				}
				if !ipIsTrustedProxy(ip) {
					return candidate
				}
			}
			if rightmost != "" {
				return rightmost
			}
		}
		if xri := strings.TrimSpace(string(ctx.Request.Header.Peek("X-Real-IP"))); xri != "" {
			return xri
		}
	}
	return ctx.RemoteIP().String()
}

func loginDeviceFingerprint(ctx *fasthttp.RequestCtx) (fingerprint, ip, ua string) {
	ip = clientIPAddress(ctx)
	ua = string(ctx.Request.Header.Peek("User-Agent"))
	fingerprint = encrypt.HashSHA256(ip + "|" + ua)
	return fingerprint, ip, ua
}

// trySendLoginNoticeEmail sends only on first login, new device, or after extended inactivity (>7 days).
func trySendLoginNoticeEmail(store configstore.ConfigStore, ctx *fasthttp.RequestCtx, username, email string) {
	email = strings.TrimSpace(email)
	if store == nil || email == "" || username == "" {
		return
	}
	smtpRow, err := store.GetSMTPConfig(ctx)
	if err != nil || smtpRow == nil || !smtpRow.Enabled || !smtpRow.NotifyOnLogin {
		return
	}
	fp, ip, ua := loginDeviceFingerprint(ctx)
	key := loginUsernameKey(username)
	dev, err := store.GetLoginDevice(ctx, key, fp)

	isNewDevice := (dev == nil || err != nil)
	isInactiveReturn := false
	if dev != nil && !dev.LastSeenAt.IsZero() {
		if time.Since(dev.LastSeenAt) > 7*24*time.Hour {
			isInactiveReturn = true
		}
	}

	_ = store.UpsertLoginDevice(ctx, &tables.TableLoginDevice{
		UsernameKey: key,
		Fingerprint: fp,
		UserAgent:   truncateASCII(ua, 500),
		IPAddress:   truncateASCII(ip, 64),
	})

	if !isNewDevice && !isInactiveReturn {
		return
	}

	resetURL := publicLoginURL()
	resetLine := ""
	if resetURL != "" && resetURL != "/login" {
		forgotURL := resetURL + "?mode=forgot&email=" + url.QueryEscape(email)
		resetLine = fmt.Sprintf("If this was not you, secure your account and reset your password immediately here:\n%s\n", forgotURL)
	} else {
		resetLine = "If this was not you, secure your account and reset your password immediately on the sign-in page.\n"
	}

	var subject string
	var reason string
	if isInactiveReturn {
		subject = "Security Alert: Sign-in after extended inactivity"
		reason = "Your Gateway account was signed into after an extended period of inactivity."
	} else {
		subject = "Security Alert: Sign-in from new device or first-time login"
		reason = "Your Gateway account signed in from a new device or for the first time."
	}

	body := fmt.Sprintf(
		"Hello %s,\n\n"+
			"%s\n\n"+
			"• Account: %s\n"+
			"• IP Address: %s\n"+
			"• Device / Browser: %s\n"+
			"• Timestamp: %s\n\n"+
			"%s\n"+
			"Please keep your credentials confidential. If you authorized this sign-in, no action is needed.\n",
		username, reason, username, ip, truncateASCII(ua, 200), time.Now().Format(time.RFC1123), resetLine,
	)
	_ = sendAuthEmail(store, ctx, email, subject, body)
}

func truncateASCII(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func generateOTP6() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
