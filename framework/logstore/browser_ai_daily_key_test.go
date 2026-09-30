package logstore

import (
	"testing"
	"time"
)

func TestDailyUninstallKey_IsExpired(t *testing.T) {
	// 1. Nil or zero time is expired
	if !IsAgentUninstallKeyExpired(nil) {
		t.Errorf("expected nil to be expired")
	}
	zero := time.Time{}
	if !IsAgentUninstallKeyExpired(&zero) {
		t.Errorf("expected zero time to be expired")
	}

	// 2. Fresh key created right now is NOT expired
	now := time.Now().UTC()
	if IsAgentUninstallKeyExpired(&now) {
		t.Errorf("expected fresh key not to be expired")
	}

	// 3. Key from 25 hours ago IS expired
	past25h := now.Add(-25 * time.Hour)
	if !IsAgentUninstallKeyExpired(&past25h) {
		t.Errorf("expected key from 25h ago to be expired")
	}

	// 4. Key from yesterday IS expired
	yesterday := now.AddDate(0, 0, -1)
	if !IsAgentUninstallKeyExpired(&yesterday) {
		t.Errorf("expected key from yesterday to be expired")
	}

	// 5. Key from 1 hour ago IS NOT expired
	past1h := now.Add(-1 * time.Hour)
	if past1h.Format("2006-01-02") == now.Format("2006-01-02") {
		if IsAgentUninstallKeyExpired(&past1h) {
			t.Errorf("expected key from 1h ago on same day to be valid")
		}
	}
}

func TestDailyUninstallKey_AssignAgentUninstallKey(t *testing.T) {
	agent := &BrowserAIAgent{
		ID:       "agent-unit-test-1",
		Hostname: "laptop-alice",
	}

	plain, err := assignAgentUninstallKey(agent)
	if err != nil {
		t.Fatalf("assignAgentUninstallKey returned error: %v", err)
	}

	// Verify plaintext format
	if plain == "" {
		t.Fatalf("expected non-empty plaintext key")
	}

	// Verify hash was computed and matches
	if agent.UninstallKeyHash != hashUninstallKey(plain) {
		t.Errorf("expected hash to match plain key")
	}

	// Verify encryption was performed and can be decrypted
	decrypted, err := openAgentUninstallKey(agent.UninstallKeyEnc)
	if err != nil {
		t.Fatalf("failed to decrypt sealed key: %v", err)
	}
	if decrypted != plain {
		t.Errorf("expected decrypted key %q to equal plaintext %q", decrypted, plain)
	}

	// Verify timestamp was set
	if agent.UninstallKeyRotatedAt == nil {
		t.Fatalf("expected UninstallKeyRotatedAt to be set")
	}
	if IsAgentUninstallKeyExpired(agent.UninstallKeyRotatedAt) {
		t.Errorf("newly assigned key must not be expired")
	}

	// Verify flag
	if !agent.HasUninstallKey {
		t.Errorf("expected HasUninstallKey to be true")
	}
}

func TestDailyUninstallKey_ExpirationSimulation(t *testing.T) {
	agent := &BrowserAIAgent{
		ID:       "agent-unit-test-2",
		Hostname: "laptop-bob",
	}
	plain, err := assignAgentUninstallKey(agent)
	if err != nil {
		t.Fatalf("assignAgentUninstallKey failed: %v", err)
	}

	// Fresh key matches
	if hashUninstallKey(plain) != agent.UninstallKeyHash {
		t.Errorf("expected fresh key hash to match")
	}
	if IsAgentUninstallKeyExpired(agent.UninstallKeyRotatedAt) {
		t.Errorf("expected key to not be expired yet")
	}

	// Simulate passage of 25 hours
	expiredTime := time.Now().UTC().Add(-25 * time.Hour)
	agent.UninstallKeyRotatedAt = &expiredTime

	// Key is now expired
	if !IsAgentUninstallKeyExpired(agent.UninstallKeyRotatedAt) {
		t.Errorf("expected key to be expired after 25h")
	}

	// Simulating auto-rotation: assignAgentUninstallKey gives a fresh key
	newPlain, err := assignAgentUninstallKey(agent)
	if err != nil {
		t.Fatalf("re-assign failed: %v", err)
	}
	if newPlain == plain {
		t.Errorf("expected new daily key to differ from old key")
	}
	if IsAgentUninstallKeyExpired(agent.UninstallKeyRotatedAt) {
		t.Errorf("expected newly rotated key to be valid")
	}
}
