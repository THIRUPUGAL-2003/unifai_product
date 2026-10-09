package logstore

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// MasterPublicKeyBytes is the built-in 32-byte Ed25519 public key.
// It can only verify signatures created by the Gateway vendor Master Private Key.
var MasterPublicKeyBytes = []byte{
	0x4b, 0x51, 0xda, 0xdd, 0xe1, 0x49, 0x09, 0xac,
	0x5c, 0xd5, 0xf2, 0xd5, 0x2b, 0x03, 0xc5, 0xe8,
	0x85, 0xa8, 0x9d, 0x36, 0xe2, 0x6f, 0xcd, 0x60,
	0xac, 0xc4, 0xdc, 0xe3, 0x27, 0x17, 0x80, 0x3a,
}

const (
	BrowserAILicenseID = "browser-ai-license-active"
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
	ClientName       string    `json:"client_name"`
	Tier             string    `json:"tier"`
	MaxSeats         int       `json:"max_seats"`
	ServerHardwareID string    `json:"server_hardware_id"`
	Features         string    `json:"features"`
	RawEnvelope      string    `gorm:"type:text" json:"raw_envelope"`
	Signature        string    `json:"signature"`
	IssuedAt         time.Time `json:"issued_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	UpdatedBy        string    `json:"updated_by"`
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
	ClientName       string   `json:"client_name"`
	Tier             string   `json:"tier"`
	MaxSeats         int      `json:"max_seats"`
	ServerHardwareID string   `json:"server_hardware_id,omitempty"`
	Features         []string `json:"features"`
	IssuedAt         string   `json:"issued_at"`
	ExpiresAt        string   `json:"expires_at"`
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
	ClientName       string   `json:"client_name"`
	Tier             string   `json:"tier"`
	MaxSeats         int      `json:"max_seats"`
	ActiveSeats      int      `json:"active_seats"`
	RemainingSeats   int      `json:"remaining_seats"`
	ServerHardwareID string   `json:"server_hardware_id,omitempty"`
	HostHardwareID   string   `json:"host_hardware_id"`
	IsHardwareBound  bool     `json:"is_hardware_bound"`
	ExpiresAt        string   `json:"expires_at"`
	IssuedAt         string   `json:"issued_at"`
	Features         []string `json:"features"`
	StatusMessage    string   `json:"status_message"`
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

	// Check Server Hardware ID (Node-Locking) if specified in license
	if strings.TrimSpace(payload.ServerHardwareID) != "" {
		hostID := GetServerHardwareID()
		if !strings.EqualFold(strings.TrimSpace(payload.ServerHardwareID), strings.TrimSpace(hostID)) {
			return &payload, &env, fmt.Errorf("LICENSE_SERVER_MISMATCH: license is locked to server hardware [%s], but current host hardware is [%s]", payload.ServerHardwareID, hostID)
		}
	}

	return &payload, &env, nil
}

// GetActiveLicense returns the current license status and seats usage.
func (m *BrowserAIManager) GetActiveLicense(ctx context.Context) (*LicenseStatusInfo, error) {
	activeCount, _, _, _ := m.CountAgentsStatus(ctx, "", "")
	allocatedSeats := int(activeCount)

	db := m.GetDB()
	var record BrowserAILicenseRecord
	var hasRecord bool
	if db != nil {
		if err := db.WithContext(ctx).Where("id = ?", BrowserAILicenseID).First(&record).Error; err == nil {
			hasRecord = true
		}
	}

	// If no record in DB, attempt reading from fallback file locations
	if !hasRecord {
		fallbackPaths := []string{
			"/app/data/gateway_license.lic",
			filepath.Join(os.Getenv("APP_DIR"), "gateway_license.lic"),
			"data/gateway_license.lic",
			"gateway_license.lic",
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
						Product:        firstNonEmpty(payload.Product, "Gateway - Real-time AI Knowledge Screening & Hazard Audit"),
						ClientName:     firstNonEmpty(payload.ClientName, "Enterprise Organization"),
						Tier:           firstNonEmpty(payload.Tier, "Enterprise On-Premise"),
						MaxSeats:         payload.MaxSeats,
						ActiveSeats:      allocatedSeats,
						RemainingSeats:   rem,
						ServerHardwareID: payload.ServerHardwareID,
						HostHardwareID:   GetServerHardwareID(),
						IsHardwareBound:  payload.ServerHardwareID != "",
						ExpiresAt:        payload.ExpiresAt,
						IssuedAt:         payload.IssuedAt,
						Features:         payload.Features,
						StatusMessage:    fmt.Sprintf("Active (%d/%d seats in use)", allocatedSeats, payload.MaxSeats),
					}, nil
				}
			}
		}

		return &LicenseStatusInfo{
			IsActive:       false,
			IsLicensed:     false,
			IsExpired:      false,
			ActiveSeats:    allocatedSeats,
			RemainingSeats: 0,
			StatusMessage:  "No signed license is activated. Upload a .lic file issued by YesPanchi.",
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
			Product:        firstNonEmpty(record.Product, "Gateway - Real-time AI Knowledge Screening & Hazard Audit"),
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
		Product:        firstNonEmpty(payload.Product, record.Product, "Gateway - Real-time AI Knowledge Screening & Hazard Audit"),
		ClientName:     payload.ClientName,
		Tier:           payload.Tier,
		MaxSeats:         payload.MaxSeats,
		ActiveSeats:      allocatedSeats,
		RemainingSeats:   rem,
		ServerHardwareID: payload.ServerHardwareID,
		HostHardwareID:   GetServerHardwareID(),
		IsHardwareBound:  payload.ServerHardwareID != "",
		ExpiresAt:        payload.ExpiresAt,
		IssuedAt:         payload.IssuedAt,
		Features:         payload.Features,
		StatusMessage:    fmt.Sprintf("Active Enterprise (%d/%d seats in use)", allocatedSeats, payload.MaxSeats),
	}, nil
}

// ActivateLicense validates and activates a new signed license file/token.
func (m *BrowserAIManager) ActivateLicense(ctx context.Context, rawLicense []byte, updatedBy string) (*LicenseStatusInfo, error) {
	payload, env, err := VerifyLicenseEnvelope(rawLicense)
	if err != nil {
		return nil, err
	}

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

	db := m.GetDB()
	if db != nil {
		rec := BrowserAILicenseRecord{
			ID:          BrowserAILicenseID,
			LicenseID:   payload.LicenseID,
			Issuer:      firstNonEmpty(payload.Issuer, "YesPanchi Group of Companies"),
			Product:     firstNonEmpty(payload.Product, "Gateway - Real-time AI Knowledge Screening & Hazard Audit"),
			ClientName:       payload.ClientName,
			Tier:             payload.Tier,
			MaxSeats:         payload.MaxSeats,
			ServerHardwareID: payload.ServerHardwareID,
			Features:         string(featuresJSON),
			RawEnvelope:      string(rawEnvBytes),
			Signature:        env.Signature,
			IssuedAt:         issTime,
			ExpiresAt:        expTime,
			UpdatedAt:        now,
			UpdatedBy:        strings.TrimSpace(updatedBy),
		}

		if err := db.WithContext(ctx).Save(&rec).Error; err != nil {
			return nil, fmt.Errorf("failed to save license record: %w", err)
		}
	}

	// Also backup to file
	appDir := os.Getenv("APP_DIR")
	if appDir == "" {
		appDir = "/app/data"
	}
	licPath := filepath.Join(appDir, "gateway_license.lic")
	_ = os.MkdirAll(appDir, 0755)
	_ = os.WriteFile(licPath, rawEnvBytes, 0644)
	_ = os.WriteFile("gateway_license.lic", rawEnvBytes, 0644)

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
	db := m.GetDB()
	if db != nil {
		// A) Exact Agent ID is already active:
		if agentID != "" {
			var agent BrowserAIAgent
			if db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error == nil {
				if agent.Status == AgentStatusActive {
					return nil // Already active, no new seat consumed!
				}
			}
		}

		// B) Same physical hardware (MAC address + Hostname) already active:
		if mac != "" && hostname != "" {
			var hardwareAgent BrowserAIAgent
			if db.WithContext(ctx).
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

var (
	serverHardwareIDOnce   sync.Once
	cachedServerHardwareID string
)

// GetServerHardwareID returns a deterministic unique hardware fingerprint for the server host.
// Format: SRV-XXXXXXXX-XXXXXXXX
func GetServerHardwareID() string {
	serverHardwareIDOnce.Do(func() {
		cachedServerHardwareID = computeServerHardwareID()
	})
	return cachedServerHardwareID
}

func computeServerHardwareID() string {
	// 1. Try Linux machine-id (/etc/machine-id, /var/lib/dbus/machine-id, DMI product UUID)
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id", "/sys/class/dmi/id/product_uuid"} {
		if b, err := os.ReadFile(p); err == nil {
			trimmed := strings.TrimSpace(string(b))
			if len(trimmed) > 0 {
				return formatHardwareID(trimmed)
			}
		}
	}

	// 2. Try Windows MachineGuid via reg query
	if runtime.GOOS == "windows" {
		if guid := readWindowsMachineGuid(); guid != "" {
			return formatHardwareID(guid)
		}
	}

	// 3. Fallback: Hostname + primary MAC addresses
	var macs []string
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback == 0 && len(iface.HardwareAddr) > 0 {
				macs = append(macs, iface.HardwareAddr.String())
			}
		}
	}
	hostname, _ := os.Hostname()
	combined := fmt.Sprintf("%s|%s|%s", hostname, runtime.GOARCH, strings.Join(macs, ","))
	return formatHardwareID(combined)
}

func readWindowsMachineGuid() string {
	cmd := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "MachineGuid") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return strings.TrimSpace(parts[len(parts)-1])
			}
		}
	}
	return ""
}

func formatHardwareID(raw string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	hexStr := strings.ToUpper(hex.EncodeToString(h[:]))
	return fmt.Sprintf("SRV-%s-%s", hexStr[:8], hexStr[8:16])
}
