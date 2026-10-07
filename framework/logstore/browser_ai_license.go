package logstore

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MasterPublicKeyBytes is the built-in 32-byte Ed25519 public key.
// It can only verify signatures created by the Raksha vendor Master Private Key.
var MasterPublicKeyBytes = []byte{
	0x4b, 0x51, 0xda, 0xdd, 0xe1, 0x49, 0x09, 0xac,
	0x5c, 0xd5, 0xf2, 0xd5, 0x2b, 0x03, 0xc5, 0xe8,
	0x85, 0xa8, 0x9d, 0x36, 0xe2, 0x6f, 0xcd, 0x60,
	0xac, 0xc4, 0xdc, 0xe3, 0x27, 0x17, 0x80, 0x3a,
}

const (
	BrowserAILicenseID     = "browser-ai-license-active"
	DefaultUnlicensedSeats = 100 // Default seat allowance if no explicit signed license uploaded yet
)

var (
	licenseCacheMu    sync.RWMutex
	cachedLicenseInfo *LicenseStatusInfo
)

// BrowserAILicenseRecord persists active license details in DB.
type BrowserAILicenseRecord struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	LicenseID   string    `json:"license_id"`
	Issuer      string    `json:"issuer"`
	Product     string    `json:"product"`
	ClientName  string    `json:"client_name"`
	Tier        string    `json:"tier"`
	MaxSeats    int       `json:"max_seats"`
	Features    string    `json:"features"`
	RawEnvelope string    `gorm:"type:text" json:"raw_envelope"`
	Signature   string    `json:"signature"`
	IssuedAt    time.Time `json:"issued_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   string    `json:"updated_by"`
}

func (BrowserAILicenseRecord) TableName() string {
	return "browser_ai_license"
}

// EnterpriseLicensePayload represents the cryptographically signed license payload.
type EnterpriseLicensePayload struct {
	Version    string   `json:"version"`
	LicenseID  string   `json:"license_id"`
	Issuer     string   `json:"issuer"`
	Product    string   `json:"product"`
	ClientName string   `json:"client_name"`
	Tier       string   `json:"tier"`
	MaxSeats   int      `json:"max_seats"`
	Features   []string `json:"features"`
	IssuedAt   string   `json:"issued_at"`
	ExpiresAt  string   `json:"expires_at"`
}

// EnterpriseLicenseEnvelope represents the exported .lic file structure.
type EnterpriseLicenseEnvelope struct {
	Format     string                   `json:"format"`
	Payload    EnterpriseLicensePayload `json:"payload"`
	PayloadB64 string                   `json:"payload_b64"`
	Signature  string                   `json:"signature"`
}

// LicenseStatusInfo represents current fleet capacity & status sent to UI/Dashboard.
type LicenseStatusInfo struct {
	IsActive       bool     `json:"is_active"`
	IsLicensed     bool     `json:"is_licensed"`
	IsExpired      bool     `json:"is_expired"`
	LicenseID      string   `json:"license_id"`
	Issuer         string   `json:"issuer"`
	Product        string   `json:"product"`
	ClientName     string   `json:"client_name"`
	Tier           string   `json:"tier"`
	MaxSeats       int      `json:"max_seats"`
	ActiveSeats    int      `json:"active_seats"`
	RemainingSeats int      `json:"remaining_seats"`
	ExpiresAt      string   `json:"expires_at"`
	IssuedAt       string   `json:"issued_at"`
	Features       []string `json:"features"`
	StatusMessage  string   `json:"status_message"`
}

// VerifyLicenseEnvelope cryptographically validates the given license against the embedded Ed25519 public key.
func VerifyLicenseEnvelope(raw []byte) (*EnterpriseLicensePayload, *EnterpriseLicenseEnvelope, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil, fmt.Errorf("empty license content")
	}

	// Try parsing envelope JSON
	var env EnterpriseLicenseEnvelope
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		// Could be base64-wrapped
		decoded, decErr := base64.StdEncoding.DecodeString(trimmed)
		if decErr == nil {
			if jsonErr := json.Unmarshal(decoded, &env); jsonErr != nil {
				return nil, nil, fmt.Errorf("invalid license JSON format: %w", err)
			}
		} else {
			return nil, nil, fmt.Errorf("invalid license format: %w", err)
		}
	}

	if env.PayloadB64 == "" || env.Signature == "" {
		return nil, nil, fmt.Errorf("malformed license: missing payload_b64 or signature")
	}

	rawPayload, err := base64.StdEncoding.DecodeString(env.PayloadB64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload encoding: %w", err)
	}

	rawSig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Cryptographic Ed25519 signature verification
	pubKey := ed25519.PublicKey(MasterPublicKeyBytes)
	if !ed25519.Verify(pubKey, rawPayload, rawSig) {
		return nil, nil, fmt.Errorf("CRYPTOGRAPHIC_SIGNATURE_INVALID: license data is tampered, corrupted, or forged")
	}

	var payload EnterpriseLicensePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, nil, fmt.Errorf("corrupt payload json: %w", err)
	}

	if payload.MaxSeats <= 0 {
		return nil, nil, fmt.Errorf("invalid max_seats in license: must be positive integer")
	}

	// Check Expiry Date
	if payload.ExpiresAt != "" {
		expTime, err := time.Parse(time.RFC3339, payload.ExpiresAt)
		if err != nil {
			// Try YYYY-MM-DD
			expTime, err = time.Parse("2006-01-02", payload.ExpiresAt)
		}
		if err == nil && time.Now().UTC().After(expTime) {
			return &payload, &env, fmt.Errorf("LICENSE_EXPIRED: license expired on %s", payload.ExpiresAt)
		}
	}

	return &payload, &env, nil
}

// GetActiveLicense returns the current license status and seats usage.
func (m *BrowserAIManager) GetActiveLicense(ctx context.Context) (*LicenseStatusInfo, error) {
	activeCount, _, _, _ := m.CountAgentsStatus(ctx, "", "")
	allocatedSeats := int(activeCount)

	m.mu.RLock()
	defer m.mu.RUnlock()

	var record BrowserAILicenseRecord
	var hasRecord bool
	if m.db != nil {
		if err := m.db.WithContext(ctx).Where("id = ?", BrowserAILicenseID).First(&record).Error; err == nil {
			hasRecord = true
		}
	}

	// If no record in DB, attempt reading from fallback file locations
	if !hasRecord {
		fallbackPaths := []string{
			"/app/data/raksha_license.lic",
			filepath.Join(os.Getenv("APP_DIR"), "raksha_license.lic"),
			"data/raksha_license.lic",
			"raksha_license.lic",
		}
		for _, fp := range fallbackPaths {
			if fp == "" {
				continue
			}
			if b, err := os.ReadFile(fp); err == nil && len(b) > 0 {
				payload, _, vErr := VerifyLicenseEnvelope(b)
				if vErr == nil && payload != nil {
					rem := payload.MaxSeats - allocatedSeats
					if rem < 0 {
						rem = 0
					}
					return &LicenseStatusInfo{
						IsActive:       true,
						IsLicensed:     true,
						IsExpired:      false,
						LicenseID:      payload.LicenseID,
						Issuer:         firstNonEmpty(payload.Issuer, "YesPanchi Group of Companies"),
						Product:        firstNonEmpty(payload.Product, "Raksha - Real-time AI Knowledge Screening & Hazard Audit"),
						ClientName:     firstNonEmpty(payload.ClientName, "Enterprise Organization"),
						Tier:           firstNonEmpty(payload.Tier, "Enterprise On-Premise"),
						MaxSeats:       payload.MaxSeats,
						ActiveSeats:    allocatedSeats,
						RemainingSeats: rem,
						ExpiresAt:      payload.ExpiresAt,
						IssuedAt:       payload.IssuedAt,
						Features:       payload.Features,
						StatusMessage:  fmt.Sprintf("Active (%d/%d seats in use)", allocatedSeats, payload.MaxSeats),
					}, nil
				}
			}
		}

		// Default enterprise state (1-year license validity)
		maxSeats := DefaultUnlicensedSeats
		rem := maxSeats - allocatedSeats
		if rem < 0 {
			rem = 0
		}
		evalExpiry := time.Now().UTC().AddDate(1, 0, 0).Format(time.RFC3339)
		evalIssued := time.Now().UTC().Format(time.RFC3339)
		return &LicenseStatusInfo{
			IsActive:       true,
			IsLicensed:     true,
			IsExpired:      false,
			LicenseID:      "YP-ENTERPRISE-PROD",
			Issuer:         "YesPanchi Group of Companies",
			Product:        "Raksha - Real-time AI Knowledge Screening & Hazard Audit",
			ClientName:     "Enterprise Organization",
			Tier:           "Enterprise On-Premise",
			MaxSeats:       maxSeats,
			ActiveSeats:    allocatedSeats,
			RemainingSeats: rem,
			ExpiresAt:      evalExpiry,
			IssuedAt:       evalIssued,
			Features:       []string{"browser_ai_guard", "dlp_regex"},
			StatusMessage:  fmt.Sprintf("Enterprise On-Premise (%d/%d seats in use)", allocatedSeats, maxSeats),
		}, nil
	}

	// Verify existing DB record signature to prevent offline database tampering
	payload, _, err := VerifyLicenseEnvelope([]byte(record.RawEnvelope))
	if err != nil {
		return &LicenseStatusInfo{
			IsActive:       false,
			IsLicensed:     true,
			IsExpired:      strings.Contains(err.Error(), "EXPIRED"),
			LicenseID:      record.LicenseID,
			Issuer:         firstNonEmpty(record.Issuer, "YesPanchi Group of Companies"),
			Product:        firstNonEmpty(record.Product, "Raksha - Real-time AI Knowledge Screening & Hazard Audit"),
			ClientName:     record.ClientName,
			Tier:           record.Tier,
			MaxSeats:       record.MaxSeats,
			ActiveSeats:    allocatedSeats,
			RemainingSeats: 0,
			ExpiresAt:      record.ExpiresAt.Format(time.RFC3339),
			IssuedAt:       record.IssuedAt.Format(time.RFC3339),
			StatusMessage:  fmt.Sprintf("License Invalid or Tampered: %v", err),
		}, nil
	}

	rem := payload.MaxSeats - allocatedSeats
	if rem < 0 {
		rem = 0
	}

	features := []string{}
	if record.Features != "" {
		_ = json.Unmarshal([]byte(record.Features), &features)
	}

	return &LicenseStatusInfo{
		IsActive:       true,
		IsLicensed:     true,
		IsExpired:      false,
		LicenseID:      payload.LicenseID,
		Issuer:         firstNonEmpty(payload.Issuer, record.Issuer, "YesPanchi Group of Companies"),
		Product:        firstNonEmpty(payload.Product, record.Product, "Raksha - Real-time AI Knowledge Screening & Hazard Audit"),
		ClientName:     payload.ClientName,
		Tier:           payload.Tier,
		MaxSeats:       payload.MaxSeats,
		ActiveSeats:    allocatedSeats,
		RemainingSeats: rem,
		ExpiresAt:      payload.ExpiresAt,
		IssuedAt:       payload.IssuedAt,
		Features:       payload.Features,
		StatusMessage:  fmt.Sprintf("Active Enterprise (%d/%d seats in use)", allocatedSeats, payload.MaxSeats),
	}, nil
}

// ActivateLicense validates and activates a new signed license file/token.
func (m *BrowserAIManager) ActivateLicense(ctx context.Context, rawLicense []byte, updatedBy string) (*LicenseStatusInfo, error) {
	payload, env, err := VerifyLicenseEnvelope(rawLicense)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()

	now := time.Now().UTC()
	var expTime time.Time
	if payload.ExpiresAt != "" {
		if t, tErr := time.Parse(time.RFC3339, payload.ExpiresAt); tErr == nil {
			expTime = t
		} else if t, tErr := time.Parse("2006-01-02", payload.ExpiresAt); tErr == nil {
			expTime = t
		}
	}

	var issTime time.Time
	if payload.IssuedAt != "" {
		if t, tErr := time.Parse(time.RFC3339, payload.IssuedAt); tErr == nil {
			issTime = t
		}
	}

	featuresJSON, _ := json.Marshal(payload.Features)
	rawEnvBytes, _ := json.MarshalIndent(env, "", "  ")

	if m.db != nil {
		rec := BrowserAILicenseRecord{
			ID:          BrowserAILicenseID,
			LicenseID:   payload.LicenseID,
			Issuer:      firstNonEmpty(payload.Issuer, "YesPanchi Group of Companies"),
			Product:     firstNonEmpty(payload.Product, "Raksha - Real-time AI Knowledge Screening & Hazard Audit"),
			ClientName:  payload.ClientName,
			Tier:        payload.Tier,
			MaxSeats:    payload.MaxSeats,
			Features:    string(featuresJSON),
			RawEnvelope: string(rawEnvBytes),
			Signature:   env.Signature,
			IssuedAt:    issTime,
			ExpiresAt:   expTime,
			UpdatedAt:   now,
			UpdatedBy:   strings.TrimSpace(updatedBy),
		}

		if err := m.db.WithContext(ctx).Save(&rec).Error; err != nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("failed to save license record: %w", err)
		}
	}

	// Also backup to file
	appDir := os.Getenv("APP_DIR")
	if appDir == "" {
		appDir = "/app/data"
	}
	licPath := filepath.Join(appDir, "raksha_license.lic")
	_ = os.MkdirAll(appDir, 0755)
	_ = os.WriteFile(licPath, rawEnvBytes, 0644)
	_ = os.WriteFile("raksha_license.lic", rawEnvBytes, 0644)

	m.mu.Unlock()

	licenseCacheMu.Lock()
	cachedLicenseInfo = nil
	licenseCacheMu.Unlock()

	return m.GetActiveLicense(ctx)
}

// CheckSeatQuotaEnforcement verifies if a laptop agent is allowed to be active.
// - Existing active laptop (same Agent ID or MAC + Hostname) consumes no new seat.
// - If another laptop was paused (OFF) or uninstalled, that seat is freed up.
// - New laptop registration is permitted if currently active laptops < maxSeats.
func (m *BrowserAIManager) CheckSeatQuotaEnforcement(ctx context.Context, agentID, hostname, mac string) error {
	lic, err := m.GetActiveLicense(ctx)
	if err != nil || lic == nil {
		return nil // Fail open on internal DB error, or default capacity
	}

	if !lic.IsActive {
		return fmt.Errorf("LICENSE_INACTIVE: Browser Guard enterprise license is expired or invalid (%s)", lic.StatusMessage)
	}

	// 1. Check if THIS specific laptop is ALREADY counted as active:
	// A laptop is uniquely identified by:
	// - Primary: exact Agent ID
	// - Secondary: Hardware MAC Address + Hostname (for reinstalls)
	if m.db != nil {
		// A) Exact Agent ID is already active:
		if agentID != "" {
			var agent BrowserAIAgent
			if m.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error == nil {
				if agent.Status == AgentStatusActive {
					return nil // Already active, no new seat consumed!
				}
			}
		}

		// B) Same physical hardware (MAC address + Hostname) already active:
		if mac != "" && hostname != "" {
			var hardwareAgent BrowserAIAgent
			if m.db.WithContext(ctx).
				Where("LOWER(mac_address) = ? AND LOWER(hostname) = ? AND LOWER(status) = ?",
					strings.ToLower(mac), strings.ToLower(hostname), AgentStatusActive).
				First(&hardwareAgent).Error == nil {
				return nil // Same physical laptop updating, no new seat consumed!
			}
		}
	}

	// 2. This is a NEW laptop or a previously paused laptop trying to become active.
	// Check if active seats >= maxSeats:
	if lic.ActiveSeats >= lic.MaxSeats {
		return fmt.Errorf("SEAT_LIMIT_REACHED: Enterprise on-premise license capacity reached (%d/%d active laptops in use). To activate this laptop, turn off or uninstall an unused laptop, or contact YesPanchi to upgrade seats.",
			lic.ActiveSeats, lic.MaxSeats)
	}

	return nil
}

