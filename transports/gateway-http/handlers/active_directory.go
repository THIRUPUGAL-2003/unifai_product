package handlers

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/gateway/gateway/framework/encrypt"
	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

const WorkspaceSettingActiveDirectory = "active_directory"

// ActiveDirectoryConfig stores settings to connect to an on-premises Microsoft Active Directory (AD DS) server via LDAP/LDAPS.
type ActiveDirectoryConfig struct {
	Enabled              bool   `json:"enabled"`
	ServerURL            string `json:"server_url"`             // e.g. ldap://192.168.1.100:389 or ldaps://ad.company.local:636
	Domain               string `json:"domain"`                 // e.g. company.local
	BaseDN               string `json:"base_dn"`                // e.g. DC=company,DC=local or OU=Employees,DC=company,DC=local
	BindDN               string `json:"bind_dn"`                // e.g. CN=Administrator,CN=Users,DC=company,DC=local or admin@company.local
	BindPassword         string `json:"bind_password"`          // Service account password
	UseTLS               bool   `json:"use_tls"`                // Upgrade plain connection to StartTLS
	InsecureSkipTLS      bool   `json:"insecure_skip_tls"`      // Trust self-signed certificates from internal AD
	UserFilter           string `json:"user_filter"`            // e.g. (&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))
	UsernameAttr         string `json:"username_attr"`          // default: sAMAccountName
	EmailAttr            string `json:"email_attr"`             // default: mail
	DisplayNameAttr      string `json:"display_name_attr"`      // default: displayName
	DefaultRole          string `json:"default_role"`           // default: user
	LastSyncAt           string `json:"last_sync_at,omitempty"`
	LastSyncStatus       string `json:"last_sync_status,omitempty"`
	LastSyncMessage      string `json:"last_sync_message,omitempty"`
	LastSyncCount        int    `json:"last_sync_count,omitempty"`
	DiscoveredUsersCount int    `json:"discovered_users_count,omitempty"`
}

const adPasswordRedacted = "********"

func (cfg *ActiveDirectoryConfig) applyDefaults() {
	if cfg.ServerURL == "" {
		cfg.ServerURL = "ldap://127.0.0.1:389"
	}
	if cfg.UserFilter == "" {
		cfg.UserFilter = "(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))"
	}
	if cfg.UsernameAttr == "" {
		cfg.UsernameAttr = "sAMAccountName"
	}
	if cfg.EmailAttr == "" {
		cfg.EmailAttr = "mail"
	}
	if cfg.DisplayNameAttr == "" {
		cfg.DisplayNameAttr = "displayName"
	}
	if cfg.DefaultRole == "" {
		cfg.DefaultRole = "user"
	}
}

// connectLDAP establishes a connection and binds to Microsoft Active Directory.
func connectLDAP(cfg *ActiveDirectoryConfig) (*ldap.Conn, error) {
	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		return nil, fmt.Errorf("Active Directory server URL is required (e.g. ldap://192.168.1.50:389)")
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipTLS,
	}

	var conn *ldap.Conn
	var err error

	isLDAPS := strings.HasPrefix(strings.ToLower(serverURL), "ldaps://")
	if isLDAPS {
		addr := strings.TrimPrefix(serverURL, "ldaps://")
		if !strings.Contains(addr, ":") {
			addr += ":636"
		}
		conn, err = ldap.DialTLS("tcp", addr, tlsConfig)
	} else {
		addr := strings.TrimPrefix(serverURL, "ldap://")
		if !strings.Contains(addr, ":") {
			addr += ":389"
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err = ldap.DialURL(serverURL, ldap.DialWithDialer(dialer))
		if err != nil {
			conn, err = ldap.Dial("tcp", addr)
		}
		if err == nil && cfg.UseTLS {
			if startTLSErr := conn.StartTLS(tlsConfig); startTLSErr != nil {
				conn.Close()
				return nil, fmt.Errorf("StartTLS handshake failed: %w", startTLSErr)
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to Active Directory server (%s): %w", serverURL, err)
	}

	// Authenticate using BindDN & Password
	bindDN := strings.TrimSpace(cfg.BindDN)
	if bindDN != "" {
		if bindErr := conn.Bind(bindDN, cfg.BindPassword); bindErr != nil {
			conn.Close()
			return nil, fmt.Errorf("Active Directory authentication (Bind) failed for '%s': %w", bindDN, bindErr)
		}
	} else {
		// Unauthenticated bind attempt
		if bindErr := conn.UnauthenticatedBind(""); bindErr != nil {
			conn.Close()
			return nil, fmt.Errorf("anonymous bind failed (service account credentials required): %w", bindErr)
		}
	}

	return conn, nil
}

// getActiveDirectoryConfig handles GET /api/scim/ad/config
func (h *WorkspaceHandler) getActiveDirectoryConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}

	row, err := store.GetWorkspaceSetting(ctx, WorkspaceSettingActiveDirectory)
	if isStoreNotFound(err) || row == nil || row.Data == "" {
		cfg := ActiveDirectoryConfig{Enabled: false}
		cfg.applyDefaults()
		SendJSON(ctx, cfg)
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load Active Directory configuration")
		return
	}

	var cfg ActiveDirectoryConfig
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse Active Directory configuration")
		return
	}
	cfg.applyDefaults()

	if cfg.BindPassword != "" {
		cfg.BindPassword = adPasswordRedacted
	}

	SendJSON(ctx, cfg)
}

// updateActiveDirectoryConfig handles PUT /api/scim/ad/config
func (h *WorkspaceHandler) updateActiveDirectoryConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}

	if h.callerRole(ctx) != "admin" {
		SendError(ctx, fasthttp.StatusForbidden, "only administrators can configure Active Directory")
		return
	}

	var incoming ActiveDirectoryConfig
	if err := json.Unmarshal(ctx.PostBody(), &incoming); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid Active Directory configuration payload")
		return
	}
	incoming.applyDefaults()

	// Restore preserved password if placeholder was sent
	if incoming.BindPassword == adPasswordRedacted || incoming.BindPassword == "" {
		if row, err := store.GetWorkspaceSetting(ctx, WorkspaceSettingActiveDirectory); err == nil && row != nil && row.Data != "" {
			var prev ActiveDirectoryConfig
			if json.Unmarshal([]byte(row.Data), &prev) == nil {
				incoming.BindPassword = prev.BindPassword
				if incoming.LastSyncAt == "" {
					incoming.LastSyncAt = prev.LastSyncAt
					incoming.LastSyncStatus = prev.LastSyncStatus
					incoming.LastSyncMessage = prev.LastSyncMessage
					incoming.LastSyncCount = prev.LastSyncCount
					incoming.DiscoveredUsersCount = prev.DiscoveredUsersCount
				}
			}
		}
	}

	raw, err := json.Marshal(incoming)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to serialize Active Directory configuration")
		return
	}

	if err := store.UpsertWorkspaceSetting(ctx, WorkspaceSettingActiveDirectory, string(raw)); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save Active Directory configuration")
		return
	}

	if incoming.BindPassword != "" {
		incoming.BindPassword = adPasswordRedacted
	}

	SendJSON(ctx, incoming)
}

// testActiveDirectoryConnection handles POST /api/scim/ad/test
func (h *WorkspaceHandler) testActiveDirectoryConnection(ctx *fasthttp.RequestCtx) {
	var cfg ActiveDirectoryConfig
	if err := json.Unmarshal(ctx.PostBody(), &cfg); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	cfg.applyDefaults()

	// If password is redacted, read stored password
	if cfg.BindPassword == adPasswordRedacted || cfg.BindPassword == "" {
		store := h.requireStore(ctx)
		if store != nil {
			if row, err := store.GetWorkspaceSetting(ctx, WorkspaceSettingActiveDirectory); err == nil && row != nil && row.Data != "" {
				var prev ActiveDirectoryConfig
				if json.Unmarshal([]byte(row.Data), &prev) == nil {
					cfg.BindPassword = prev.BindPassword
				}
			}
		}
	}

	conn, err := connectLDAP(&cfg)
	if err != nil {
		SendJSON(ctx, map[string]any{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	defer conn.Close()

	baseDN := strings.TrimSpace(cfg.BaseDN)
	if baseDN == "" {
		SendJSON(ctx, map[string]any{
			"success": true,
			"message": "Connected and authenticated successfully to Active Directory! Note: Please set Base DN (e.g. DC=company,DC=local) to query users.",
		})
		return
	}

	// Test user search query with limit 5
	searchReq := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 5, 10, false,
		cfg.UserFilter,
		[]string{cfg.UsernameAttr, cfg.EmailAttr, cfg.DisplayNameAttr, "userPrincipalName", "cn"},
		nil,
	)

	sr, err := conn.Search(searchReq)
	if err != nil {
		SendJSON(ctx, map[string]any{
			"success": false,
			"message": fmt.Sprintf("Authentication succeeded, but Base DN user search failed: %v", err),
		})
		return
	}

	sampleUsers := make([]string, 0, len(sr.Entries))
	for _, entry := range sr.Entries {
		name := entry.GetAttributeValue(cfg.UsernameAttr)
		if name == "" {
			name = entry.GetAttributeValue("userPrincipalName")
		}
		if name == "" {
			name = entry.GetAttributeValue("cn")
		}
		if name != "" {
			email := entry.GetAttributeValue(cfg.EmailAttr)
			if email != "" {
				name += " (" + email + ")"
			}
			sampleUsers = append(sampleUsers, name)
		}
	}

	SendJSON(ctx, map[string]any{
		"success":          true,
		"message":          fmt.Sprintf("Active Directory connection & search succeeded! Found %d sample users in Base DN.", len(sr.Entries)),
		"discovered_count": len(sr.Entries),
		"sample_users":     sampleUsers,
	})
}

// syncActiveDirectoryUsers handles POST /api/scim/ad/sync
func (h *WorkspaceHandler) syncActiveDirectoryUsers(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}

	if h.callerRole(ctx) != "admin" {
		SendError(ctx, fasthttp.StatusForbidden, "only administrators can perform Active Directory sync")
		return
	}

	row, err := store.GetWorkspaceSetting(ctx, WorkspaceSettingActiveDirectory)
	if err != nil || row == nil || row.Data == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Active Directory is not configured yet. Please configure server details first.")
		return
	}

	var cfg ActiveDirectoryConfig
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse Active Directory configuration")
		return
	}
	cfg.applyDefaults()

	if !cfg.Enabled {
		SendError(ctx, fasthttp.StatusBadRequest, "Active Directory sync is currently disabled. Please enable it in settings.")
		return
	}

	baseDN := strings.TrimSpace(cfg.BaseDN)
	if baseDN == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Base DN is required for Active Directory user sync (e.g. DC=company,DC=local)")
		return
	}

	conn, err := connectLDAP(&cfg)
	if err != nil {
		h.recordADSyncResult(ctx, &cfg, "failed", err.Error(), 0, 0)
		SendError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	defer conn.Close()

	// Search up to 1000 users in Active Directory
	searchReq := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1000, 30, false,
		cfg.UserFilter,
		[]string{cfg.UsernameAttr, cfg.EmailAttr, cfg.DisplayNameAttr, "userPrincipalName", "cn"},
		nil,
	)

	sr, err := conn.Search(searchReq)
	if err != nil {
		msg := fmt.Sprintf("Active Directory user search failed: %v", err)
		h.recordADSyncResult(ctx, &cfg, "failed", msg, 0, 0)
		SendError(ctx, fasthttp.StatusInternalServerError, msg)
		return
	}

	discoveredCount := len(sr.Entries)
	createdCount := 0
	existingCount := 0
	quotaReached := false

	now := time.Now().UTC()
	for _, entry := range sr.Entries {
		username := strings.TrimSpace(entry.GetAttributeValue(cfg.UsernameAttr))
		if username == "" {
			upn := entry.GetAttributeValue("userPrincipalName")
			if upn != "" {
				username = strings.Split(upn, "@")[0]
			}
		}
		if username == "" {
			username = strings.TrimSpace(entry.GetAttributeValue("cn"))
		}
		if username == "" {
			continue
		}

		// Normalize username
		username = strings.ToLower(username)

		email := strings.TrimSpace(strings.ToLower(entry.GetAttributeValue(cfg.EmailAttr)))
		if email == "" {
			upn := strings.TrimSpace(entry.GetAttributeValue("userPrincipalName"))
			if strings.Contains(upn, "@") {
				email = strings.ToLower(upn)
			}
		}
		if email == "" && cfg.Domain != "" {
			email = username + "@" + strings.TrimPrefix(cfg.Domain, "@")
		}

		// Check if user already exists
		existing, getErr := h.store.ConfigStore.GetUserByUsername(ctx, username)
		if getErr == nil && existing != nil {
			existingCount++
			continue
		}

		// Verify license limit before creating new user
		if h.store != nil && h.store.ConfigStore != nil {
			usage, usageErr := loadProductUserUsage(ctx, h.store.ConfigStore)
			if usageErr == nil {
				if blockReason := usage.BlockReason(); blockReason != "" {
					quotaReached = true
					logger.Warn("Active Directory sync stopped user creation: %s", blockReason)
					break
				}
			}
		}

		// Generate random internal secret for the user
		tempPass, _ := generateTemporaryPassword()
		hashedPassword, hashErr := encrypt.Hash(tempPass)
		if hashErr != nil {
			hashedPassword = "ad-managed-account"
		}

		role := cfg.DefaultRole
		if role == "" {
			role = "user"
		}

		newUser := &tables.TableUser{
			ID:                 uuid.NewString(),
			Username:           username,
			Email:              email,
			Password:           hashedPassword,
			Role:               role,
			Status:             tables.UserStatusApproved,
			ExternalID:         "ad:" + username,
			MustChangePassword: false,
			CreatedAt:          now,
			UpdatedAt:          now,
		}

		if createErr := h.store.ConfigStore.CreateUser(ctx, newUser); createErr != nil {
			logger.Warn("failed to persist Active Directory user %s: %v", username, createErr)
			continue
		}

		if h.promptLifecycle != nil {
			_ = h.promptLifecycle.OnUserCreated(ctx, newUser, true)
		}

		createdCount++
	}

	statusMsg := fmt.Sprintf("Successfully synced Active Directory! Discovered %d users: %d new users created, %d existing users.", discoveredCount, createdCount, existingCount)
	if quotaReached {
		statusMsg += " Note: Further accounts paused because the license user limit was reached."
	}

	h.recordADSyncResult(ctx, &cfg, "success", statusMsg, createdCount, discoveredCount)

	SendJSON(ctx, map[string]any{
		"success":          true,
		"message":          statusMsg,
		"discovered_count": discoveredCount,
		"created_count":    createdCount,
		"existing_count":   existingCount,
		"quota_reached":    quotaReached,
	})
}

func (h *WorkspaceHandler) recordADSyncResult(ctx *fasthttp.RequestCtx, cfg *ActiveDirectoryConfig, status, message string, syncedCount, discoveredCount int) {
	cfg.LastSyncAt = time.Now().UTC().Format(time.RFC3339)
	cfg.LastSyncStatus = status
	cfg.LastSyncMessage = message
	cfg.LastSyncCount = syncedCount
	cfg.DiscoveredUsersCount = discoveredCount

	raw, err := json.Marshal(cfg)
	if err == nil {
		store := h.requireStore(ctx)
		if store != nil {
			_ = store.UpsertWorkspaceSetting(ctx, WorkspaceSettingActiveDirectory, string(raw))
		}
	}
}
