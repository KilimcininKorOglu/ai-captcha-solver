package captcha

import (
	"encoding/json"
	"strings"
	"time"
)

// dailyResetZone is the fixed offset the Gemini free tier daily quota resets
// on. Google resets it at midnight Pacific time. The offset is fixed rather
// than loaded from the tz database, because an embedded target often ships no
// zoneinfo and LoadLocation would fail there. During daylight saving the
// computed reset is one hour late; the next 429 sets a new cooldown, so the key
// is never held past the real reset by more than that hour.
var dailyResetZone = time.FixedZone("PST", -8*60*60)

// quotaKind says which quota window a 429 exhausted.
type quotaKind int

const (
	quotaUnknown quotaKind = iota
	quotaPerMinute
	quotaPerDay
)

// quotaCooldown reads how long the key must rest from a 429 response body. A
// per-day quota does not refill inside the delay Google suggests, so the key
// rests until the daily reset; retrying every minute against a spent daily
// quota only repeats the same failure until the day ends.
func quotaCooldown(body []byte, retryAfter string, now time.Time) (time.Duration, quotaKind) {
	var ge geminiError
	_ = json.Unmarshal(body, &ge)

	kind := quotaKindOf(ge)
	if kind == quotaPerDay {
		return timeUntilDailyReset(now), kind
	}
	if d, ok := retryInfoDelay(ge); ok {
		return d, kind
	}
	return parseRetryAfter(retryAfter, rpmWindow), kind
}

// quotaKindOf classifies the exhausted quota from the QuotaFailure detail.
func quotaKindOf(ge geminiError) quotaKind {
	for _, detail := range ge.Error.Details {
		for _, v := range detail.Violations {
			switch {
			case strings.Contains(v.QuotaID, "PerDay"):
				return quotaPerDay
			case strings.Contains(v.QuotaID, "PerMinute"):
				return quotaPerMinute
			}
		}
	}
	return quotaUnknown
}

// retryInfoDelay returns the delay Google asked for in the RetryInfo detail.
func retryInfoDelay(ge geminiError) (time.Duration, bool) {
	for _, detail := range ge.Error.Details {
		if detail.RetryDelay == "" {
			continue
		}
		if d, err := time.ParseDuration(detail.RetryDelay); err == nil && d > 0 {
			return d, true
		}
	}
	return 0, false
}

// timeUntilDailyReset returns the wait to the next midnight in the reset zone.
func timeUntilDailyReset(now time.Time) time.Duration {
	local := now.In(dailyResetZone)
	next := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, dailyResetZone)
	return next.Sub(local)
}

// String names the quota window for a log line.
func (k quotaKind) String() string {
	switch k {
	case quotaPerDay:
		return "daily quota"
	case quotaPerMinute:
		return "per-minute quota"
	default:
		return "rate limit"
	}
}
