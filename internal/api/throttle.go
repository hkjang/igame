package api

import (
	"strings"
	"sync"
	"time"
)

// Local password login is the only credential an attacker can guess online, so
// it is throttled per account and per source address. State is in memory: an
// air-gapped deployment runs a single instance, and losing counters on restart
// is preferable to a write amplification on every failed login.
const (
	loginFailureWindow   = 15 * time.Minute
	loginAccountFailures = 10
	loginAddressFailures = 50
	loginThrottleMaxKeys = 10000
)

type failureRecord struct {
	count   int
	expires time.Time
}

type loginThrottle struct {
	mu       sync.Mutex
	failures map[string]failureRecord
}

// retryAfter reports how long the caller must wait before another attempt is
// accepted, or zero when the attempt may proceed.
func (t *loginThrottle) retryAfter(now time.Time, keys map[string]int) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	var wait time.Duration
	for key, limit := range keys {
		record, ok := t.failures[key]
		if !ok || !now.Before(record.expires) {
			continue
		}
		if record.count >= limit && record.expires.Sub(now) > wait {
			wait = record.expires.Sub(now)
		}
	}
	return wait
}

func (t *loginThrottle) recordFailure(now time.Time, keys map[string]int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failures == nil {
		t.failures = map[string]failureRecord{}
	}
	if len(t.failures) >= loginThrottleMaxKeys {
		t.evictExpiredLocked(now)
		if len(t.failures) >= loginThrottleMaxKeys {
			// A flood of distinct keys must not grow without bound; dropping the
			// table costs accuracy, never correctness.
			clear(t.failures)
		}
	}
	for key := range keys {
		record := t.failures[key]
		if !now.Before(record.expires) {
			record = failureRecord{}
		}
		record.count++
		record.expires = now.Add(loginFailureWindow)
		t.failures[key] = record
	}
}

func (t *loginThrottle) recordSuccess(keys map[string]int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key := range keys {
		delete(t.failures, key)
	}
}

func (t *loginThrottle) evictExpiredLocked(now time.Time) {
	for key, record := range t.failures {
		if !now.Before(record.expires) {
			delete(t.failures, key)
		}
	}
}

func loginThrottleKeys(remoteAddr, username string) map[string]int {
	return map[string]int{
		"account\x00" + strings.ToLower(strings.TrimSpace(username)) + "\x00" + remoteAddr: loginAccountFailures,
		"address\x00" + remoteAddr: loginAddressFailures,
	}
}
