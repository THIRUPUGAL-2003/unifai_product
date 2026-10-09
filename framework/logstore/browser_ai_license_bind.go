package logstore

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const BrowserAIInstallRowID = "primary"

// BrowserAIInstallRecord is the one database identity. A dump keeps this id.
// A second database gets a different id, so the same license file cannot activate there.
type BrowserAIInstallRecord struct {
	ID                string `gorm:"primaryKey"`
	InstallID         string `gorm:"uniqueIndex"`
	AcceptedLicenseID string
	AcceptedRevision  int
	LastAuthorityOK   *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (BrowserAIInstallRecord) TableName() string {
	return "browser_ai_install"
}

type memoryInstallState struct {
	id        string
	licenseID string
	revision  int
	lastOK    time.Time
}

var memoryInstall memoryInstallState
var memoryInstallMu sync.Mutex

func (m *BrowserAIManager) GetOrCreateInstallID(ctx context.Context) (string, error) {
	db := m.GetDB()
	if db == nil {
		memoryInstallMu.Lock()
		defer memoryInstallMu.Unlock()
		if memoryInstall.id == "" {
			memoryInstall.id = "DB-" + strings.ToUpper(uuid.New().String())
		}
		return memoryInstall.id, nil
	}
	if err := db.WithContext(ctx).AutoMigrate(&BrowserAIInstallRecord{}); err != nil {
		return "", err
	}
	var row BrowserAIInstallRecord
	err := db.WithContext(ctx).Where("id = ?", BrowserAIInstallRowID).First(&row).Error
	if err == nil && strings.TrimSpace(row.InstallID) != "" {
		return row.InstallID, nil
	}
	now := time.Now().UTC()
	row = BrowserAIInstallRecord{
		ID:        BrowserAIInstallRowID,
		InstallID: "DB-" + strings.ToUpper(uuid.New().String()),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.WithContext(ctx).Save(&row).Error; err != nil {
		return "", err
	}
	return row.InstallID, nil
}

func (m *BrowserAIManager) installRow(ctx context.Context) (BrowserAIInstallRecord, error) {
	if _, err := m.GetOrCreateInstallID(ctx); err != nil {
		return BrowserAIInstallRecord{}, err
	}
	db := m.GetDB()
	if db == nil {
		memoryInstallMu.Lock()
		defer memoryInstallMu.Unlock()
		return BrowserAIInstallRecord{
			ID:                BrowserAIInstallRowID,
			InstallID:         memoryInstall.id,
			AcceptedLicenseID: memoryInstall.licenseID,
			AcceptedRevision:  memoryInstall.revision,
			LastAuthorityOK:   timePtr(memoryInstall.lastOK),
		}, nil
	}
	var row BrowserAIInstallRecord
	err := db.WithContext(ctx).Where("id = ?", BrowserAIInstallRowID).First(&row).Error
	return row, err
}

func (m *BrowserAIManager) assertLicenseBinding(ctx context.Context, payload *EnterpriseLicensePayload) error {
	if payload == nil {
		return fmt.Errorf("LICENSE_UNBOUND: empty license")
	}
	if strings.TrimSpace(payload.InstallID) == "" || strings.TrimSpace(payload.ServerHardwareID) == "" {
		return fmt.Errorf("LICENSE_UNBOUND: license must be issued for this database install_id and this server_hardware_id")
	}
	installID, err := m.GetOrCreateInstallID(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(payload.InstallID), installID) {
		return fmt.Errorf("LICENSE_DB_MISMATCH: license is locked to database [%s], but this database is [%s]", payload.InstallID, installID)
	}
	hostID := GetServerHardwareID()
	if !strings.EqualFold(strings.TrimSpace(payload.ServerHardwareID), strings.TrimSpace(hostID)) {
		return fmt.Errorf("LICENSE_SERVER_MISMATCH: license is locked to server hardware [%s], but current host hardware is [%s]", payload.ServerHardwareID, hostID)
	}
	rev := payload.Revision
	if rev <= 0 {
		rev = 1
	}
	row, err := m.installRow(ctx)
	if err != nil {
		return err
	}
	if row.AcceptedLicenseID != "" {
		if row.AcceptedLicenseID == payload.LicenseID && rev < row.AcceptedRevision {
			return fmt.Errorf("LICENSE_REVOKED: this key was replaced by revision %d", row.AcceptedRevision)
		}
		if row.AcceptedLicenseID != payload.LicenseID && rev <= row.AcceptedRevision {
			return fmt.Errorf("LICENSE_REVOKED: a newer key is already active for this database")
		}
	}
	return nil
}

func (m *BrowserAIManager) rememberAcceptedLicense(ctx context.Context, payload *EnterpriseLicensePayload) error {
	if payload == nil {
		return nil
	}
	rev := payload.Revision
	if rev <= 0 {
		rev = 1
	}
	db := m.GetDB()
	if db == nil {
		memoryInstallMu.Lock()
		defer memoryInstallMu.Unlock()
		if memoryInstall.licenseID == payload.LicenseID && rev < memoryInstall.revision {
			return fmt.Errorf("LICENSE_REVOKED: this key was replaced by revision %d", memoryInstall.revision)
		}
		memoryInstall.licenseID = payload.LicenseID
		if rev > memoryInstall.revision {
			memoryInstall.revision = rev
		}
		return nil
	}
	var row BrowserAIInstallRecord
	if err := db.WithContext(ctx).Where("id = ?", BrowserAIInstallRowID).First(&row).Error; err != nil {
		return err
	}
	if row.AcceptedLicenseID == payload.LicenseID && rev < row.AcceptedRevision {
		return fmt.Errorf("LICENSE_REVOKED: this key was replaced by revision %d", row.AcceptedRevision)
	}
	if row.AcceptedLicenseID != payload.LicenseID && row.AcceptedLicenseID != "" && rev <= row.AcceptedRevision {
		return fmt.Errorf("LICENSE_REVOKED: a newer key is already active for this database")
	}
	row.AcceptedLicenseID = payload.LicenseID
	if rev > row.AcceptedRevision {
		row.AcceptedRevision = rev
	}
	row.UpdatedAt = time.Now().UTC()
	return db.WithContext(ctx).Save(&row).Error
}

func (m *BrowserAIManager) finishLicenseStatus(ctx context.Context, info *LicenseStatusInfo, payload *EnterpriseLicensePayload) (*LicenseStatusInfo, error) {
	if info == nil {
		return nil, nil
	}
	if id, err := m.GetOrCreateInstallID(ctx); err == nil {
		info.InstallID = id
	}
	if info.HostHardwareID == "" {
		info.HostHardwareID = GetServerHardwareID()
	}
	if payload != nil && payload.Revision > 0 {
		info.Revision = payload.Revision
	}
	if payload != nil && info.IsActive {
		if err := m.assertLicenseBinding(ctx, payload); err != nil {
			info.IsActive = false
			info.RemainingSeats = 0
			info.StatusMessage = err.Error()
			info.AuthorityStatus = "rejected"
			return info, nil
		}
		m.applyAuthority(ctx, payload, info)
	}
	if info.AuthorityStatus == "" {
		info.AuthorityStatus = "local"
	}
	return info, nil
}

func (m *BrowserAIManager) BlockAgentIfLicenseRevoked(ctx context.Context) error {
	lic, err := m.GetActiveLicense(ctx)
	if err != nil {
		return fmt.Errorf("LICENSE_CHECK_FAILED: %w", err)
	}
	if lic == nil || !lic.IsLicensed {
		return nil
	}
	if !lic.IsActive {
		msg := strings.TrimSpace(lic.StatusMessage)
		if msg == "" {
			msg = "LICENSE_INACTIVE"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

type LicenseAuthorityDecision struct {
	LicenseID string `json:"license_id"`
	Allowed   bool   `json:"allowed"`
	Revision  int    `json:"revision"`
	MaxSeats  int    `json:"max_seats"`
	Reason    string `json:"reason"`
	CheckedAt string `json:"checked_at"`
}

type licenseRegistryFile struct {
	Licenses map[string]licenseRegistryRow `json:"licenses"`
}

type licenseRegistryRow struct {
	LicenseID        string `json:"license_id"`
	InstallID        string `json:"install_id"`
	ServerHardwareID string `json:"server_hardware_id"`
	MaxSeats         int    `json:"max_seats"`
	Revision         int    `json:"revision"`
	ExpiresAt        string `json:"expires_at"`
	Status           string `json:"status"`
}

// DecideLicenseAuthority is the vendor-side check. Only the current registry row is allowed.
func DecideLicenseAuthority(registryJSON []byte, licenseID, installID, serverID string, revision int) LicenseAuthorityDecision {
	now := time.Now().UTC().Format(time.RFC3339)
	decision := LicenseAuthorityDecision{LicenseID: licenseID, CheckedAt: now, Reason: "unknown license"}
	var reg licenseRegistryFile
	if err := json.Unmarshal(registryJSON, &reg); err != nil || reg.Licenses == nil {
		decision.Reason = "authority registry unavailable"
		return decision
	}
	row, ok := reg.Licenses[licenseID]
	if !ok {
		decision.Reason = "license is not in the vendor registry"
		return decision
	}
	decision.Revision = row.Revision
	decision.MaxSeats = row.MaxSeats
	if !strings.EqualFold(row.Status, "active") {
		decision.Reason = "license revoked"
		return decision
	}
	if !strings.EqualFold(strings.TrimSpace(row.InstallID), strings.TrimSpace(installID)) {
		decision.Reason = "database install_id does not match the issued key"
		return decision
	}
	if !strings.EqualFold(strings.TrimSpace(row.ServerHardwareID), strings.TrimSpace(serverID)) {
		decision.Reason = "server hardware id does not match the issued key"
		return decision
	}
	if revision <= 0 {
		revision = 1
	}
	if revision != row.Revision {
		decision.Reason = fmt.Sprintf("key revision %d was replaced by revision %d", revision, row.Revision)
		return decision
	}
	if row.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, row.ExpiresAt); err == nil && time.Now().UTC().After(exp) {
			decision.Reason = "license expired"
			return decision
		}
	}
	decision.Allowed = true
	decision.Reason = "active"
	return decision
}

func SignLicenseAuthorityDecision(privateKey ed25519.PrivateKey, decision LicenseAuthorityDecision) (payloadB64, signatureB64 string, err error) {
	raw, err := json.Marshal(decision)
	if err != nil {
		return "", "", err
	}
	sig := ed25519.Sign(privateKey, raw)
	return base64.StdEncoding.EncodeToString(raw), base64.StdEncoding.EncodeToString(sig), nil
}

func ParseLicenseAuthorityPrivateKey(pemBytes []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("private key pem is empty")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ed25519")
	}
	return key, nil
}

func verifyAuthorityEnvelope(body []byte) (LicenseAuthorityDecision, error) {
	var env struct {
		PayloadB64 string `json:"payload_b64"`
		Signature  string `json:"signature"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return LicenseAuthorityDecision{}, err
	}
	raw, err := base64.StdEncoding.DecodeString(env.PayloadB64)
	if err != nil {
		return LicenseAuthorityDecision{}, err
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		return LicenseAuthorityDecision{}, err
	}
	if !ed25519.Verify(ed25519.PublicKey(MasterPublicKeyBytes), raw, sig) {
		return LicenseAuthorityDecision{}, fmt.Errorf("authority signature invalid")
	}
	var decision LicenseAuthorityDecision
	if err := json.Unmarshal(raw, &decision); err != nil {
		return LicenseAuthorityDecision{}, err
	}
	return decision, nil
}

var authorityCache struct {
	sync.Mutex
	key      string
	at       time.Time
	decision LicenseAuthorityDecision
	err      error
}

func (m *BrowserAIManager) applyAuthority(ctx context.Context, payload *EnterpriseLicensePayload, info *LicenseStatusInfo) {
	authorityURL := strings.TrimSpace(os.Getenv("LICENSE_AUTHORITY_URL"))
	if authorityURL == "" || payload == nil || info == nil {
		info.AuthorityStatus = "local"
		return
	}
	rev := payload.Revision
	if rev <= 0 {
		rev = 1
	}
	decision, err := cachedAuthority(authorityURL, payload.LicenseID, info.InstallID, info.HostHardwareID, rev)
	if err != nil {
		if m.authorityGraceOK(ctx) {
			info.AuthorityStatus = "grace"
			info.StatusMessage = info.StatusMessage + " (vendor check unreachable, grace period)"
			return
		}
		info.IsActive = false
		info.RemainingSeats = 0
		info.AuthorityStatus = "unreachable"
		info.StatusMessage = "LICENSE_AUTHORITY_UNREACHABLE: vendor license check failed"
		return
	}
	if !decision.Allowed {
		info.IsActive = false
		info.RemainingSeats = 0
		info.AuthorityStatus = "revoked"
		info.StatusMessage = "LICENSE_REVOKED: " + decision.Reason
		return
	}
	_ = m.markAuthorityOK(ctx)
	info.AuthorityStatus = "allowed"
}

func cachedAuthority(authorityURL, licenseID, installID, serverID string, revision int) (LicenseAuthorityDecision, error) {
	key := licenseID + "|" + installID + "|" + serverID + "|" + strconv.Itoa(revision)
	authorityCache.Lock()
	if authorityCache.key == key && time.Since(authorityCache.at) < 60*time.Second {
		decision, err := authorityCache.decision, authorityCache.err
		authorityCache.Unlock()
		return decision, err
	}
	authorityCache.Unlock()

	decision, err := callLicenseAuthority(authorityURL, licenseID, installID, serverID, revision)
	authorityCache.Lock()
	authorityCache.key = key
	authorityCache.at = time.Now()
	authorityCache.decision = decision
	authorityCache.err = err
	authorityCache.Unlock()
	return decision, err
}

func callLicenseAuthority(authorityURL, licenseID, installID, serverID string, revision int) (LicenseAuthorityDecision, error) {
	body, _ := json.Marshal(map[string]any{
		"license_id":         licenseID,
		"install_id":         installID,
		"server_hardware_id": serverID,
		"revision":           revision,
	})
	req, err := http.NewRequest(http.MethodPost, authorityURL, bytes.NewReader(body))
	if err != nil {
		return LicenseAuthorityDecision{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return LicenseAuthorityDecision{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return LicenseAuthorityDecision{}, fmt.Errorf("authority status %d", resp.StatusCode)
	}
	return verifyAuthorityEnvelope(raw)
}

func (m *BrowserAIManager) authorityGraceOK(ctx context.Context) bool {
	hours := 24 * 7
	if raw := strings.TrimSpace(os.Getenv("LICENSE_AUTHORITY_GRACE_HOURS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			hours = n
		}
	}
	row, err := m.installRow(ctx)
	if err != nil || row.LastAuthorityOK == nil || row.LastAuthorityOK.IsZero() {
		return false
	}
	return time.Since(row.LastAuthorityOK.UTC()) < time.Duration(hours)*time.Hour
}

func (m *BrowserAIManager) markAuthorityOK(ctx context.Context) error {
	now := time.Now().UTC()
	db := m.GetDB()
	if db == nil {
		memoryInstallMu.Lock()
		memoryInstall.lastOK = now
		memoryInstallMu.Unlock()
		return nil
	}
	return db.WithContext(ctx).Model(&BrowserAIInstallRecord{}).
		Where("id = ?", BrowserAIInstallRowID).
		Updates(map[string]any{"last_authority_ok": now, "updated_at": now}).Error
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	out := t
	return &out
}
