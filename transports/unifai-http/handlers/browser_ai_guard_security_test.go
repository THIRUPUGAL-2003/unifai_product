package handlers

import "testing"

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
