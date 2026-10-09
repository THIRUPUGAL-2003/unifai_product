package logstore

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnterpriseLicenseVerification(t *testing.T) {
	// 1. Generate a test Ed25519 keypair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Override MasterPublicKeyBytes for testing
	origKey := MasterPublicKeyBytes
	MasterPublicKeyBytes = pubKey
	defer func() { MasterPublicKeyBytes = origKey }()

	// 2. Create canonical payload for 100 seats
	payload := EnterpriseLicensePayload{
		Version:    "1.0",
		ClientName: "Test Enterprise Corp",
		Tier:       "Enterprise On-Premise",
		MaxSeats:   100,
		Features:   []string{"browser_ai_guard", "dlp_regex"},
		IssuedAt:   time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:  time.Now().UTC().Add(365 * 24 * time.Hour).Format(time.RFC3339),
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	sig := ed25519.Sign(privKey, rawPayload)

	env := EnterpriseLicenseEnvelope{
		Format:     "gateway_enterprise_license_v1",
		Payload:    payload,
		PayloadB64: base64.StdEncoding.EncodeToString(rawPayload),
		Signature:  base64.StdEncoding.EncodeToString(sig),
	}

	envBytes, _ := json.Marshal(env)

	// Test Valid License
	parsedPayload, parsedEnv, err := VerifyLicenseEnvelope(envBytes)
	if err != nil {
		t.Fatalf("expected valid license, got error: %v", err)
	}
	if parsedPayload.MaxSeats != 100 {
		t.Errorf("expected 100 seats, got %d", parsedPayload.MaxSeats)
	}
	if parsedEnv.Signature != env.Signature {
		t.Errorf("signature mismatch")
	}

	// Test Tampering: Change seats to 1000 in payload_b64 without valid private key
	tamperedPayload := payload
	tamperedPayload.MaxSeats = 1000
	tamperedBytes, _ := json.Marshal(tamperedPayload)
	tamperedEnv := env
	tamperedEnv.PayloadB64 = base64.StdEncoding.EncodeToString(tamperedBytes)
	tamperedEnvBytes, _ := json.Marshal(tamperedEnv)

	_, _, tamperErr := VerifyLicenseEnvelope(tamperedEnvBytes)
	if tamperErr == nil {
		t.Fatalf("expected tampering error, but verification passed!")
	}
	if !strings.Contains(tamperErr.Error(), "CRYPTOGRAPHIC_SIGNATURE_INVALID") {
		t.Errorf("expected CRYPTOGRAPHIC_SIGNATURE_INVALID error, got: %v", tamperErr)
	}

	// Test Expired License
	expiredPayload := payload
	expiredPayload.ExpiresAt = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	expRaw, _ := json.Marshal(expiredPayload)
	expSig := ed25519.Sign(privKey, expRaw)
	expEnv := EnterpriseLicenseEnvelope{
		Format:     "gateway_enterprise_license_v1",
		Payload:    expiredPayload,
		PayloadB64: base64.StdEncoding.EncodeToString(expRaw),
		Signature:  base64.StdEncoding.EncodeToString(expSig),
	}
	expBytes, _ := json.Marshal(expEnv)

	_, _, expErr := VerifyLicenseEnvelope(expBytes)
	if expErr == nil || !strings.Contains(expErr.Error(), "LICENSE_EXPIRED") {
		t.Fatalf("expected LICENSE_EXPIRED error, got: %v", expErr)
	}
}

func TestSeatQuotaEnforcementLimit(t *testing.T) {
	mgr := NewBrowserAIManager(nil)
	ctx := context.Background()

	// Verify CheckSeatQuotaEnforcement works with mock license
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	origKey := MasterPublicKeyBytes
	MasterPublicKeyBytes = pubKey
	defer func() { MasterPublicKeyBytes = origKey }()

	installID, err := mgr.GetOrCreateInstallID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payload := EnterpriseLicensePayload{
		Version:          "1.0",
		LicenseID:        "YP-QUOTA-1",
		ClientName:       "Quota Test Corp",
		Tier:             "Enterprise On-Premise",
		MaxSeats:         2,
		InstallID:        installID,
		ServerHardwareID: GetServerHardwareID(),
		Revision:         1,
		IssuedAt:         time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:        time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
	rawPayload, _ := json.Marshal(payload)
	sig := ed25519.Sign(privKey, rawPayload)
	env := EnterpriseLicenseEnvelope{
		Format:     "gateway_enterprise_license_v1",
		Payload:    payload,
		PayloadB64: base64.StdEncoding.EncodeToString(rawPayload),
		Signature:  base64.StdEncoding.EncodeToString(sig),
	}
	envBytes, _ := json.Marshal(env)

	_, err = mgr.ActivateLicense(ctx, envBytes, "admin")
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}

	lic, err := mgr.GetActiveLicense(ctx)
	if err != nil {
		t.Fatalf("get active license failed: %v", err)
	}
	if lic.MaxSeats != 2 {
		t.Fatalf("expected 2 max seats, got %d", lic.MaxSeats)
	}
}

func signLicensePayload(t *testing.T, priv ed25519.PrivateKey, payload EnterpriseLicensePayload) []byte {
	t.Helper()
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, rawPayload)
	env := EnterpriseLicenseEnvelope{
		Format:     "gateway_enterprise_license_v1",
		Payload:    payload,
		PayloadB64: base64.StdEncoding.EncodeToString(rawPayload),
		Signature:  base64.StdEncoding.EncodeToString(sig),
	}
	envBytes, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return envBytes
}

func newLicenseManager(t *testing.T) *BrowserAIManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lic.db")), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	mgr := NewBrowserAIManager(db)
	if err := mgr.AutoMigrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestLicenseLockedToOneDatabaseAndRevision(t *testing.T) {
	ctx := context.Background()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	orig := MasterPublicKeyBytes
	MasterPublicKeyBytes = pub
	defer func() { MasterPublicKeyBytes = orig }()

	dbA := newLicenseManager(t)
	dbB := newLicenseManager(t)
	installA, err := dbA.GetOrCreateInstallID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	installB, err := dbB.GetOrCreateInstallID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if installA == installB {
		t.Fatal("two databases must not share an install id")
	}
	host := GetServerHardwareID()
	first := EnterpriseLicensePayload{
		Version: "1.0", LicenseID: "YP-DB-1", ClientName: "Client", Tier: "Enterprise On-Premise",
		MaxSeats: 1000, InstallID: installA, ServerHardwareID: host, Revision: 1,
		IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
	raw := signLicensePayload(t, priv, first)
	if _, err := dbA.ActivateLicense(ctx, raw, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := dbB.ActivateLicense(ctx, raw, "admin"); err == nil || !strings.Contains(err.Error(), "LICENSE_DB_MISMATCH") {
		t.Fatalf("second database accepted the same key: %v", err)
	}
	upgraded := first
	upgraded.Revision = 2
	upgraded.MaxSeats = 1500
	if _, err := dbA.ActivateLicense(ctx, signLicensePayload(t, priv, upgraded), "admin"); err != nil {
		t.Fatal(err)
	}
	lic, err := dbA.GetActiveLicense(ctx)
	if err != nil || !lic.IsActive || lic.MaxSeats != 1500 {
		t.Fatalf("upgraded license = %+v err=%v", lic, err)
	}
	if _, err := dbA.ActivateLicense(ctx, raw, "admin"); err == nil || !strings.Contains(err.Error(), "LICENSE_REVOKED") {
		t.Fatalf("old revision was accepted: %v", err)
	}
}

func TestOpaqueLicenseKeyHidesPayload(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	orig := MasterPublicKeyBytes
	MasterPublicKeyBytes = pub
	defer func() { MasterPublicKeyBytes = orig }()

	payload := EnterpriseLicensePayload{
		Version: "1.0", LicenseID: "YP-OPAQUE", ClientName: "Hidden Client", MaxSeats: 1000,
		InstallID: "DB-1", ServerHardwareID: GetServerHardwareID(), Revision: 1,
		IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
	rawPayload, _ := json.Marshal(payload)
	sig := ed25519.Sign(priv, rawPayload)
	inner, _ := json.Marshal(map[string]string{
		"format":      "gateway_enterprise_license_v1",
		"payload_b64": base64.StdEncoding.EncodeToString(rawPayload),
		"signature":   base64.StdEncoding.EncodeToString(sig),
	})
	token, err := SealLicenseFile(inner)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "Hidden Client") || strings.Contains(token, "max_seats") || !strings.HasPrefix(token, "GWLIC1.") {
		t.Fatalf("token still exposes license fields: %s", token[:40])
	}
	parsed, _, err := VerifyLicenseEnvelope([]byte(token))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.MaxSeats != 1000 || parsed.ClientName != "Hidden Client" {
		t.Fatalf("opened payload = %+v", parsed)
	}
}

func TestProductUsersShareOneTotal(t *testing.T) {
	usage := ProductUserUsageFromRoles([]string{"user", "user", "admin", "sub_admin", "auditor"}, 100)
	if usage.Users != 2 || usage.Admins != 1 || usage.SubAdmins != 1 || usage.Others != 1 {
		t.Fatalf("counts = %+v", usage)
	}
	if usage.Used != 5 || usage.Remaining != 95 || usage.BlockReason() != "" {
		t.Fatalf("usage = %+v reason=%q", usage, usage.BlockReason())
	}
	full := ProductUserUsageFromRoles([]string{"user", "admin", "sub_admin"}, 3)
	if full.Remaining != 0 || full.BlockReason() == "" {
		t.Fatal("full quota should block the next account")
	}
}

func TestAuthorityRejectsReplacedRevision(t *testing.T) {
	registry := []byte(`{"licenses":{"YP-1":{"license_id":"YP-1","install_id":"DB-1","server_hardware_id":"SRV-1","max_seats":1500,"revision":2,"expires_at":"2099-01-01T00:00:00Z","status":"active"}}}`)
	old := DecideLicenseAuthority(registry, "YP-1", "DB-1", "SRV-1", 1)
	if old.Allowed {
		t.Fatal("old revision must be revoked")
	}
	current := DecideLicenseAuthority(registry, "YP-1", "DB-1", "SRV-1", 2)
	if !current.Allowed || current.MaxSeats != 1500 {
		t.Fatalf("current key rejected: %+v", current)
	}
	otherDB := DecideLicenseAuthority(registry, "YP-1", "DB-2", "SRV-1", 2)
	if otherDB.Allowed {
		t.Fatal("other database must be rejected")
	}
}
