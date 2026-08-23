package api

import (
	"testing"
	"time"
)

func TestLoginThrottleLocksAccountAndReleasesAfterWindow(t *testing.T) {
	var throttle loginThrottle
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	keys := loginThrottleKeys("10.0.0.9", "Operator")

	for i := 0; i < loginAccountFailures-1; i++ {
		throttle.recordFailure(now, keys)
		if wait := throttle.retryAfter(now, keys); wait != 0 {
			t.Fatalf("locked out after %d failures, want %d", i+1, loginAccountFailures)
		}
	}
	throttle.recordFailure(now, keys)
	if wait := throttle.retryAfter(now, keys); wait != loginFailureWindow {
		t.Fatalf("retryAfter=%s, want %s", wait, loginFailureWindow)
	}
	if wait := throttle.retryAfter(now.Add(loginFailureWindow), keys); wait != 0 {
		t.Fatalf("still locked out %s after the last failure", loginFailureWindow)
	}
}

func TestLoginThrottleIsCaseInsensitiveAndClearedOnSuccess(t *testing.T) {
	var throttle loginThrottle
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < loginAccountFailures; i++ {
		throttle.recordFailure(now, loginThrottleKeys("10.0.0.9", " operator "))
	}
	if wait := throttle.retryAfter(now, loginThrottleKeys("10.0.0.9", "OPERATOR")); wait == 0 {
		t.Fatal("username casing and padding bypassed the lockout")
	}
	if wait := throttle.retryAfter(now, loginThrottleKeys("10.0.0.10", "OPERATOR")); wait != 0 {
		t.Fatal("one address locked out an unrelated address")
	}
	throttle.recordSuccess(loginThrottleKeys("10.0.0.9", "operator"))
	if wait := throttle.retryAfter(now, loginThrottleKeys("10.0.0.9", "operator")); wait != 0 {
		t.Fatal("a successful sign-in did not clear the failure counters")
	}
}

func TestLoginThrottleCountsAddressAcrossAccounts(t *testing.T) {
	var throttle loginThrottle
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < loginAddressFailures; i++ {
		throttle.recordFailure(now, loginThrottleKeys("10.0.0.9", "user"+string(rune('a'+i%26))+string(rune('a'+i/26))))
	}
	if wait := throttle.retryAfter(now, loginThrottleKeys("10.0.0.9", "someone-new")); wait == 0 {
		t.Fatal("spraying distinct usernames from one address was not throttled")
	}
}

func TestLoginThrottleStaysBounded(t *testing.T) {
	var throttle loginThrottle
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < loginThrottleMaxKeys+100; i++ {
		throttle.recordFailure(now, map[string]int{"account\x00" + string(rune(i)) + "\x00addr": loginAccountFailures})
	}
	if len(throttle.failures) > loginThrottleMaxKeys {
		t.Fatalf("throttle holds %d keys, want at most %d", len(throttle.failures), loginThrottleMaxKeys)
	}
}
