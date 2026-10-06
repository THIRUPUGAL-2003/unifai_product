package logstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/raksha/raksha/framework/encrypt"
	"golang.org/x/net/idna"
	"gorm.io/gorm"
)

var pacHostSafe = regexp.MustCompile(`^[a-z0-9.-]+$`)
var pacProxyAddrSafe = regexp.MustCompile(`^[A-Za-z0-9.:\[\]-]+$`)
var nicGUID = regexp.MustCompile(`(?i)\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}`)
var ideExeAtHash = regexp.MustCompile(`(?i)\b[\w.-]+\.exe\*@`)
var hexAtHash24 = regexp.MustCompile(`(?i)@[a-f0-9]{24,}`)
var cpuVendorTelemetry = regexp.MustCompile(`(?i)\b(?:amd|intel|qualcomm|apple)\b.+\b(?:cpu|gpu|mhz|ghz)\b`)

// IsOpaqueOrWirePrompt reports IDE/binary/wire junk that must never drive Guard Bot or Prompt Logs.
func IsOpaqueOrWirePrompt(s string) bool {
	return looksLikeBinaryOrWireGarbage(s)
}

func defaultProxyAddrFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("RAKSHA_PROXY_ADDR")); v != "" {
		return v
	}
	if p := strings.TrimSpace(os.Getenv("PROXY_PORT")); p != "" {
		return "127.0.0.1:" + p
	}
	// No stale hardcoded port — set RAKSHA_PROXY_ADDR in .env / fleet config.
	return ""
}

// looksLikeBinaryOrWireGarbage rejects IDE/proxy decode soup that is not a real typed prompt.
// Mirrors apps/browser-guard/proxy/browser_ai_proxy.py::_looks_like_binary_or_wire_garbage.
func looksLikeBinaryOrWireGarbage(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return true
	}
	low := strings.ToLower(t)

	// Cursor / IDE exe attestation + content hashes (not typed chat)
	if strings.Contains(low, "cursor.exe") || ideExeAtHash.MatchString(t) {
		return true
	}
	if strings.Contains(low, ".exe") && strings.Contains(t, "@") && hexAtHash24.MatchString(t) {
		return true
	}
	if strings.Contains(low, "intel(r)") || strings.Contains(low, "core(tm)") {
		return true
	}
	if strings.Contains(low, "cpu @") && strings.Contains(low, "ghz") {
		return true
	}
	if cpuVendorTelemetry.MatchString(t) && len([]rune(t)) < 120 {
		return true
	}
	if strings.Contains(t, "*") && strings.Contains(t, "@") && len([]rune(t)) >= 40 {
		hexish := 0
		runesT := []rune(t)
		for _, r := range runesT {
			if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
				hexish++
			}
		}
		if float64(hexish)/float64(len(runesT)) >= 0.5 {
			return true
		}
	}

	for _, r := range t {
		if r < 9 || (r > 10 && r < 32 && r != 13) || r == 127 {
			return true
		}
	}
	runes := []rune(t)
	n := len(runes)

	// Only reject true replacement-character mojibake (>= 8% of content).
	// NEVER treat non-ASCII (Tamil, Hindi, Arabic, Chinese, Russian, emojis, etc.) as garbage!
	mojibake := 0
	for _, r := range runes {
		if r == '\ufffd' {
			mojibake++
		}
	}
	if n > 0 && float64(mojibake)/float64(n) >= 0.08 {
		return true
	}

	// Any non-ASCII Unicode script (Tamil, Hindi, Chinese, Japanese, Arabic, Russian, emojis, etc.)
	// is ALWAYS a real user prompt in world languages. Never drop by language!
	for _, r := range runes {
		if r > 127 {
			return false
		}
	}

	alnum, other := 0, 0
	for _, r := range runes {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			alnum++
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			// whitespace
		default:
			other++
		}
	}

	// User-typed symbols of ANY length, emojis, or punctuation (e.g. "!@#$%^%%#$%@#!^", "@$$$%@#$%$%@$%", "$$$", "???")
	// must ALWAYS be preserved in Prompt Logs and evaluated by Guard Rules!
	if alnum == 0 && other > 0 {
		return false
	}

	return false
}

// GuardSeverityScore maps rule severity to predictive risk score + label.
func GuardSeverityScore(severity string) (int, string) {
	return guardSeverityScore(severity)
}

// guardSeverityScore maps rule severity to predictive risk score + label.
func guardSeverityScore(severity string) (int, string) {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "CRITICAL":
		return 95, "CRITICAL"
	case "HIGH":
		return 80, "HIGH"
	case "MEDIUM":
		return 55, "MEDIUM"
	case "LOW":
		return 30, "LOW"
	default:
		return 85, "HIGH"
	}
}

// CompileGuardRegex builds a case-insensitive RE2 pattern, stripping a leading (?i) from admin input.
func CompileGuardRegex(pattern string) (*regexp.Regexp, error) {
	p := strings.TrimSpace(pattern)
	if len(p) >= 4 && strings.EqualFold(p[:4], "(?i)") {
		p = p[4:]
	} else if len(p) >= 5 && strings.EqualFold(p[:5], "(?-i)") {
		p = p[5:]
	}
	return regexp.Compile("(?i)" + p)
}

type BrowserAILog struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Timestamp time.Time `gorm:"index" json:"timestamp"`
	Platform  string    `gorm:"index" json:"platform"`
	// Domain: monitored target host (indexed) — also kept in Metadata for legacy readers.
	Domain            string `gorm:"index" json:"domain"`
	UserPromptPreview string `json:"user_prompt_preview"`
	UserPromptFull    string `gorm:"type:text" json:"user_prompt_full"`
	EstTokens         int    `json:"est_tokens"`
	ClientIP          string `json:"client_ip"`
	AgentID           string `gorm:"index" json:"agent_id"`
	AgentHostname     string `json:"agent_hostname"`
	Status            string `gorm:"index" json:"status"`
	Action            string `json:"action"`
	RuleTriggered     string `json:"rule_triggered"`
	RiskScore         int    `json:"risk_score"`
	PredictiveRisk    string `json:"predictive_risk"`    // "LOW", "MEDIUM", "HIGH", "CRITICAL"
	PredictedCategory string `json:"predicted_category"` // "SAFE", "SECRET_LEAK_REDACTED", "SECURITY_POLICY_VIOLATION", "PREDICTED_SECRET_EXPOSURE"
	ReplyBotProvider  string `json:"reply_bot_provider"`
	ReplyBotModel     string `json:"reply_bot_model"`
	ReplyBotText      string `gorm:"type:text" json:"reply_bot_text"`
	// Attachment* — intercepted file upload stored under APP_DIR/attachments (legacy: pdf/).
	AttachmentName        string     `json:"attachment_name,omitempty"`
	AttachmentStoredName  string     `json:"attachment_stored_name,omitempty"` // basename only under pdf/
	AttachmentContentType string     `json:"attachment_content_type,omitempty"`
	AttachmentSizeBytes   int64      `json:"attachment_size_bytes,omitempty"`
	AttachmentPath        string     `json:"attachment_path,omitempty"` // relative path under APP_DIR
	AttachmentExpiresAt   *time.Time `json:"attachment_expires_at,omitempty"`
	Metadata              string     `gorm:"type:text" json:"metadata"`
	CreatedAt             time.Time  `json:"created_at"`
}

// BrowserAISearchLog represents an enterprise search event stored in Postgres (visible in pgAdmin).
type BrowserAISearchLog struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	Timestamp      time.Time `gorm:"index" json:"timestamp"`
	Engine         string    `gorm:"index" json:"engine"`          // "Google", "Bing", "Safari / Apple", "DuckDuckGo", "Yahoo"
	Browser        string    `gorm:"index" json:"browser"`         // "Edge", "Chrome", "Safari", "Firefox", "Brave"
	IsIncognito    bool      `gorm:"index" json:"is_incognito"`    // true if incognito / private mode
	Query          string    `gorm:"type:text" json:"query"`       // Searched keywords
	ClickedURL     string    `gorm:"type:text" json:"clicked_url"` // URL of search result link clicked
	ClickedTitle   string    `gorm:"type:text" json:"clicked_title"`
	URL            string    `gorm:"type:text" json:"url"` // Raw search URL
	Host           string    `gorm:"index" json:"host"`    // Search engine host
	ClientIP       string    `json:"client_ip"`
	AgentHostname  string    `json:"agent_hostname"`
	AgentID        string    `gorm:"index" json:"agent_id"`
	RiskScore      int       `json:"risk_score"`
	PredictiveRisk string    `gorm:"index" json:"predictive_risk"` // "LOW" | "MEDIUM" | "HIGH" | "CRITICAL"
	RiskCategory   string    `json:"risk_category"`
	CreatedAt      time.Time `json:"created_at"`
}

func (BrowserAISearchLog) TableName() string { return "browser_ai_search_logs" }

// BrowserGuardFleetConfigID is the singleton fleet defaults row.
const BrowserGuardFleetConfigID = "browser-guard-fleet-default"

// BrowserGuardFleetConfig stores company-wide Guard defaults in the same Postgres DB
// (laptop + network modes). Agents pull this on heartbeat; local JSON is install override only.
type BrowserGuardFleetConfig struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	DefaultProxyAddr string    `json:"default_proxy_addr"`
	PacAdvertiseAddr string    `json:"pac_advertise_addr"`
	ServerModePolicy string    `json:"server_mode_policy"` // endpoint_default | network_allowed | network_preferred
	ListenHostPolicy string    `json:"listen_host_policy"` // 127.0.0.1 | 0.0.0.0
	PacSyncSeconds   int       `json:"pac_sync_seconds"`
	AgentTypeDefault string    `json:"agent_type_default"` // endpoint | network
	BackendURLHint   string    `json:"backend_url_hint"`
	Notes            string    `gorm:"type:text" json:"notes"`
	UpdatedAt        time.Time `json:"updated_at"`
	UpdatedBy        string    `json:"updated_by"`
}

func (BrowserGuardFleetConfig) TableName() string { return "browser_guard_fleet_config" }

// BrowserGuardRebuildLog is one row per admin "Rebuild" press (success or failure).
type BrowserGuardRebuildLog struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	Status        string    `gorm:"index" json:"status"` // success | failed
	Mode          string    `json:"mode"`                // prebuilt (code hot-update only) | rebuilt (installers rebuilt)
	Version       string    `json:"version"`
	MacVersion    string    `json:"mac_version"`
	WindowsReady  bool      `json:"windows_ready"`
	MacosReady    bool      `json:"macos_ready"`
	BundleSHA     string    `gorm:"type:varchar(64)" json:"bundle_sha"`
	BundleFiles   int       `json:"bundle_files"`
	GuardVersions string    `json:"guard_versions"`
	Message       string    `gorm:"type:text" json:"message"`
	Log           string    `gorm:"type:text" json:"log"`
	RequestedBy   string    `json:"requested_by"`
	DurationMs    int64     `json:"duration_ms"`
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
}

func (BrowserGuardRebuildLog) TableName() string { return "browser_guard_rebuild_logs" }

// RecordGuardRebuild stores a Rebuild history row.
func (m *BrowserAIManager) RecordGuardRebuild(ctx context.Context, entry *BrowserGuardRebuildLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || entry == nil {
		return fmt.Errorf("database not initialized")
	}
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	return m.db.WithContext(ctx).Create(entry).Error
}

// ListGuardRebuilds returns the newest Rebuild history rows first.
func (m *BrowserAIManager) ListGuardRebuilds(ctx context.Context, limit int) ([]BrowserGuardRebuildLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []BrowserGuardRebuildLog{}
	if m.db == nil {
		return out, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	err := m.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

// BrowserAIAgent tracks every installed Guard EXE (unlimited scale in raksha_new).
type BrowserAIAgent struct {
	ID       string `gorm:"primaryKey" json:"id"`
	Hostname string `gorm:"index" json:"hostname"`
	Username string `json:"username"`
	// ContactEmail: preferred address for Guard Insights warning mail (optional).
	ContactEmail string `json:"contact_email"`
	// ContactEmailPinned: admin saved (or cleared) ContactEmail — heartbeats and warning mails never change it.
	ContactEmailPinned bool   `gorm:"default:false" json:"contact_email_pinned"`
	IPAddress          string `json:"ip_address"`
	MacAddress         string `json:"mac_address"`
	TransportName      string `json:"transport_name"`
	OSVersion          string `json:"os_version"`
	AgentVersion       string `json:"agent_version"`
	// AgentType: endpoint (laptop Guard) | network (shared/server proxy). Same dashboard.
	AgentType string `gorm:"index" json:"agent_type"`
	// HealthStatus: ok | degraded | error — reported by Guard EXE health loop.
	HealthStatus       string `json:"health_status"`
	HealthDetail       string `gorm:"type:text" json:"health_detail"`
	Status             string `gorm:"index" json:"status"` // active | uninstall_pending | uninstalled
	UninstallRequested bool   `json:"uninstall_requested"`
	// Per-Guard uninstall key (auto-generated). Hash for verify; Enc for admin reveal.
	UninstallKeyHash      string     `gorm:"type:varchar(128)" json:"-"`
	UninstallKeyEnc       string     `gorm:"type:text" json:"-"`
	HasUninstallKey       bool       `gorm:"-" json:"has_uninstall_key"`
	UninstallKeyRotatedAt *time.Time `gorm:"index" json:"uninstall_key_rotated_at,omitempty"`
	LastSeenAt            time.Time  `gorm:"index" json:"last_seen_at"`
	InstalledAt           time.Time  `json:"installed_at"`
	// VersionUpdatedAt: first heartbeat on the current AgentVersion (install or auto-update).
	VersionUpdatedAt *time.Time `json:"version_updated_at,omitempty"`
	// ProxyBundleSHA: server-published proxy code the Guard is running ("" = code built into the installer).
	ProxyBundleSHA       string     `gorm:"type:varchar(64)" json:"proxy_bundle_sha"`
	ProxyBundleUpdatedAt *time.Time `json:"proxy_bundle_updated_at,omitempty"`
	UninstalledAt        *time.Time `json:"uninstalled_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// BrowserAIWarningEmailLog audits Guard Insights / Agents security warning emails.
type BrowserAIWarningEmailLog struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	SentAt        time.Time `gorm:"index" json:"sent_at"`
	ToEmail       string    `gorm:"index" json:"to_email"`
	Subject       string    `json:"subject"`
	Message       string    `gorm:"type:text" json:"message"`
	AgentID       string    `gorm:"index" json:"agent_id"`
	AgentHostname string    `json:"agent_hostname"`
	SentBy        string    `json:"sent_by"`
	Status        string    `json:"status"` // sent | failed
	ErrorDetail   string    `gorm:"type:text" json:"error_detail,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func (BrowserAIWarningEmailLog) TableName() string { return "browser_ai_warning_emails" }

// BrowserAIAgentInsightStats is per-agent action totals for Guard Insights (full DB, not windowed).
type BrowserAIAgentInsightStats struct {
	AgentID      string `json:"agent_id"`
	AllowedCount int64  `json:"allowed_count"`
	BlockedCount int64  `json:"blocked_count"`
	WarnCount    int64  `json:"warn_count"`
	RedactCount  int64  `json:"redact_count"`
	TotalCount   int64  `json:"total_count"`
}

// NormalizeBrowserAIAgentType maps free-form values onto endpoint|network.
func NormalizeBrowserAIAgentType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "network", "server", "gateway", "corp", "shared":
		return "network"
	default:
		return "endpoint"
	}
}

// BrowserAIAgentSettings stores company uninstall-key policy (single row).
type BrowserAIAgentSettings struct {
	ID                  string    `gorm:"primaryKey" json:"id"`
	UninstallKeyHash    string    `json:"-"`
	UninstallKeyEnc     string    `gorm:"type:text" json:"-"`
	RequireUninstallKey bool      `json:"require_uninstall_key"`
	KeyConfigured       bool      `gorm:"-" json:"key_configured"`
	UpdatedAt           time.Time `json:"updated_at"`
	UpdatedBy           string    `json:"updated_by"`
}

const BrowserAIAgentSettingsID = "browser-agent-settings-default"
const AgentStatusActive = "active"
const AgentStatusPaused = "paused"
const AgentStatusInactive = "inactive"
const AgentStatusSleep = "sleep"
const AgentStatusShutdown = "shutdown"
const AgentStatusUninstalled = "uninstalled"
const AgentStatusUninstallPending = "uninstall_pending"

type BrowserGuardRule struct {
	ID                    string    `gorm:"primaryKey" json:"id"`
	Name                  string    `json:"name"`
	RuleType              string    `json:"rule_type"`                                      // "regex" (default) or "ai_bot"
	BotProvider           string    `json:"bot_provider"`                                   // e.g. "openai", "anthropic", "gemini"
	BotModel              string    `json:"bot_model"`                                      // e.g. "gpt-4o-mini", "claude-3-5-haiku"
	BotPrompt             string    `gorm:"type:text" json:"bot_prompt"`                    // Custom evaluation instruction for the LLM
	BotReferenceImage     string    `gorm:"type:text" json:"bot_reference_image,omitempty"` // Base64 reference template (vision rules)
	BotReferenceImageType string    `json:"bot_reference_image_type,omitempty"`             // e.g. image/png
	Severity              string    `json:"severity"`                                       // "CRITICAL", "HIGH", "MEDIUM"
	Action                string    `json:"action"`                                         // "BLOCK", "REDACT", or "WARN"
	Pattern               string    `json:"pattern"`
	Active                bool      `json:"active"`
	Description           string    `json:"description"`
	WarningMessage        string    `json:"warning_message"` // shown in-chat when this rule blocks
	CreatedAt             time.Time `json:"created_at"`
}

// BrowserControlSettings is a single-row policy for browser AI interaction controls.
type BrowserControlSettings struct {
	ID            string `gorm:"primaryKey" json:"id"`
	Enabled       bool   `json:"enabled"`                         // master switch for upload control
	BlockUpload   bool   `json:"block_upload"`                    // block file uploads to AI sites
	UploadWarning string `gorm:"type:text" json:"upload_warning"` // shown when this upload policy blocks; empty = no message
	// AttachmentRetention: temporary file binary storage retention ("10m" | "1h" | "2h" | "1d" | "7d")
	AttachmentRetention string `json:"attachment_retention"`
	// Search log retention: when SearchLogAutoDelete is true, logs older than
	// SearchLogRetention (1d|7d|30d|90d|180d|365d) are purged automatically.
	SearchLogAutoDelete bool   `json:"search_log_auto_delete"`
	SearchLogRetention  string `json:"search_log_retention"` // "1d" | "7d" | "30d" | "90d" | "180d" | "365d"
	// Prompt log retention: when PromptLogAutoDelete is true, logs older than
	// PromptLogRetention (1d|7d|30d|90d|180d|365d) are purged automatically.
	PromptLogAutoDelete bool      `json:"prompt_log_auto_delete"`
	PromptLogRetention  string    `json:"prompt_log_retention"` // "1d" | "7d" | "30d" | "90d" | "180d" | "365d"
	UpdatedAt           time.Time `json:"updated_at"`
}

type BrowserTargetWebsite struct {
	ID           string `gorm:"primaryKey" json:"id"`
	Domain       string `gorm:"uniqueIndex" json:"domain"`
	PlatformName string `json:"platform_name"`
	Monitored    bool   `json:"monitored"`
	// BlockSite: when true, Guard blocks opening the whole website (not only prompts).
	BlockSite        bool   `json:"block_site"`
	InterceptedCount int64  `json:"intercepted_count"`
	Status           string `json:"status"` // "MONITORED", "PAUSED", "BLOCKED"
	ReplyBotEnabled  bool   `json:"reply_bot_enabled"`
	ReplyBotProvider string `json:"reply_bot_provider"`
	ReplyBotModel    string `json:"reply_bot_model"`
	// ReplyBotMode: "violations" (default) = reply only on Guard BLOCK;
	// "all" = answer every intercepted prompt via Reply Bot (never forward to the site AI).
	ReplyBotMode string `json:"reply_bot_mode"`
	// ParentID: related host nested under another Target Website. Empty = top-level domain.
	ParentID string `gorm:"index" json:"parent_id"`
	// HostRole: admin label — "ui" | "chat" | "file" | "" (auto). Drives proxy intercept path.
	HostRole  string    `json:"host_role"`
	CreatedAt time.Time `json:"created_at"`
}

func (BrowserAILog) TableName() string {
	return "browser_ai_logs"
}

func (BrowserGuardRule) TableName() string {
	return "browser_guard_rules"
}

func (BrowserControlSettings) TableName() string {
	return "browser_control_settings"
}

func (BrowserTargetWebsite) TableName() string {
	return "browser_target_websites"
}

func (BrowserAIAgent) TableName() string {
	return "browser_ai_agents"
}

func (BrowserAIAgentSettings) TableName() string {
	return "browser_ai_agent_settings"
}

const BrowserControlSettingsID = "browser-controls-default"

// NormalizeDomain cleans values like "https://www.example.com/", "*.example.com" or
// "example.com." -> "example.com" (same rules as the Guard). Unicode names become punycode.
// Returns "" when the value cannot be a host, so it is rejected instead of silently
// missing from proxy.pac.
func NormalizeDomain(raw string) string {
	domain := cleanDomainInput(raw)
	if domain == "" {
		return ""
	}
	if ip := net.ParseIP(domain); ip != nil {
		return domain
	}
	if ascii, err := idna.Lookup.ToASCII(domain); err == nil && ascii != "" {
		domain = ascii
	}
	if !pacHostSafe.MatchString(domain) || strings.Contains(domain, "..") {
		return ""
	}
	return domain
}

func cleanDomainInput(raw string) string {
	domain := strings.TrimSpace(strings.ToLower(raw))
	if domain == "" {
		return ""
	}
	if i := strings.Index(domain, "://"); i >= 0 {
		domain = domain[i+3:]
	}
	domain = strings.Split(domain, "/")[0]
	domain = strings.Split(domain, "?")[0]
	domain = strings.Split(domain, "#")[0]
	if strings.HasPrefix(domain, "[") {
		if end := strings.Index(domain, "]"); end > 0 {
			domain = domain[1:end]
		}
	} else if i := strings.LastIndex(domain, ":"); i > 0 {
		// strip port, but keep IPv6 handled above
		maybePort := domain[i+1:]
		if maybePort != "" && strings.Trim(maybePort, "0123456789") == "" {
			domain = domain[:i]
		}
	}
	domain = strings.TrimLeft(strings.TrimSpace(domain), "*.")
	domain = strings.TrimRight(domain, ".")
	domain = strings.TrimPrefix(domain, "www.")
	return strings.TrimSpace(domain)
}

type BrowserAIManager struct {
	db       *gorm.DB
	migrated bool
	mu       sync.RWMutex
}

func NewBrowserAIManager(db *gorm.DB) *BrowserAIManager {
	m := &BrowserAIManager{db: db}
	if db != nil {
		_ = m.AutoMigrate(context.Background())
	}
	return m
}

func (m *BrowserAIManager) SetDB(db *gorm.DB) {
	m.mu.Lock()
	m.db = db
	needsMigrate := !m.migrated && db != nil
	m.mu.Unlock()

	if needsMigrate {
		_ = m.AutoMigrate(context.Background())
	}
}

func (m *BrowserAIManager) GetDB() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db
}

func (m *BrowserAIManager) AutoMigrate(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}

	err := m.db.WithContext(ctx).AutoMigrate(
		&BrowserAILog{},
		&BrowserAISearchLog{},
		&BrowserGuardRule{},
		&BrowserControlSettings{},
		&BrowserTargetWebsite{},
		&BrowserAIAgent{},
		&BrowserAIAgentSettings{},
		&BrowserGuardFleetConfig{},
		&BrowserAIWarningEmailLog{},
		&BrowserGuardRebuildLog{},
	)
	if err != nil {
		return err
	}

	// Seed default browser control settings if missing
	var ctrl BrowserControlSettings
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserControlSettingsID).First(&ctrl).Error; err != nil {
		_ = m.db.WithContext(ctx).Create(&BrowserControlSettings{
			ID:                  BrowserControlSettingsID,
			Enabled:             true,
			BlockUpload:         false,
			AttachmentRetention: "1h",
			UpdatedAt:           time.Now(),
		}).Error
	}

	var agentSettings BrowserAIAgentSettings
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserAIAgentSettingsID).First(&agentSettings).Error; err != nil {
		_ = m.db.WithContext(ctx).Create(&BrowserAIAgentSettings{
			ID:                  BrowserAIAgentSettingsID,
			RequireUninstallKey: true,
			UpdatedAt:           time.Now(),
		}).Error
	}

	var fleet BrowserGuardFleetConfig
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserGuardFleetConfigID).First(&fleet).Error; err != nil {
		_ = m.db.WithContext(ctx).Create(&BrowserGuardFleetConfig{
			ID:               BrowserGuardFleetConfigID,
			DefaultProxyAddr: defaultProxyAddrFromEnv(),
			PacAdvertiseAddr: defaultProxyAddrFromEnv(),
			ServerModePolicy: "endpoint_default",
			ListenHostPolicy: "127.0.0.1",
			PacSyncSeconds:   3,
			AgentTypeDefault: "endpoint",
			UpdatedAt:        time.Now(),
		}).Error
	}

	// Do NOT seed default guard rules.
	// Proxy applies only rules the admin adds in Guard Rules.

	// Do NOT seed default target websites.
	// Proxy only monitors domains the admin adds in Target Websites.

	m.migrated = true
	return nil
}

func (m *BrowserAIManager) GetLogs(ctx context.Context, platform, status, action, search string, limit, offset int) ([]BrowserAILog, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var logs []BrowserAILog
	var total int64

	if m.db == nil {
		return logs, 0, nil
	}

	query := m.db.WithContext(ctx).Model(&BrowserAILog{})

	if platform != "" && strings.ToLower(platform) != "all" {
		pLower := strings.ToLower(platform)
		query = query.Where("LOWER(platform) = ? OR LOWER(domain) = ?", pLower, pLower)
	}
	if status != "" && strings.ToLower(status) != "all" {
		if strings.ToLower(status) == "blocked" {
			query = query.Where("LOWER(action) = ?", "blocked")
		} else if strings.ToLower(status) == "allowed" {
			query = query.Where("LOWER(action) = ?", "allowed")
		} else {
			query = query.Where("LOWER(status) LIKE ?", "%"+strings.ToLower(status)+"%")
		}
	}
	if action != "" && strings.ToLower(action) != "all" {
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "siteblocked", "site_blocked", "site blocked":
			// Full-site lock logs (Target Websites → Block entire website), not DLP prompt blocks.
			query = query.Where(
				"LOWER(user_prompt_full) LIKE ? OR LOWER(user_prompt_preview) LIKE ? OR LOWER(rule_triggered) = ? OR LOWER(predicted_category) = ?",
				"%[site blocked]%",
				"%[site blocked]%",
				"block entire website",
				"site_block",
			)
		default:
			query = query.Where("LOWER(action) = ?", strings.ToLower(action))
		}
	}
	if search != "" {
		s := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(user_prompt_full) LIKE ? OR LOWER(platform) LIKE ? OR LOWER(domain) LIKE ? OR LOWER(client_ip) LIKE ? OR LOWER(rule_triggered) LIKE ? OR LOWER(agent_id) LIKE ? OR LOWER(agent_hostname) LIKE ?", s, s, s, s, s, s, s)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 50
	}

	err := query.Order("timestamp DESC").Limit(limit).Offset(offset).Find(&logs).Error
	return logs, total, err
}

// BrowserAILogStats are whole-table aggregates for the Browser AI overview cards.
type BrowserAILogStats struct {
	Total    int64 `json:"total"`
	Blocked  int64 `json:"blocked"`
	Warned   int64 `json:"warned"` // Redacted + Warned
	HighRisk int64 `json:"high_risk"`
	AvgRisk  int   `json:"avg_risk"`
}

// GetLogStats aggregates every prompt log (not just the current page) for the overview.
func (m *BrowserAIManager) GetLogStats(ctx context.Context) (BrowserAILogStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out BrowserAILogStats
	if m.db == nil {
		return out, nil
	}
	var row struct {
		Total    int64
		Blocked  int64
		Warned   int64
		HighRisk int64
		AvgRisk  *float64
	}
	err := m.db.WithContext(ctx).Model(&BrowserAILog{}).Select(`
		COUNT(*) AS total,
		COALESCE(SUM(CASE WHEN LOWER(action) = 'blocked' THEN 1 ELSE 0 END), 0) AS blocked,
		COALESCE(SUM(CASE WHEN LOWER(action) IN ('redacted', 'warned') THEN 1 ELSE 0 END), 0) AS warned,
		COALESCE(SUM(CASE WHEN risk_score >= 70 OR UPPER(predictive_risk) IN ('HIGH', 'CRITICAL') THEN 1 ELSE 0 END), 0) AS high_risk,
		AVG(CASE WHEN risk_score > 0 THEN risk_score ELSE 10 END) AS avg_risk`).Scan(&row).Error
	if err != nil {
		return out, err
	}
	out = BrowserAILogStats{Total: row.Total, Blocked: row.Blocked, Warned: row.Warned, HighRisk: row.HighRisk}
	if row.AvgRisk != nil && row.Total > 0 {
		out.AvgRisk = int(*row.AvgRisk + 0.5)
	}
	return out, nil
}

func (m *BrowserAIManager) ClearLogs(ctx context.Context) error {
	return m.ClearLogsInRange(ctx, nil, nil)
}

// ClearLogsInRange deletes prompt logs. If both since and until are nil, clears all.
// Otherwise deletes rows where timestamp >= since (if set) AND timestamp < until (if set).
func (m *BrowserAIManager) ClearLogsInRange(ctx context.Context, since, until *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}
	q := m.db.WithContext(ctx)
	if since == nil && until == nil {
		return q.Exec("DELETE FROM browser_ai_logs").Error
	}
	db := q.Model(&BrowserAILog{})
	if since != nil {
		db = db.Where("timestamp >= ?", *since)
	}
	if until != nil {
		db = db.Where("timestamp < ?", *until)
	}
	return db.Delete(&BrowserAILog{}).Error
}

func (m *BrowserAIManager) RecordSearchLog(ctx context.Context, entry *BrowserAISearchLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = entry.Timestamp
	}
	return m.db.WithContext(ctx).Create(entry).Error
}

func (m *BrowserAIManager) GetSearchLogs(ctx context.Context, engine, browser, isIncognito, search string, limit, offset int) ([]BrowserAISearchLog, int64, int64, int64, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var logs []BrowserAISearchLog
	var total int64
	var incognitoCount int64
	var queriesCount int64
	var clicksCount int64

	if m.db == nil {
		return logs, 0, 0, 0, 0, nil
	}

	// Overall KPI metrics (unfiltered totals)
	_ = m.db.WithContext(ctx).Model(&BrowserAISearchLog{}).Where("is_incognito = ?", true).Count(&incognitoCount).Error
	_ = m.db.WithContext(ctx).Model(&BrowserAISearchLog{}).Where("query != '' AND query IS NOT NULL").Count(&queriesCount).Error
	_ = m.db.WithContext(ctx).Model(&BrowserAISearchLog{}).Where("clicked_url != '' AND clicked_url IS NOT NULL").Count(&clicksCount).Error

	query := m.db.WithContext(ctx).Model(&BrowserAISearchLog{})

	if engine != "" && strings.ToLower(engine) != "all" {
		query = query.Where("LOWER(engine) LIKE ?", "%"+strings.ToLower(engine)+"%")
	}
	if browser != "" && strings.ToLower(browser) != "all" {
		query = query.Where("LOWER(browser) LIKE ?", "%"+strings.ToLower(browser)+"%")
	}
	if isIncognito == "true" {
		query = query.Where("is_incognito = ?", true)
	} else if isIncognito == "false" {
		query = query.Where("is_incognito = ?", false)
	}
	if search != "" {
		s := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(query) LIKE ? OR LOWER(clicked_url) LIKE ? OR LOWER(clicked_title) LIKE ? OR LOWER(agent_hostname) LIKE ? OR LOWER(client_ip) LIKE ?", s, s, s, s, s)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, 0, 0, 0, err
	}

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	err := query.Order("timestamp DESC").Limit(limit).Offset(offset).Find(&logs).Error
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	if logs == nil {
		logs = []BrowserAISearchLog{}
	}
	return logs, total, incognitoCount, queriesCount, clicksCount, nil
}

func (m *BrowserAIManager) ClearSearchLogs(ctx context.Context) error {
	return m.ClearSearchLogsInRange(ctx, nil, nil)
}

// ClearSearchLogsInRange deletes search logs. If both since and until are nil, clears all.
// Otherwise deletes rows where timestamp >= since (if set) AND timestamp < until (if set).
func (m *BrowserAIManager) ClearSearchLogsInRange(ctx context.Context, since, until *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}
	q := m.db.WithContext(ctx)
	if since == nil && until == nil {
		return q.Exec("DELETE FROM browser_ai_search_logs").Error
	}
	db := q.Model(&BrowserAISearchLog{})
	if since != nil {
		db = db.Where("timestamp >= ?", *since)
	}
	if until != nil {
		db = db.Where("timestamp < ?", *until)
	}
	return db.Delete(&BrowserAISearchLog{}).Error
}

// DeleteLogsByIDs deletes the given prompt log rows and returns how many were removed.
func (m *BrowserAIManager) DeleteLogsByIDs(ctx context.Context, ids []string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || len(ids) == 0 {
		return 0, nil
	}
	res := m.db.WithContext(ctx).Where("id IN ?", ids).Delete(&BrowserAILog{})
	return res.RowsAffected, res.Error
}

// DeleteSearchLogsByIDs deletes the given search log rows and returns how many were removed.
func (m *BrowserAIManager) DeleteSearchLogsByIDs(ctx context.Context, ids []string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || len(ids) == 0 {
		return 0, nil
	}
	res := m.db.WithContext(ctx).Where("id IN ?", ids).Delete(&BrowserAISearchLog{})
	return res.RowsAffected, res.Error
}

// ParseAttachmentRetentionDuration maps attachment retention ("10m", "1h", "2h", "1d", "7d") to time.Duration.
func ParseAttachmentRetentionDuration(retention string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(retention)) {
	case "10m":
		return 10 * time.Minute
	case "1h":
		return 1 * time.Hour
	case "2h":
		return 2 * time.Hour
	case "1d":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return 1 * time.Hour
	}
}

func parseRetentionDuration(retention string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(retention)) {
	case "1d":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "90d":
		return 90 * 24 * time.Hour
	case "180d":
		return 180 * 24 * time.Hour
	case "365d":
		return 365 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

// ApplySearchLogAutoDelete purges search logs older than the configured retention
// when search_log_auto_delete is enabled. Returns the cutoff used, or nil if disabled.
func (m *BrowserAIManager) ApplySearchLogAutoDelete(ctx context.Context) *time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}
	var ctrl BrowserControlSettings
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserControlSettingsID).First(&ctrl).Error; err != nil {
		return nil
	}
	if !ctrl.SearchLogAutoDelete {
		return nil
	}
	age := parseRetentionDuration(ctrl.SearchLogRetention)
	cutoff := time.Now().Add(-age)
	_ = m.db.WithContext(ctx).Where("timestamp < ?", cutoff).Delete(&BrowserAISearchLog{}).Error
	return &cutoff
}

// ApplyPromptLogAutoDelete purges prompt logs older than the configured retention
// when prompt_log_auto_delete is enabled. Returns the cutoff used, or nil if disabled.
func (m *BrowserAIManager) ApplyPromptLogAutoDelete(ctx context.Context) *time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}
	var ctrl BrowserControlSettings
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserControlSettingsID).First(&ctrl).Error; err != nil {
		return nil
	}
	if !ctrl.PromptLogAutoDelete {
		return nil
	}
	age := parseRetentionDuration(ctrl.PromptLogRetention)
	cutoff := time.Now().Add(-age)
	_ = m.db.WithContext(ctx).Where("timestamp < ?", cutoff).Delete(&BrowserAILog{}).Error
	return &cutoff
}

func (m *BrowserAIManager) GetRules(ctx context.Context) ([]BrowserGuardRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var rules []BrowserGuardRule
	if m.db == nil {
		return rules, nil
	}
	err := m.db.WithContext(ctx).Order("created_at ASC").Find(&rules).Error
	if err != nil {
		return rules, err
	}
	// Keep WARN as WARN (do not rewrite to REDACT). Normalize severity too.
	for i := range rules {
		rules[i].Action = NormalizeGuardRuleAction(rules[i].Action)
		rules[i].Severity = NormalizeGuardRuleSeverity(rules[i].Severity)
	}
	return rules, nil
}

func (m *BrowserAIManager) CreateRule(ctx context.Context, rule *BrowserGuardRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if rule.ID == "" {
		rule.ID = "rule-" + uuid.New().String()[:8]
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.RuleType == "" {
		rule.RuleType = "regex"
	}
	rule.RuleType = strings.TrimSpace(rule.RuleType)
	rule.BotProvider = strings.TrimSpace(rule.BotProvider)
	rule.BotModel = strings.TrimSpace(rule.BotModel)
	rule.BotPrompt = strings.TrimSpace(rule.BotPrompt)
	rule.BotReferenceImage = strings.TrimSpace(rule.BotReferenceImage)
	rule.BotReferenceImageType = strings.TrimSpace(rule.BotReferenceImageType)
	rule.Pattern = strings.TrimSpace(rule.Pattern)
	rule.Description = strings.TrimSpace(rule.Description)
	rule.WarningMessage = strings.TrimSpace(rule.WarningMessage)
	rule.Action = NormalizeGuardRuleAction(rule.Action)
	rule.Severity = NormalizeGuardRuleSeverity(rule.Severity)
	rule.CreatedAt = time.Now()
	return m.db.WithContext(ctx).Create(rule).Error
}

func (m *BrowserAIManager) UpdateRule(ctx context.Context, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}
	allowed := map[string]bool{
		"name": true, "severity": true, "action": true, "pattern": true,
		"active": true, "description": true, "warning_message": true,
		"rule_type": true, "bot_provider": true, "bot_model": true, "bot_prompt": true,
		"bot_reference_image": true, "bot_reference_image_type": true,
	}
	filtered := make(map[string]any, len(updates))
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		if s, ok := v.(string); ok && (k == "name" || k == "pattern" || k == "description" || k == "warning_message" || k == "severity" || k == "action" || k == "rule_type" || k == "bot_provider" || k == "bot_model" || k == "bot_prompt" || k == "bot_reference_image" || k == "bot_reference_image_type") {
			if k == "action" {
				filtered[k] = NormalizeGuardRuleAction(s)
			} else if k == "severity" {
				filtered[k] = NormalizeGuardRuleSeverity(s)
			} else {
				filtered[k] = strings.TrimSpace(s)
			}
			continue
		}
		filtered[k] = v
	}
	if len(filtered) == 0 {
		return nil
	}
	return m.db.WithContext(ctx).Model(&BrowserGuardRule{}).Where("id = ?", id).Updates(filtered).Error
}

func (m *BrowserAIManager) UpdateLogRuleViolation(ctx context.Context, id, action, status, ruleTriggered string, riskScore int, predictiveRisk, predictedCategory string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil
	}
	return m.db.WithContext(ctx).Model(&BrowserAILog{}).Where("id = ?", id).Updates(map[string]any{
		"action":             action,
		"status":             status,
		"rule_triggered":     ruleTriggered,
		"risk_score":         riskScore,
		"predictive_risk":    predictiveRisk,
		"predicted_category": predictedCategory,
	}).Error
}

func (m *BrowserAIManager) DeleteRule(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return m.db.WithContext(ctx).Where("id = ?", id).Delete(&BrowserGuardRule{}).Error
}

func (m *BrowserAIManager) GetControls(ctx context.Context) (*BrowserControlSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return &BrowserControlSettings{
			ID:                  BrowserControlSettingsID,
			Enabled:             true,
			BlockUpload:         false,
			AttachmentRetention: "1h",
			SearchLogAutoDelete: false,
			SearchLogRetention:  "7d",
			PromptLogAutoDelete: false,
			PromptLogRetention:  "7d",
			UpdatedAt:           time.Now(),
		}, nil
	}
	var ctrl BrowserControlSettings
	err := m.db.WithContext(ctx).Where("id = ?", BrowserControlSettingsID).First(&ctrl).Error
	if err != nil {
		ctrl = BrowserControlSettings{
			ID:                  BrowserControlSettingsID,
			Enabled:             true,
			BlockUpload:         false,
			AttachmentRetention: "1h",
			SearchLogAutoDelete: false,
			SearchLogRetention:  "7d",
			PromptLogAutoDelete: false,
			PromptLogRetention:  "7d",
			UpdatedAt:           time.Now(),
		}
		if createErr := m.db.WithContext(ctx).Create(&ctrl).Error; createErr != nil {
			return &ctrl, createErr
		}
	}
	if ctrl.AttachmentRetention == "" {
		ctrl.AttachmentRetention = "1h"
	}
	if ctrl.SearchLogRetention == "" {
		ctrl.SearchLogRetention = "7d"
	}
	if ctrl.PromptLogRetention == "" {
		ctrl.PromptLogRetention = "7d"
	}
	return &ctrl, nil
}

func (m *BrowserAIManager) UpdateControls(ctx context.Context, updates map[string]any) (*BrowserControlSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var ctrl BrowserControlSettings
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserControlSettingsID).First(&ctrl).Error; err != nil {
		ctrl = BrowserControlSettings{
			ID:                  BrowserControlSettingsID,
			Enabled:             true,
			BlockUpload:         false,
			AttachmentRetention: "1h",
			SearchLogAutoDelete: false,
			SearchLogRetention:  "7d",
			PromptLogAutoDelete: false,
			PromptLogRetention:  "7d",
			UpdatedAt:           time.Now(),
		}
		if createErr := m.db.WithContext(ctx).Create(&ctrl).Error; createErr != nil {
			return nil, createErr
		}
	}

	allowed := map[string]bool{
		"enabled":                true,
		"block_upload":           true,
		"upload_warning":         true,
		"attachment_retention":   true,
		"search_log_auto_delete": true,
		"search_log_retention":   true,
		"prompt_log_auto_delete": true,
		"prompt_log_retention":   true,
	}
	filtered := map[string]any{}
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		if k == "upload_warning" {
			if s, ok := v.(string); ok {
				filtered[k] = strings.TrimSpace(s)
				continue
			}
		}
		if k == "attachment_retention" {
			if s, ok := v.(string); ok {
				s = strings.ToLower(strings.TrimSpace(s))
				switch s {
				case "10m", "1h", "2h", "1d", "7d":
					filtered[k] = s
				default:
					filtered[k] = "1h"
				}
				continue
			}
		}
		if k == "search_log_retention" || k == "prompt_log_retention" {
			if s, ok := v.(string); ok {
				s = strings.ToLower(strings.TrimSpace(s))
				switch s {
				case "1d", "7d", "30d", "90d", "180d", "365d":
					filtered[k] = s
				default:
					filtered[k] = "7d"
				}
				continue
			}
		}
		filtered[k] = v
	}
	filtered["updated_at"] = time.Now()

	if err := m.db.WithContext(ctx).Model(&BrowserControlSettings{}).Where("id = ?", BrowserControlSettingsID).Updates(filtered).Error; err != nil {
		return nil, err
	}
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserControlSettingsID).First(&ctrl).Error; err != nil {
		return nil, err
	}
	if ctrl.AttachmentRetention == "" {
		ctrl.AttachmentRetention = "1h"
	}
	if ctrl.SearchLogRetention == "" {
		ctrl.SearchLogRetention = "7d"
	}
	if ctrl.PromptLogRetention == "" {
		ctrl.PromptLogRetention = "7d"
	}
	return &ctrl, nil
}

func (m *BrowserAIManager) GetTargets(ctx context.Context) ([]BrowserTargetWebsite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var targets []BrowserTargetWebsite
	if m.db == nil {
		return targets, nil
	}
	if err := m.db.WithContext(ctx).Order("domain ASC").Find(&targets).Error; err != nil {
		return targets, err
	}
	// Heal status vs block_site/monitored drift (children could keep BLOCKED after Block was turned off).
	for i := range targets {
		t := &targets[i]
		want := "MONITORED"
		if t.BlockSite {
			want = "BLOCKED"
		} else if !t.Monitored {
			want = "PAUSED"
		}
		if t.Status != want {
			t.Status = want
			_ = m.db.WithContext(ctx).Model(&BrowserTargetWebsite{}).Where("id = ?", t.ID).Update("status", want).Error
		}
	}
	return targets, nil
}

func sanitizeProxyAddr(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > 80 || !pacProxyAddrSafe.MatchString(s) {
		return defaultProxyAddrFromEnv()
	}
	return s
}

// searchEnginePACRule routes search-engine result hosts through Guard so Search Logs
// can capture typed queries and result clicks. Guard runs no prompt/DLP rules on them.
// Keep in sync with SEARCH_ENGINE_PAC_RULE in apps/browser-guard/agent/agent_pac_content.py.
func searchEnginePACRule(proxyAddr string) string {
	return `    if (shExpMatch(host, "google.*") || shExpMatch(host, "www.google.*") ||
        host === "bing.com" || host === "www.bing.com" ||
        host === "duckduckgo.com" || host === "html.duckduckgo.com" ||
        dnsDomainIs(host, "search.yahoo.com") || host === "search.brave.com") {
        return "PROXY ` + proxyAddr + `";
    }

`
}

func emptyPAC(proxyAddr string) string {
	return `// Raksha Browser AI Guard — no Target Websites yet (search engines only, for Search Logs).
function FindProxyForURL(url, host) {
    host = host.toLowerCase();

` + searchEnginePACRule(proxyAddr) + `    return "DIRECT";
}
`
}

// minimizePACHosts drops hosts already covered by a parent in the same list.
// Example: example.com + www.example.com → keep example.com only (subdomain match).
// No product hardcoding — purely structural. Cuts PAC size for 1000+ target rows.
func minimizePACHosts(hosts []string) []string {
	if len(hosts) <= 1 {
		return hosts
	}
	set := map[string]bool{}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			set[h] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		covered := false
		parts := strings.Split(h, ".")
		for i := 1; i < len(parts); i++ {
			parent := strings.Join(parts[i:], ".")
			if set[parent] {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

func buildDomainPAC(hosts []string, proxyAddr string) string {
	hosts = minimizePACHosts(hosts)
	var b strings.Builder
	b.WriteString("// Raksha Browser AI Guard — admin Target Websites from dashboard only.\n")
	b.WriteString("// Parent domains only when children are covered by subdomain match. No hardcoded products.\n")
	b.WriteString("function FindProxyForURL(url, host) {\n")
	b.WriteString("    host = host.toLowerCase();\n\n")
	b.WriteString(searchEnginePACRule(proxyAddr))
	b.WriteString("    var aiHosts = [\n")
	for _, d := range hosts {
		b.WriteString("        \"")
		b.WriteString(d)
		b.WriteString("\",\n")
	}
	b.WriteString("    ];\n\n")
	b.WriteString("    for (var i = 0; i < aiHosts.length; i++) {\n")
	b.WriteString("        var d = aiHosts[i];\n")
	b.WriteString("        if (host === d || dnsDomainIs(host, \".\" + d) || shExpMatch(host, \"*.\" + d)) {\n")
	b.WriteString("            // Strict: monitored hosts use Guard only. Agent fail-opens to all-DIRECT if proxy is down.\n")
	b.WriteString("            return \"PROXY ")
	b.WriteString(proxyAddr)
	b.WriteString("\";\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
	b.WriteString("    return \"DIRECT\";\n")
	b.WriteString("}\n")
	return b.String()
}

// BuildProxyPAC builds a PAC script from Target Websites that are monitored
// and/or fully blocked (block_site). Empty list means all traffic is DIRECT.
// Never returns an error — a valid PAC is always produced so employee browsers
// are not left without a proxy config (a 500 here used to break Guard).
func (m *BrowserAIManager) BuildProxyPAC(ctx context.Context, proxyAddr string) (string, error) {
	proxyAddr = sanitizeProxyAddr(proxyAddr)
	var targets []BrowserTargetWebsite
	if m != nil && m.db != nil {
		dbCtx := ctx
		if dbCtx == nil {
			dbCtx = context.Background()
		}
		if err := m.db.WithContext(dbCtx).Order("domain ASC").Find(&targets).Error; err != nil {
			return emptyPAC(proxyAddr), nil
		}
	}

	seen := map[string]bool{}
	var hosts []string
	for _, t := range targets {
		// Must route through local Guard proxy to monitor prompts OR lock the whole site.
		if !t.Monitored && !t.BlockSite {
			continue
		}
		d := NormalizeDomain(t.Domain)
		if d == "" || seen[d] {
			continue
		}
		if !pacHostSafe.MatchString(d) {
			continue
		}
		seen[d] = true
		hosts = append(hosts, d)
	}

	sort.Strings(hosts)

	if len(hosts) == 0 {
		return emptyPAC(proxyAddr), nil
	}

	return buildDomainPAC(hosts, proxyAddr), nil
}

// GetTargetByDomain finds a monitored target matching host or parent domain.
func (m *BrowserAIManager) GetTargetByDomain(ctx context.Context, host string) (*BrowserTargetWebsite, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return nil, nil
	}
	host = NormalizeDomain(host)
	if host == "" {
		return nil, nil
	}
	var targets []BrowserTargetWebsite
	if err := m.db.WithContext(ctx).Find(&targets).Error; err != nil {
		return nil, err
	}
	for i := range targets {
		d := strings.ToLower(strings.TrimSpace(targets[i].Domain))
		if d == "" {
			continue
		}
		if host == d || strings.HasSuffix(host, "."+d) {
			t := targets[i]
			return &t, nil
		}
	}
	return nil, nil
}

func (m *BrowserAIManager) CreateTarget(ctx context.Context, target *BrowserTargetWebsite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}
	target.Domain = NormalizeDomain(target.Domain)
	if target.Domain == "" {
		return fmt.Errorf("invalid target domain")
	}
	target.HostRole = NormalizeHostRole(target.HostRole)
	if strings.TrimSpace(target.PlatformName) == "" {
		target.PlatformName = target.Domain
	}
	if target.ID == "" {
		target.ID = "tgt-" + uuid.New().String()[:8]
	}
	parentMonitored := true
	parentBlockSite := false
	target.ParentID = strings.TrimSpace(target.ParentID)
	if target.ParentID != "" {
		if target.ParentID == target.ID {
			target.ParentID = ""
		} else {
			var parent BrowserTargetWebsite
			if err := m.db.WithContext(ctx).Where("id = ?", target.ParentID).First(&parent).Error; err != nil {
				target.ParentID = ""
			} else if strings.TrimSpace(parent.ParentID) != "" {
				target.ParentID = parent.ParentID
				parentMonitored = parent.Monitored
				parentBlockSite = parent.BlockSite
			} else {
				target.ParentID = parent.ID
				parentMonitored = parent.Monitored
				parentBlockSite = parent.BlockSite
			}
		}
	}
	if target.ParentID != "" {
		target.Monitored = parentMonitored
		if parentBlockSite {
			target.BlockSite = true
		}
	} else {
		target.Monitored = true
	}
	if target.BlockSite {
		target.Status = "BLOCKED"
	} else if !target.Monitored {
		target.Status = "PAUSED"
	} else {
		target.Status = "MONITORED"
	}
	target.CreatedAt = time.Now()
	if err := m.db.WithContext(ctx).Create(target).Error; err != nil {
		return err
	}
	return nil
}

func (m *BrowserAIManager) UpdateTarget(ctx context.Context, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if raw, ok := updates["domain"]; ok {
		if s, ok := raw.(string); ok {
			updates["domain"] = NormalizeDomain(s)
		}
	}
	if raw, ok := updates["host_role"]; ok {
		if s, ok := raw.(string); ok {
			updates["host_role"] = NormalizeHostRole(s)
		}
	}
	allowed := map[string]bool{
		"domain":             true,
		"platform_name":      true,
		"monitored":          true,
		"block_site":         true,
		"status":             true,
		"reply_bot_enabled":  true,
		"reply_bot_provider": true,
		"reply_bot_model":    true,
		"reply_bot_mode":     true,
		"parent_id":          true,
		"host_role":          true,
	}
	filtered := map[string]any{}
	for k, v := range updates {
		if allowed[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if rawParent, ok := filtered["parent_id"].(string); ok {
		pid := strings.TrimSpace(rawParent)
		if pid == "" || pid == id {
			delete(filtered, "parent_id")
		} else {
			var parent BrowserTargetWebsite
			if err := m.db.WithContext(ctx).Where("id = ?", pid).First(&parent).Error; err != nil {
				delete(filtered, "parent_id")
			} else if strings.TrimSpace(parent.ParentID) != "" {
				filtered["parent_id"] = parent.ParentID
			} else {
				filtered["parent_id"] = parent.ID
			}
		}
		if len(filtered) == 0 {
			return nil
		}
	}
	// Keep status aligned with block_site when that flag is updated alone.
	if bs, ok := filtered["block_site"].(bool); ok {
		if _, hasStatus := filtered["status"]; !hasStatus {
			if bs {
				filtered["status"] = "BLOCKED"
			} else if mon, ok := filtered["monitored"].(bool); ok && !mon {
				filtered["status"] = "PAUSED"
			} else {
				filtered["status"] = "MONITORED"
			}
		}
	}
	if err := m.db.WithContext(ctx).Model(&BrowserTargetWebsite{}).Where("id = ?", id).Updates(filtered).Error; err != nil {
		return err
	}
	child := map[string]any{}
	if mon, ok := filtered["monitored"].(bool); ok {
		child["monitored"] = mon
		if st, ok := filtered["status"].(string); ok && st != "" {
			child["status"] = st
		} else if mon {
			child["status"] = "MONITORED"
		} else {
			child["status"] = "PAUSED"
		}
	}
	if bs, ok := filtered["block_site"].(bool); ok {
		child["block_site"] = bs
		if bs {
			child["status"] = "BLOCKED"
		} else if _, hasStatus := child["status"]; !hasStatus {
			// Turning Block off must clear stale BLOCKED status on children.
			child["status"] = "MONITORED"
		} else if st, _ := child["status"].(string); strings.EqualFold(st, "BLOCKED") {
			if mon, ok := child["monitored"].(bool); ok && !mon {
				child["status"] = "PAUSED"
			} else {
				child["status"] = "MONITORED"
			}
		}
	}
	if len(child) > 0 {
		_ = m.db.WithContext(ctx).Model(&BrowserTargetWebsite{}).Where("parent_id = ?", id).Updates(child).Error
	}
	return nil
}

func (m *BrowserAIManager) DeleteTarget(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return m.db.WithContext(ctx).Where("id = ? OR parent_id = ?", id, id).Delete(&BrowserTargetWebsite{}).Error
}

// NormalizeHostRole keeps only supported admin labels.
func NormalizeHostRole(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ui", "main", "main_ui":
		return "ui"
	case "chat", "chat_domain":
		return "chat"
	case "file", "file_domain", "upload":
		return "file"
	default:
		return ""
	}
}

func (m *BrowserAIManager) InterceptPrompt(ctx context.Context, platform, promptFull, clientIP string, metadata map[string]any) (*BrowserAILog, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		return nil, "", fmt.Errorf("database connection not available")
	}

	// Do not persist Cursor/IDE binary decode fragments as Prompt Logs.
	if looksLikeBinaryOrWireGarbage(promptFull) &&
		!strings.HasPrefix(strings.TrimSpace(promptFull), "[FILE UPLOAD]") &&
		!strings.HasPrefix(strings.TrimSpace(promptFull), "[VOICE UPLOAD]") &&
		!strings.HasPrefix(strings.TrimSpace(promptFull), "[SITE BLOCKED") {
		return &BrowserAILog{
			ID:                uuid.New().String(),
			Timestamp:         time.Now(),
			Platform:          platform,
			UserPromptPreview: "",
			UserPromptFull:    "",
			EstTokens:         0,
			ClientIP:          clientIP,
			Status:            "Skipped (opaque wire)",
			Action:            "Allowed",
			RiskScore:         0,
			PredictiveRisk:    "LOW",
			PredictedCategory: "SAFE",
			CreatedAt:         time.Now(),
		}, "", nil
	}

	words := strings.Fields(promptFull)
	estTokens := int(float64(len(words)) * 1.3)
	if estTokens == 0 && len(promptFull) > 0 {
		estTokens = 1
	}

	preview := promptFull
	if len(preview) > 100 {
		preview = preview[:97] + "..."
	}

	var rules []BrowserGuardRule
	_ = m.db.WithContext(ctx).Where("active = ?", true).Find(&rules).Error

	action := "Allowed"
	status := "Allowed"
	ruleTriggered := ""
	matchedWarning := ""
	riskScore := 10
	predictiveRisk := "LOW"
	predictedCategory := "SAFE"

	if isBlocked, ok := metadata["is_blocked"].(bool); ok && isBlocked {
		action = "Blocked"
		if blockedReason, ok := metadata["blocked_reason"].(string); ok && blockedReason != "" {
			status = fmt.Sprintf("Blocked (%s)", blockedReason)
			ruleTriggered = blockedReason
			if strings.EqualFold(strings.TrimSpace(blockedReason), "Block Entire Website") {
				predictedCategory = "SITE_BLOCK"
			}
		} else if statusStr, ok := metadata["status"].(string); ok && statusStr != "" {
			status = statusStr
		} else {
			status = "Blocked (Security Violation)"
		}
		riskScore = 95
		predictiveRisk = "CRITICAL"
		if predictedCategory == "SAFE" {
			predictedCategory = "SECURITY_POLICY_VIOLATION"
		}
	}

	if estTokens > 500 {
		riskScore += 15
	}

	sort.SliceStable(rules, func(i, j int) bool {
		// BLOCK before REDACT before WARN — admin rules only.
		rank := func(action string) int {
			switch NormalizeGuardRuleAction(action) {
			case "BLOCK":
				return 0
			case "REDACT":
				return 1
			case "WARN":
				return 2
			default:
				return 3
			}
		}
		return rank(rules[i].Action) < rank(rules[j].Action)
	})

	// Proxy already decided Blocked (upload / site lock) — do not let a REDACT rule downgrade it.
	alreadyBlocked := action == "Blocked"
	uploadScan := false
	if v, ok := metadata["upload_scan"].(bool); ok && v {
		uploadScan = true
	}
	if strings.HasPrefix(strings.TrimSpace(promptFull), "[FILE UPLOAD]") ||
		strings.HasPrefix(strings.TrimSpace(promptFull), "[VOICE UPLOAD]") {
		uploadScan = true
	}
	regexScanText := promptFull
	if uploadScan {
		if ext, ok := metadata["extracted_text"].(string); ok && strings.TrimSpace(ext) != "" {
			regexScanText = strings.TrimSpace(ext)
		} else {
			regexScanText = ""
		}
	}

	for _, rule := range rules {
		if alreadyBlocked {
			break
		}
		if uploadScan && regexScanText == "" {
			break
		}
		if strings.ToLower(rule.RuleType) == "ai_bot" || rule.Pattern == "" {
			continue
		}
		re, err := CompileGuardRegex(rule.Pattern)
		if err != nil || !re.MatchString(regexScanText) {
			continue
		}
		ruleTriggered = rule.Name
		matchedWarning = strings.TrimSpace(rule.WarningMessage)
		ruleAction := NormalizeGuardRuleAction(rule.Action)
		sevScore, sevLabel := guardSeverityScore(rule.Severity)

		if ruleAction == "BLOCK" {
			action = "Blocked"
			status = fmt.Sprintf("Blocked (%s)", rule.Name)
			riskScore = sevScore
			if riskScore < 70 {
				riskScore = 85
			}
			predictiveRisk = sevLabel
			if predictiveRisk == "LOW" || predictiveRisk == "MEDIUM" {
				predictiveRisk = "HIGH"
			}
			predictedCategory = "SECURITY_POLICY_VIOLATION"
			break
		} else if ruleAction == "REDACT" {
			// Log keeps the real prompt; ChatGPT gets prompt + redact notice via forward_prompt on the API.
			action = "Redacted"
			status = fmt.Sprintf("Redacted (%s)", rule.Name)
			riskScore = sevScore
			if riskScore > 70 {
				riskScore = 60
			}
			if riskScore < 40 {
				riskScore = 45
			}
			predictiveRisk = "MEDIUM"
			if sevLabel == "CRITICAL" || sevLabel == "HIGH" {
				predictiveRisk = "HIGH"
			}
			predictedCategory = "SUSPICIOUS_CONTENT"
			break
		} else if ruleAction == "WARN" {
			// Allow send; append warning notice only (no block, no redact label).
			action = "Warned"
			status = fmt.Sprintf("Warned (%s)", rule.Name)
			riskScore = sevScore
			if riskScore > 50 {
				riskScore = 40
			}
			if riskScore < 20 {
				riskScore = 25
			}
			predictiveRisk = "LOW"
			if sevLabel == "CRITICAL" || sevLabel == "HIGH" {
				predictiveRisk = "MEDIUM"
			}
			predictedCategory = "SUSPICIOUS_CONTENT"
			break
		}
	}

	// No built-in / hardcoded DLP patterns — only admin-added regex/bot rules above.

	if action == "Allowed" && ruleTriggered == "" && (status == "Allowed" || status == "") {
		status = "Allowed (no guard rule matched)"
		predictedCategory = "SAFE"
	}

	// Long unmatched prompts bump to MEDIUM; proxy blocks keep their CRITICAL label.
	if ruleTriggered == "" && action == "Allowed" && riskScore > 20 {
		predictiveRisk = "MEDIUM"
	}

	preview = promptFull
	if len(preview) > 100 {
		preview = preview[:97] + "..."
	}

	metaBytes, _ := json.Marshal(metadata)

	agentID := ""
	agentHostname := ""
	domain := ""
	if v, ok := metadata["agent_id"].(string); ok {
		agentID = strings.TrimSpace(v)
	}
	if v, ok := metadata["agent_hostname"].(string); ok {
		agentHostname = strings.TrimSpace(v)
	}
	if v, ok := metadata["domain"].(string); ok {
		domain = NormalizeDomain(v)
	}
	attachmentName := ""
	if v, ok := metadata["file_name"].(string); ok {
		attachmentName = strings.TrimSpace(v)
	}

	logEntry := BrowserAILog{
		ID:                uuid.New().String(),
		Timestamp:         time.Now(),
		Platform:          platform,
		Domain:            domain,
		UserPromptPreview: preview,
		UserPromptFull:    promptFull,
		EstTokens:         estTokens,
		ClientIP:          clientIP,
		AgentID:           agentID,
		AgentHostname:     agentHostname,
		Status:            status,
		Action:            action,
		RuleTriggered:     ruleTriggered,
		RiskScore:         riskScore,
		PredictiveRisk:    predictiveRisk,
		PredictedCategory: predictedCategory,
		AttachmentName:    attachmentName,
		Metadata:          string(metaBytes),
		CreatedAt:         time.Now(),
	}

	if err := m.db.WithContext(ctx).Create(&logEntry).Error; err != nil {
		return nil, "", err
	}

	if domain != "" {
		m.db.WithContext(ctx).Model(&BrowserTargetWebsite{}).Where("domain = ?", domain).UpdateColumn("intercepted_count", gorm.Expr("intercepted_count + 1"))
	}

	return &logEntry, matchedWarning, nil
}

// UpdateLogAttachment links a stored upload file to an intercept log.
// When storedName is empty, only attachment_name / content_type are updated (permanent name).
func (m *BrowserAIManager) UpdateLogAttachment(ctx context.Context, logID, name, storedName, contentType string) error {
	return m.UpdateLogAttachmentMeta(ctx, logID, name, storedName, contentType, 0, "", nil)
}

// UpdateLogAttachmentMeta persists attachment index fields in Postgres (bytes stay on disk).
func (m *BrowserAIManager) UpdateLogAttachmentMeta(ctx context.Context, logID, name, storedName, contentType string, sizeBytes int64, relPath string, expiresAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || strings.TrimSpace(logID) == "" {
		return nil
	}
	updates := map[string]any{}
	if n := strings.TrimSpace(name); n != "" {
		updates["attachment_name"] = n
	}
	if s := strings.TrimSpace(storedName); s != "" {
		updates["attachment_stored_name"] = s
	}
	if c := strings.TrimSpace(contentType); c != "" {
		updates["attachment_content_type"] = c
	}
	if sizeBytes > 0 {
		updates["attachment_size_bytes"] = sizeBytes
	}
	if p := strings.TrimSpace(relPath); p != "" {
		updates["attachment_path"] = p
	}
	if expiresAt != nil {
		updates["attachment_expires_at"] = *expiresAt
	}
	if len(updates) == 0 {
		return nil
	}
	return m.db.WithContext(ctx).Model(&BrowserAILog{}).Where("id = ?", logID).Updates(updates).Error
}

// GetFleetConfig returns the singleton Guard fleet defaults (creates seed if missing).
func (m *BrowserAIManager) GetFleetConfig(ctx context.Context) (*BrowserGuardFleetConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return defaultFleetConfig(), nil
	}
	var row BrowserGuardFleetConfig
	err := m.db.WithContext(ctx).Where("id = ?", BrowserGuardFleetConfigID).First(&row).Error
	if err != nil {
		seed := defaultFleetConfig()
		_ = m.db.WithContext(ctx).Create(seed).Error
		return seed, nil
	}
	return &row, nil
}

// SaveFleetConfig upserts company Guard fleet defaults into Postgres.
func (m *BrowserAIManager) SaveFleetConfig(ctx context.Context, incoming *BrowserGuardFleetConfig) (*BrowserGuardFleetConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if incoming == nil {
		return nil, fmt.Errorf("fleet config is required")
	}
	now := time.Now()
	row := BrowserGuardFleetConfig{
		ID:               BrowserGuardFleetConfigID,
		DefaultProxyAddr: strings.TrimSpace(incoming.DefaultProxyAddr),
		PacAdvertiseAddr: strings.TrimSpace(incoming.PacAdvertiseAddr),
		ServerModePolicy: strings.TrimSpace(incoming.ServerModePolicy),
		ListenHostPolicy: strings.TrimSpace(incoming.ListenHostPolicy),
		PacSyncSeconds:   incoming.PacSyncSeconds,
		AgentTypeDefault: strings.TrimSpace(incoming.AgentTypeDefault),
		BackendURLHint:   strings.TrimSpace(incoming.BackendURLHint),
		Notes:            strings.TrimSpace(incoming.Notes),
		UpdatedAt:        now,
		UpdatedBy:        strings.TrimSpace(incoming.UpdatedBy),
	}
	if row.DefaultProxyAddr == "" {
		row.DefaultProxyAddr = defaultProxyAddrFromEnv()
	}
	if row.PacAdvertiseAddr == "" {
		row.PacAdvertiseAddr = row.DefaultProxyAddr
	}
	if row.ServerModePolicy == "" {
		row.ServerModePolicy = "endpoint_default"
	}
	if row.ListenHostPolicy == "" {
		row.ListenHostPolicy = "127.0.0.1"
	}
	if row.PacSyncSeconds <= 0 {
		row.PacSyncSeconds = 3
	}
	if row.PacSyncSeconds > 600 {
		row.PacSyncSeconds = 600
	}
	switch strings.ToLower(row.AgentTypeDefault) {
	case "network", "server":
		row.AgentTypeDefault = "network"
	default:
		row.AgentTypeDefault = "endpoint"
	}
	if err := m.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func defaultFleetConfig() *BrowserGuardFleetConfig {
	return &BrowserGuardFleetConfig{
		ID:               BrowserGuardFleetConfigID,
		DefaultProxyAddr: defaultProxyAddrFromEnv(),
		PacAdvertiseAddr: defaultProxyAddrFromEnv(),
		ServerModePolicy: "endpoint_default",
		ListenHostPolicy: "127.0.0.1",
		PacSyncSeconds:   3,
		AgentTypeDefault: "endpoint",
		UpdatedAt:        time.Now(),
	}
}

// ClearLogAttachmentFile removes the temp disk pointer only — keeps attachment_name forever.
func (m *BrowserAIManager) ClearLogAttachmentFile(ctx context.Context, logID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || strings.TrimSpace(logID) == "" {
		return nil
	}
	return m.db.WithContext(ctx).Model(&BrowserAILog{}).Where("id = ?", logID).Updates(map[string]any{
		"attachment_stored_name": "",
	}).Error
}

// ClearLogAttachmentFileByStoredName clears stored file refs that match a disk basename.
func (m *BrowserAIManager) ClearLogAttachmentFileByStoredName(ctx context.Context, storedName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	storedName = strings.TrimSpace(storedName)
	if m.db == nil || storedName == "" {
		return nil
	}
	return m.db.WithContext(ctx).Model(&BrowserAILog{}).Where("attachment_stored_name = ?", storedName).Updates(map[string]any{
		"attachment_stored_name": "",
	}).Error
}

// GetLogByID returns one intercept log by id.
func (m *BrowserAIManager) GetLogByID(ctx context.Context, id string) (*BrowserAILog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil || strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("log not found")
	}
	var log BrowserAILog
	if err := m.db.WithContext(ctx).Where("id = ?", id).First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

// UpdateLogReplyBot stores the Reply Bot provider/model/text on an existing intercept log.
func (m *BrowserAIManager) UpdateLogReplyBot(ctx context.Context, logID, provider, model, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || strings.TrimSpace(logID) == "" {
		return nil
	}
	return m.db.WithContext(ctx).Model(&BrowserAILog{}).Where("id = ?", logID).Updates(map[string]any{
		"reply_bot_provider": provider,
		"reply_bot_model":    model,
		"reply_bot_text":     text,
	}).Error
}

// UpdateLogActionStatus updates action/status after Reply Bot "all questions" mode answers.
func (m *BrowserAIManager) UpdateLogActionStatus(ctx context.Context, logID, action, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || strings.TrimSpace(logID) == "" {
		return nil
	}
	updates := map[string]any{}
	if strings.TrimSpace(action) != "" {
		updates["action"] = action
	}
	if strings.TrimSpace(status) != "" {
		updates["status"] = status
	}
	if len(updates) == 0 {
		return nil
	}
	return m.db.WithContext(ctx).Model(&BrowserAILog{}).Where("id = ?", logID).Updates(updates).Error
}

// NormalizeGuardRuleAction canonicalizes rule actions. Supported: BLOCK, REDACT, WARN.
// Legacy ALERT maps to WARN.
func NormalizeGuardRuleAction(action string) string {
	a := strings.ToUpper(strings.TrimSpace(action))
	switch a {
	case "REDACT":
		return "REDACT"
	case "WARN", "ALERT":
		return "WARN"
	case "BLOCK":
		return "BLOCK"
	case "":
		return "BLOCK"
	default:
		return "BLOCK"
	}
}

// NormalizeGuardRuleSeverity canonicalizes rule severity. Supported: CRITICAL, HIGH, MEDIUM.
// Legacy LOW maps to MEDIUM.
func NormalizeGuardRuleSeverity(severity string) string {
	s := strings.ToUpper(strings.TrimSpace(severity))
	switch s {
	case "CRITICAL":
		return "CRITICAL"
	case "MEDIUM", "LOW":
		return "MEDIUM"
	case "HIGH":
		return "HIGH"
	case "":
		return "HIGH"
	default:
		return "HIGH"
	}
}

// FormatWarnedForwardPrompt is what ChatGPT/browser receives on REDACT: full original prompt + notice.
// Prompt Logs still store the original prompt only.
func FormatWarnedForwardPrompt(original, warningMessage string) string {
	w := strings.TrimSpace(warningMessage)
	if w == "" {
		w = "This prompt triggered a Raksha Guard redaction policy."
	}
	return strings.TrimRight(original, " \t\r\n") + "\n\n[RAKSHA REDACTED] " + w
}

// FormatWarningForwardPrompt is what ChatGPT/browser receives on WARN: full original prompt + warning.
func FormatWarningForwardPrompt(original, warningMessage string) string {
	w := strings.TrimSpace(warningMessage)
	if w == "" {
		w = "This prompt triggered a Raksha Guard warning policy."
	}
	return strings.TrimRight(original, " \t\r\n") + "\n\n[RAKSHA WARNING] " + w
}

// SecurityReplyForRule returns only the admin-authored warning. Empty if none was set.
func SecurityReplyForRule(ruleTriggered, warningMessage string) string {
	return strings.TrimSpace(warningMessage)
}

func hashUninstallKey(plaintext string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plaintext)))
	return hex.EncodeToString(sum[:])
}

func generateAgentUninstallKeyPlain() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func sealAgentUninstallKey(plain string) string {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return ""
	}
	if encrypt.IsEnabled() {
		if enc, err := encrypt.Encrypt(plain); err == nil {
			return "enc:" + enc
		}
	}
	return "b64:" + base64.StdEncoding.EncodeToString([]byte(plain))
}

func openAgentUninstallKey(sealed string) (string, error) {
	sealed = strings.TrimSpace(sealed)
	if sealed == "" {
		return "", fmt.Errorf("no uninstall key stored")
	}
	switch {
	case strings.HasPrefix(sealed, "enc:"):
		return encrypt.Decrypt(strings.TrimPrefix(sealed, "enc:"))
	case strings.HasPrefix(sealed, "b64:"):
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, "b64:"))
		if err != nil {
			return "", err
		}
		return string(raw), nil
	default:
		return "", fmt.Errorf("unknown uninstall key encoding")
	}
}

func assignAgentUninstallKey(agent *BrowserAIAgent) (plaintext string, err error) {
	if agent == nil {
		return "", fmt.Errorf("nil agent")
	}
	plain, err := generateAgentUninstallKeyPlain()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	agent.UninstallKeyHash = hashUninstallKey(plain)
	agent.UninstallKeyEnc = sealAgentUninstallKey(plain)
	agent.UninstallKeyRotatedAt = &now
	agent.HasUninstallKey = true
	return plain, nil
}

// IsAgentUninstallKeyExpired reports whether an agent's daily rolling uninstall key has expired.
// Rolling keys are valid for the UTC calendar day they were issued, up to a maximum of 24 hours.
func IsAgentUninstallKeyExpired(rotatedAt *time.Time) bool {
	if rotatedAt == nil || rotatedAt.IsZero() {
		return true
	}
	now := time.Now().UTC()
	rot := rotatedAt.UTC()
	if rot.Format("2006-01-02") != now.Format("2006-01-02") || now.Sub(rot) >= 24*time.Hour {
		return true
	}
	return false
}

func markAgentUninstallKeyFlag(agent *BrowserAIAgent) {
	if agent == nil {
		return
	}
	agent.HasUninstallKey = strings.TrimSpace(agent.UninstallKeyHash) != ""
}

func (m *BrowserAIManager) ensureAgentSettingsLocked(ctx context.Context) (*BrowserAIAgentSettings, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var settings BrowserAIAgentSettings
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserAIAgentSettingsID).First(&settings).Error; err != nil {
		settings = BrowserAIAgentSettings{
			ID:                  BrowserAIAgentSettingsID,
			RequireUninstallKey: true,
			UpdatedAt:           time.Now(),
		}
		if createErr := m.db.WithContext(ctx).Create(&settings).Error; createErr != nil {
			return nil, createErr
		}
	}
	settings.RequireUninstallKey = true
	settings.KeyConfigured = strings.TrimSpace(settings.UninstallKeyHash) != ""
	return &settings, nil
}

func (m *BrowserAIManager) GetAgentSettings(ctx context.Context) (*BrowserAIAgentSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureAgentSettingsLocked(ctx)
}

func (m *BrowserAIManager) SaveUninstallKey(ctx context.Context, plaintext, updatedBy string, requireKey *bool) (*BrowserAIAgentSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.ensureAgentSettingsLocked(ctx); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"updated_at": time.Now(),
		"updated_by": strings.TrimSpace(updatedBy),
		// Company policy: uninstall is always key-gated (Settings / CLI / remote).
		"require_uninstall_key": true,
	}
	if plaintext = strings.TrimSpace(plaintext); plaintext != "" {
		updates["uninstall_key_hash"] = hashUninstallKey(plaintext)
		updates["uninstall_key_enc"] = sealAgentUninstallKey(plaintext)
	}
	_ = requireKey // ignored — key is always required
	if err := m.db.WithContext(ctx).Model(&BrowserAIAgentSettings{}).Where("id = ?", BrowserAIAgentSettingsID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return m.ensureAgentSettingsLocked(ctx)
}

// GetCompanyUninstallKeyReveal returns the plaintext company uninstall key if configured.
func (m *BrowserAIManager) GetCompanyUninstallKeyReveal(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	settings, err := m.ensureAgentSettingsLocked(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(settings.UninstallKeyEnc) != "" {
		plain, err := openAgentUninstallKey(settings.UninstallKeyEnc)
		if err == nil && plain != "" {
			return plain, nil
		}
	}
	if settings.UninstallKeyHash == hashUninstallKey("12345678") {
		enc := sealAgentUninstallKey("12345678")
		_ = m.db.WithContext(ctx).Model(&BrowserAIAgentSettings{}).Where("id = ?", BrowserAIAgentSettingsID).Update("uninstall_key_enc", enc)
		settings.UninstallKeyEnc = enc
		return "12345678", nil
	}
	return "", fmt.Errorf("no company uninstall key stored")
}

func (m *BrowserAIManager) VerifyUninstallKey(ctx context.Context, plaintext string) (bool, *BrowserAIAgentSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return false, nil, fmt.Errorf("database not initialized")
	}
	var settings BrowserAIAgentSettings
	if err := m.db.WithContext(ctx).Where("id = ?", BrowserAIAgentSettingsID).First(&settings).Error; err != nil {
		return false, nil, err
	}
	settings.RequireUninstallKey = true
	settings.KeyConfigured = strings.TrimSpace(settings.UninstallKeyHash) != ""
	// No key configured → unlock nothing. Admin must set a company key first.
	if !settings.KeyConfigured {
		return false, &settings, nil
	}
	ok := hashUninstallKey(plaintext) == settings.UninstallKeyHash
	return ok, &settings, nil
}

// VerifyAgentUninstallKey accepts the per-Guard key OR the company uninstall key.
func (m *BrowserAIManager) VerifyAgentUninstallKey(ctx context.Context, agentID, plaintext string) (bool, *BrowserAIAgentSettings, error) {
	plaintext = strings.TrimSpace(plaintext)
	agentID = strings.TrimSpace(agentID)
	if plaintext == "" {
		return false, nil, nil
	}

	// Prefer per-Guard key when agent_id is known.
	if agentID != "" && m.db != nil {
		m.mu.RLock()
		var agent BrowserAIAgent
		err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error
		m.mu.RUnlock()
		if err == nil && strings.TrimSpace(agent.UninstallKeyHash) != "" {
			// Same expiry rule as reveal/auto-rotate, so yesterday's key stops working
			// the moment the admin UI shows today's key.
			if !IsAgentUninstallKeyExpired(agent.UninstallKeyRotatedAt) && hashUninstallKey(plaintext) == agent.UninstallKeyHash {
				settings, sErr := m.GetAgentSettings(ctx)
				return true, settings, sErr
			}
		}
	}

	// Fallback: company-wide uninstall key still works for any Guard.
	return m.VerifyUninstallKey(ctx, plaintext)
}

// GetAgentUninstallKeyReveal returns the plaintext Guard uninstall key for admin UI.
// Missing or expired daily keys are automatically regenerated.
func (m *BrowserAIManager) GetAgentUninstallKeyReveal(ctx context.Context, agentID string) (plaintext string, agent *BrowserAIAgent, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return "", nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", nil, fmt.Errorf("agent id is required")
	}
	var row BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&row).Error; err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(row.UninstallKeyEnc) != "" && !IsAgentUninstallKeyExpired(row.UninstallKeyRotatedAt) {
		plain, openErr := openAgentUninstallKey(row.UninstallKeyEnc)
		if openErr == nil && plain != "" {
			markAgentUninstallKeyFlag(&row)
			return plain, &row, nil
		}
	}
	// Expired or missing -> auto-rotate daily key for today!
	plain, genErr := assignAgentUninstallKey(&row)
	if genErr != nil {
		return "", nil, genErr
	}
	row.UpdatedAt = time.Now()
	if err := m.db.WithContext(ctx).Model(&BrowserAIAgent{}).Where("id = ?", row.ID).Updates(map[string]any{
		"uninstall_key_hash":       row.UninstallKeyHash,
		"uninstall_key_enc":        row.UninstallKeyEnc,
		"uninstall_key_rotated_at": row.UninstallKeyRotatedAt,
		"updated_at":               row.UpdatedAt,
	}).Error; err != nil {
		return "", nil, err
	}
	markAgentUninstallKeyFlag(&row)
	return plain, &row, nil
}

// RotateAgentUninstallKey issues a new per-Guard uninstall key.
func (m *BrowserAIManager) RotateAgentUninstallKey(ctx context.Context, agentID string) (plaintext string, agent *BrowserAIAgent, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return "", nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", nil, fmt.Errorf("agent id is required")
	}
	var row BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&row).Error; err != nil {
		return "", nil, err
	}
	plain, genErr := assignAgentUninstallKey(&row)
	if genErr != nil {
		return "", nil, genErr
	}
	row.UpdatedAt = time.Now()
	if err := m.db.WithContext(ctx).Model(&BrowserAIAgent{}).Where("id = ?", row.ID).Updates(map[string]any{
		"uninstall_key_hash":       row.UninstallKeyHash,
		"uninstall_key_enc":        row.UninstallKeyEnc,
		"uninstall_key_rotated_at": row.UninstallKeyRotatedAt,
		"updated_at":               row.UpdatedAt,
	}).Error; err != nil {
		return "", nil, err
	}
	markAgentUninstallKeyFlag(&row)
	return plain, &row, nil
}

func (m *BrowserAIManager) UpsertAgentHeartbeat(ctx context.Context, incoming *BrowserAIAgent) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if incoming == nil || strings.TrimSpace(incoming.ID) == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	now := time.Now()
	var existing BrowserAIAgent
	err := m.db.WithContext(ctx).Where("id = ?", incoming.ID).First(&existing).Error
	if err != nil {
		agent := BrowserAIAgent{
			ID:            strings.TrimSpace(incoming.ID),
			Hostname:      strings.TrimSpace(incoming.Hostname),
			Username:      strings.TrimSpace(incoming.Username),
			ContactEmail:  preferEmail(incoming.ContactEmail, incoming.Username),
			IPAddress:     strings.TrimSpace(incoming.IPAddress),
			MacAddress:    strings.TrimSpace(incoming.MacAddress),
			TransportName: nicGUIDFromTransport(incoming.TransportName),
			OSVersion:     strings.TrimSpace(incoming.OSVersion),
			AgentVersion:  strings.TrimSpace(incoming.AgentVersion),
			AgentType:     NormalizeBrowserAIAgentType(incoming.AgentType),
			HealthStatus:  strings.TrimSpace(incoming.HealthStatus),
			HealthDetail:  strings.TrimSpace(incoming.HealthDetail),
			Status:        AgentStatusActive,
			LastSeenAt:    now,
			InstalledAt:   now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if agent.AgentVersion != "" {
			agent.VersionUpdatedAt = &now
		}
		if sha := strings.TrimSpace(incoming.ProxyBundleSHA); sha != "" {
			agent.ProxyBundleSHA = sha
			agent.ProxyBundleUpdatedAt = &now
		}
		// Reinstall on the same laptop gets a new Guard ID — keep the admin-saved contact email.
		if agent.Hostname != "" {
			var prev BrowserAIAgent
			if m.db.WithContext(ctx).
				Where("LOWER(hostname) = ? AND contact_email_pinned = ?", strings.ToLower(agent.Hostname), true).
				Order("updated_at DESC").First(&prev).Error == nil {
				agent.ContactEmail = prev.ContactEmail
				agent.ContactEmailPinned = true
			}
		}
		if _, genErr := assignAgentUninstallKey(&agent); genErr != nil {
			return nil, fmt.Errorf("generate guard uninstall key: %w", genErr)
		}
		if createErr := m.db.WithContext(ctx).Create(&agent).Error; createErr != nil {
			return nil, createErr
		}
		m.purgeHostDuplicates(ctx, agent.ID, agent.Hostname, agent.MacAddress, agent.Username)
		markAgentUninstallKeyFlag(&agent)
		return &agent, nil
	}

	existing.Hostname = firstNonEmpty(strings.TrimSpace(incoming.Hostname), existing.Hostname)
	existing.Username = firstNonEmpty(strings.TrimSpace(incoming.Username), existing.Username)
	if !existing.ContactEmailPinned {
		if ce := preferEmail(incoming.ContactEmail, ""); ce != "" {
			existing.ContactEmail = ce
		} else if strings.TrimSpace(existing.ContactEmail) == "" {
			existing.ContactEmail = preferEmail("", existing.Username)
		}
	}
	existing.IPAddress = firstNonEmpty(strings.TrimSpace(incoming.IPAddress), existing.IPAddress)
	existing.MacAddress = firstNonEmpty(strings.TrimSpace(incoming.MacAddress), existing.MacAddress)
	if t := nicGUIDFromTransport(incoming.TransportName); t != "" {
		existing.TransportName = t
	}
	existing.OSVersion = firstNonEmpty(strings.TrimSpace(incoming.OSVersion), existing.OSVersion)
	if v := strings.TrimSpace(incoming.AgentVersion); v != "" && (v != existing.AgentVersion || existing.VersionUpdatedAt == nil) {
		existing.AgentVersion = v
		existing.VersionUpdatedAt = &now
	}
	if sha := strings.TrimSpace(incoming.ProxyBundleSHA); sha != existing.ProxyBundleSHA {
		existing.ProxyBundleSHA = sha
		existing.ProxyBundleUpdatedAt = &now
	}
	if at := strings.TrimSpace(incoming.AgentType); at != "" {
		existing.AgentType = NormalizeBrowserAIAgentType(at)
	} else if strings.TrimSpace(existing.AgentType) == "" {
		existing.AgentType = "endpoint"
	}
	if hs := strings.TrimSpace(incoming.HealthStatus); hs != "" {
		existing.HealthStatus = hs
	}
	if hd := strings.TrimSpace(incoming.HealthDetail); hd != "" {
		existing.HealthDetail = hd
	}
	existing.LastSeenAt = now
	existing.UpdatedAt = now
	if existing.Status == AgentStatusUninstalled {
		// Agent has already been uninstalled. Do not resurrect it back to active on trailing heartbeats.
		// Status remains AgentStatusUninstalled and UninstalledAt timestamp is preserved.
	} else if existing.Status == AgentStatusPaused {
		// Agent is in temporary standby / paused mode.
		existing.Status = AgentStatusPaused
	} else if existing.UninstallRequested || existing.Status == AgentStatusUninstallPending {
		existing.Status = AgentStatusUninstallPending
		existing.UninstallRequested = true
	} else {
		existing.Status = AgentStatusActive
		existing.UninstalledAt = nil
	}
	// Backfill per-Guard uninstall key or auto-rotate if daily key expired.
	if strings.TrimSpace(existing.UninstallKeyHash) == "" || IsAgentUninstallKeyExpired(existing.UninstallKeyRotatedAt) {
		if _, genErr := assignAgentUninstallKey(&existing); genErr != nil {
			return nil, fmt.Errorf("generate guard uninstall key: %w", genErr)
		}
	}
	if err := m.db.WithContext(ctx).Save(&existing).Error; err != nil {
		return nil, err
	}
	if existing.Status == AgentStatusActive {
		m.purgeHostDuplicates(ctx, existing.ID, existing.Hostname, existing.MacAddress, existing.Username)
	}
	markAgentUninstallKeyFlag(&existing)
	return &existing, nil
}

func (m *BrowserAIManager) purgeHostDuplicates(ctx context.Context, keepID, hostname, macAddress, username string) {
	if m.db == nil || strings.TrimSpace(hostname) == "" {
		return
	}
	hostLower := strings.ToLower(strings.TrimSpace(hostname))
	macClean := strings.ToLower(strings.TrimSpace(macAddress))
	userLower := strings.ToLower(strings.TrimSpace(username))

	subQuery := m.db.WithContext(ctx).Model(&BrowserAIAgent{}).
		Where("id != ? AND LOWER(hostname) = ?", keepID, hostLower)

	if macClean != "" && macClean != "—" && macClean != "00:00:00:00:00:00" && !strings.HasPrefix(macClean, "00:00") {
		subQuery = subQuery.Where("(LOWER(mac_address) = ? OR mac_address = '—' OR mac_address = '' OR mac_address IS NULL)", macClean)
	} else if userLower != "" {
		subQuery = subQuery.Where("LOWER(username) = ?", userLower)
	}

	var dupIDs []string
	if err := subQuery.Pluck("id", &dupIDs).Error; err == nil && len(dupIDs) > 0 {
		_ = m.db.WithContext(ctx).Model(&BrowserAILog{}).Where("agent_id IN ?", dupIDs).Update("agent_id", keepID).Error
		_ = m.db.WithContext(ctx).Where("id IN ?", dupIDs).Delete(&BrowserAIAgent{}).Error
	}
}

// AutoRotateDailyAgentUninstallKeys scans active agents and auto-rotates any expired daily uninstall keys.
func (m *BrowserAIManager) AutoRotateDailyAgentUninstallKeys(ctx context.Context) (int, error) {
	if m.db == nil {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var agents []BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("status != ?", AgentStatusUninstalled).Find(&agents).Error; err != nil {
		return 0, err
	}

	rotatedCount := 0
	for i := range agents {
		if IsAgentUninstallKeyExpired(agents[i].UninstallKeyRotatedAt) || strings.TrimSpace(agents[i].UninstallKeyHash) == "" {
			if _, err := assignAgentUninstallKey(&agents[i]); err == nil {
				agents[i].UpdatedAt = time.Now()
				_ = m.db.WithContext(ctx).Model(&BrowserAIAgent{}).Where("id = ?", agents[i].ID).Updates(map[string]any{
					"uninstall_key_hash":       agents[i].UninstallKeyHash,
					"uninstall_key_enc":        agents[i].UninstallKeyEnc,
					"uninstall_key_rotated_at": agents[i].UninstallKeyRotatedAt,
					"updated_at":               agents[i].UpdatedAt,
				}).Error
				rotatedCount++
			}
		}
	}
	return rotatedCount, nil
}

func nicGUIDFromTransport(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if m := nicGUID.FindString(s); m != "" {
		return strings.ToUpper(m)
	}
	// macOS / Linux: keep "Wi-Fi", "Ethernet", "en0" — not Windows GUID-only.
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (m *BrowserAIManager) ListAgents(ctx context.Context, status, search string, limit, offset int, agentType ...string) ([]BrowserAIAgent, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var agents []BrowserAIAgent
	var total int64
	if m.db == nil {
		return agents, 0, nil
	}

	query := m.db.WithContext(ctx).Model(&BrowserAIAgent{})
	if status != "" && strings.ToLower(status) != "all" {
		query = query.Where("LOWER(status) = ?", strings.ToLower(status))
	}
	typeFilter := ""
	if len(agentType) > 0 {
		typeFilter = strings.TrimSpace(agentType[0])
	}
	if typeFilter != "" && strings.ToLower(typeFilter) != "all" {
		query = query.Where("LOWER(agent_type) = ?", NormalizeBrowserAIAgentType(typeFilter))
	}
	if search != "" {
		s := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(hostname) LIKE ? OR LOWER(username) LIKE ? OR LOWER(ip_address) LIKE ? OR LOWER(mac_address) LIKE ? OR LOWER(transport_name) LIKE ? OR LOWER(id) LIKE ? OR LOWER(agent_version) LIKE ? OR LOWER(agent_type) LIKE ?",
			s, s, s, s, s, s, s, s,
		)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	err := query.Order("last_seen_at DESC").Limit(limit).Offset(offset).Find(&agents).Error
	for i := range agents {
		markAgentUninstallKeyFlag(&agents[i])
	}
	return agents, total, err
}

func (m *BrowserAIManager) CountAgentsStatus(ctx context.Context, search string, agentType ...string) (active int64, uninstalled int64, paused int64, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return 0, 0, 0, nil
	}
	buildBase := func() *gorm.DB {
		q := m.db.WithContext(ctx).Model(&BrowserAIAgent{})
		typeFilter := ""
		if len(agentType) > 0 {
			typeFilter = strings.TrimSpace(agentType[0])
		}
		if typeFilter != "" && strings.ToLower(typeFilter) != "all" {
			q = q.Where("LOWER(agent_type) = ?", NormalizeBrowserAIAgentType(typeFilter))
		}
		if search != "" {
			s := "%" + strings.ToLower(search) + "%"
			q = q.Where(
				"LOWER(hostname) LIKE ? OR LOWER(username) LIKE ? OR LOWER(ip_address) LIKE ? OR LOWER(mac_address) LIKE ? OR LOWER(transport_name) LIKE ? OR LOWER(id) LIKE ? OR LOWER(agent_version) LIKE ? OR LOWER(agent_type) LIKE ?",
				s, s, s, s, s, s, s, s,
			)
		}
		return q
	}
	_ = buildBase().Where("LOWER(status) = ?", AgentStatusActive).Count(&active).Error
	_ = buildBase().Where("LOWER(status) = ?", AgentStatusPaused).Count(&paused).Error
	_ = buildBase().Where("LOWER(status) = ?", AgentStatusUninstalled).Count(&uninstalled).Error
	return active, uninstalled, paused, nil
}

func (m *BrowserAIManager) GetAgent(ctx context.Context, agentID string) (*BrowserAIAgent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var agent BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", strings.TrimSpace(agentID)).First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (m *BrowserAIManager) RecordAgentTamper(ctx context.Context, agentID, detail string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || strings.TrimSpace(agentID) == "" {
		return nil
	}
	now := time.Now()
	return m.db.WithContext(ctx).Model(&BrowserAIAgent{}).
		Where("id = ?", strings.TrimSpace(agentID)).
		Updates(map[string]any{
			"health_status": "tampered",
			"health_detail": detail,
			"updated_at":    now,
		}).Error
}

func (m *BrowserAIManager) MarkAgentUninstalled(ctx context.Context, agentID string) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	now := time.Now()
	var agent BrowserAIAgent
	err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error
	if err != nil {
		// Never registered / heartbeat never landed — still record uninstall so Windows setup can finish.
		agent = BrowserAIAgent{
			ID:                 agentID,
			Status:             AgentStatusUninstalled,
			UninstallRequested: false,
			LastSeenAt:         now,
			InstalledAt:        now,
			UninstalledAt:      &now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if createErr := m.db.WithContext(ctx).Create(&agent).Error; createErr != nil {
			return nil, createErr
		}
		return &agent, nil
	}
	agent.Status = AgentStatusUninstalled
	agent.UninstallRequested = false
	agent.UninstalledAt = &now
	agent.UpdatedAt = now
	if err := m.db.WithContext(ctx).Save(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (m *BrowserAIManager) RequestRemoteUninstall(ctx context.Context, agentID string) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	var agent BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found")
	}
	if agent.Status == AgentStatusUninstalled {
		return &agent, nil
	}
	now := time.Now()
	agent.UninstallRequested = true
	agent.Status = AgentStatusUninstallPending
	agent.UpdatedAt = now
	if err := m.db.WithContext(ctx).Save(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (m *BrowserAIManager) AckRemoteUninstall(ctx context.Context, agentID string) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	var agent BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found")
	}
	if !agent.UninstallRequested && agent.Status != AgentStatusUninstallPending {
		return nil, fmt.Errorf("remote uninstall was not requested")
	}
	now := time.Now()
	agent.Status = AgentStatusUninstalled
	agent.UninstallRequested = false
	agent.UninstalledAt = &now
	agent.UpdatedAt = now
	if err := m.db.WithContext(ctx).Save(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (m *BrowserAIManager) PauseAgent(ctx context.Context, agentID string) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	var agent BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found")
	}
	if agent.Status == AgentStatusUninstalled {
		return nil, fmt.Errorf("cannot pause an uninstalled agent")
	}
	now := time.Now()
	agent.Status = AgentStatusPaused
	agent.UpdatedAt = now
	if err := m.db.WithContext(ctx).Save(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (m *BrowserAIManager) ResumeAgent(ctx context.Context, agentID string) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	var agent BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found")
	}
	now := time.Now()
	agent.Status = AgentStatusActive
	agent.UninstallRequested = false
	agent.UninstalledAt = nil
	agent.UpdatedAt = now
	if err := m.db.WithContext(ctx).Save(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (m *BrowserAIManager) AllowReinstallAgent(ctx context.Context, agentID string) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	var agent BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found")
	}
	now := time.Now()
	agent.Status = AgentStatusActive
	agent.UninstallRequested = false
	agent.UninstalledAt = nil
	agent.UpdatedAt = now
	if err := m.db.WithContext(ctx).Save(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (m *BrowserAIManager) DeleteAgents(ctx context.Context, ids []string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return 0, fmt.Errorf("at least one agent id is required")
	}
	result := m.db.WithContext(ctx).Where("id IN ?", unique).Delete(&BrowserAIAgent{})
	return result.RowsAffected, result.Error
}

func preferEmail(primary, fallback string) string {
	for _, s := range []string{strings.TrimSpace(primary), strings.TrimSpace(fallback)} {
		if s != "" && strings.Contains(s, "@") {
			return s
		}
	}
	return strings.TrimSpace(primary)
}

// RecordWarningEmail persists a Guard Insights / Agents warning email audit row.
func (m *BrowserAIManager) RecordWarningEmail(ctx context.Context, entry *BrowserAIWarningEmailLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil || entry == nil {
		return nil
	}
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.SentAt.IsZero() {
		entry.SentAt = time.Now()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = entry.SentAt
	}
	if entry.Status == "" {
		entry.Status = "sent"
	}
	return m.db.WithContext(ctx).Create(entry).Error
}

// UpdateAgentContactEmail is the admin save: sets (or clears, with "") the warning-mail
// address and pins it so heartbeats / later warning mails never overwrite it.
func (m *BrowserAIManager) UpdateAgentContactEmail(ctx context.Context, agentID, email string) (*BrowserAIAgent, error) {
	return m.setAgentContactEmail(ctx, agentID, email, true)
}

// RememberAgentContactEmail fills the address used for a warning mail only when the
// admin has not pinned one.
func (m *BrowserAIManager) RememberAgentContactEmail(ctx context.Context, agentID, email string) (*BrowserAIAgent, error) {
	return m.setAgentContactEmail(ctx, agentID, email, false)
}

func (m *BrowserAIManager) setAgentContactEmail(ctx context.Context, agentID, email string, pin bool) (*BrowserAIAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	email = strings.TrimSpace(email)
	if email != "" && (!strings.Contains(email, "@") || strings.ContainsAny(email, " ,;")) {
		return nil, fmt.Errorf("invalid email address")
	}
	var agent BrowserAIAgent
	if err := m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found")
	}
	if !pin && agent.ContactEmailPinned {
		return &agent, nil
	}
	agent.ContactEmail = email
	if pin {
		agent.ContactEmailPinned = true
	}
	agent.UpdatedAt = time.Now()
	if err := m.db.WithContext(ctx).Model(&BrowserAIAgent{}).Where("id = ?", agent.ID).Updates(map[string]any{
		"contact_email":        agent.ContactEmail,
		"contact_email_pinned": agent.ContactEmailPinned,
		"updated_at":           agent.UpdatedAt,
	}).Error; err != nil {
		return nil, err
	}
	markAgentUninstallKeyFlag(&agent)
	return &agent, nil
}

// GetAgentInsightStats returns full-DB action totals per agent_id for Guard Insights.
func (m *BrowserAIManager) GetAgentInsightStats(ctx context.Context) ([]BrowserAIAgentInsightStats, map[string]int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []BrowserAIAgentInsightStats{}
	totals := map[string]int64{
		"allowed": 0,
		"blocked": 0,
		"warn":    0,
		"redact":  0,
		"total":   0,
	}
	if m.db == nil {
		return out, totals, nil
	}

	type row struct {
		AgentID string
		Action  string
		Count   int64
	}
	var rows []row
	err := m.db.WithContext(ctx).Model(&BrowserAILog{}).
		Select("agent_id as agent_id, LOWER(action) as action, COUNT(*) as count").
		Where("agent_id IS NOT NULL AND agent_id <> ''").
		Group("agent_id, LOWER(action)").
		Scan(&rows).Error
	if err != nil {
		return out, totals, err
	}

	byAgent := map[string]*BrowserAIAgentInsightStats{}
	for _, r := range rows {
		id := strings.TrimSpace(r.AgentID)
		if id == "" {
			continue
		}
		st, ok := byAgent[id]
		if !ok {
			st = &BrowserAIAgentInsightStats{AgentID: id}
			byAgent[id] = st
		}
		act := strings.ToLower(strings.TrimSpace(r.Action))
		switch {
		case strings.Contains(act, "block"):
			st.BlockedCount += r.Count
			totals["blocked"] += r.Count
		case strings.Contains(act, "redact"):
			st.RedactCount += r.Count
			totals["redact"] += r.Count
		case strings.Contains(act, "warn"):
			st.WarnCount += r.Count
			totals["warn"] += r.Count
		default:
			st.AllowedCount += r.Count
			totals["allowed"] += r.Count
		}
		st.TotalCount += r.Count
		totals["total"] += r.Count
	}

	out = make([]BrowserAIAgentInsightStats, 0, len(byAgent))
	for _, st := range byAgent {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalCount == out[j].TotalCount {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].TotalCount > out[j].TotalCount
	})
	return out, totals, nil
}
