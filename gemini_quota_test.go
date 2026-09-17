package captcha

import (
	"testing"
	"time"
)

// A spent daily quota must not be retried a minute later. Google still sends a
// short RetryInfo delay in that case, so honouring it repeats the same failure
// until the day ends.
func TestPerDayQuotaWaitsForTheDailyReset(t *testing.T) {
	body := []byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[
	  {"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[
	    {"quotaMetric":"generativelanguage.googleapis.com/generate_content_free_tier_requests",
	     "quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier","quotaValue":"20"}]},
	  {"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"28s"}]}}`)

	now := time.Date(2026, 9, 17, 10, 0, 0, 0, dailyResetZone)
	got, kind := quotaCooldown(body, "", now)

	if kind != quotaPerDay {
		t.Fatalf("kind = %v, want quotaPerDay", kind)
	}
	if want := 14 * time.Hour; got != want {
		t.Errorf("cooldown = %v, want %v", got, want)
	}
}

// A per-minute quota refills inside the delay Google suggests, so that delay is
// used instead of the fixed window.
func TestPerMinuteQuotaHonoursRetryInfo(t *testing.T) {
	body := []byte(`{"error":{"code":429,"details":[
	  {"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[
	    {"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier","quotaValue":"15"}]},
	  {"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"12.5s"}]}}`)

	got, kind := quotaCooldown(body, "", time.Now())

	if kind != quotaPerMinute {
		t.Fatalf("kind = %v, want quotaPerMinute", kind)
	}
	if want := 12500 * time.Millisecond; got != want {
		t.Errorf("cooldown = %v, want %v", got, want)
	}
}

// A body with no quota detail falls back to the Retry-After header, which is
// the behaviour every non-Gemini 429 already relied on.
func TestUnknownQuotaFallsBackToRetryAfter(t *testing.T) {
	got, kind := quotaCooldown([]byte(`{"error":{"code":429}}`), "30", time.Now())

	if kind != quotaUnknown {
		t.Fatalf("kind = %v, want quotaUnknown", kind)
	}
	if want := 30 * time.Second; got != want {
		t.Errorf("cooldown = %v, want %v", got, want)
	}
}

// An empty body and no header leave the fixed per-minute window.
func TestEmptyBodyUsesTheWindow(t *testing.T) {
	got, _ := quotaCooldown(nil, "", time.Now())
	if got != rpmWindow {
		t.Errorf("cooldown = %v, want %v", got, rpmWindow)
	}
}

// The reset is the next midnight in the reset zone, never a negative wait.
func TestDailyResetIsAlwaysAhead(t *testing.T) {
	for _, hour := range []int{0, 1, 12, 23} {
		now := time.Date(2026, 9, 17, hour, 30, 0, 0, dailyResetZone)
		if got := timeUntilDailyReset(now); got <= 0 || got > 24*time.Hour {
			t.Errorf("hour %d: wait = %v, want between 0 and 24h", hour, got)
		}
	}
}
