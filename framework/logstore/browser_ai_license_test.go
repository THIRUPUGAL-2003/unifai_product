package logstore

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
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
		Format:     "raksha_enterprise_license_v1",
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
		Format:     "raksha_enterprise_license_v1",
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

	// Issue a 2-seat license
	payload := EnterpriseLicensePayload{
		Version:    "1.0",
		ClientName: "Quota Test Corp",
		Tier:       "Enterprise On-Premise",
		MaxSeats:   2,
		IssuedAt:   time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
	rawPayload, _ := json.Marshal(payload)
	sig := ed25519.Sign(privKey, rawPayload)
	env := EnterpriseLicenseEnvelope{
		Format:     "raksha_enterprise_license_v1",
		Payload:    payload,
		PayloadB64: base64.StdEncoding.EncodeToString(rawPayload),
		Signature:  base64.StdEncoding.EncodeToString(sig),
	}
	envBytes, _ := json.Marshal(env)

	_, err := mgr.ActivateLicense(ctx, envBytes, "admin")
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
