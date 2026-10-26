package handlers

import (
	"testing"
)

func TestGuardSecretMustMatch(t *testing.T) {
	if !guardSecretRequired() {
		t.Fatal("Guard authentication must fail closed when no secret is configured")
	}
	if !guardSecretMatches("correct-secret", "correct-secret") {
		t.Fatal("matching Guard secret was rejected")
	}
	if guardSecretMatches("wrong-secret", "correct-secret") {
		t.Fatal("mismatched Guard secret was accepted")
	}
	if guardSecretMatches("", "correct-secret") || guardSecretMatches("anything", "") {
		t.Fatal("empty Guard secret must never authenticate")
	}
}

func TestConfiguredGuardSecretReadsEnvironment(t *testing.T) {
	t.Setenv("GATEWAY_GUARD_SECRET", "  test-shared-secret  ")
	if got := configuredGuardSecret(); got != "test-shared-secret" {
		t.Fatalf("configuredGuardSecret() = %q, want trimmed environment value", got)
	}
}

func TestGuardSyncPollingDoesNotStarveIntercepts(t *testing.T) {
	ip := "203.0.113.77"
	for i := 0; i < 500; i++ {
		if isGuardSyncRateLimited(ip) {
			t.Fatalf("policy sync limited after %d requests", i)
		}
	}
	if isBrowserAIRateLimited(ip) {
		t.Fatal("policy sync polling must not consume the intercept bucket")
	}
}

func TestBrowserAIRateLimitStillCapsUnkeyedFlood(t *testing.T) {
	ip := "203.0.113.78"
	limited := false
	for i := 0; i < browserAIMaxRequestsPerIP+5; i++ {
		limited = isBrowserAIRateLimited(ip)
	}
	if !limited {
		t.Fatal("unkeyed flood must be rate limited")
	}
}
