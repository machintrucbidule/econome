package auth

import (
	"sync"
	"time"
)

// LockoutThreshold is the number of consecutive failures tolerated before
// progressive backoff begins (technical/05 §6, functional/01 §1.3).
const LockoutThreshold = 5

// backoffs are the progressive lock durations applied once the threshold is
// exceeded: failure 6 -> 1 s, 7 -> 30 s, 8+ -> 300 s.
var backoffs = []time.Duration{1 * time.Second, 30 * time.Second, 300 * time.Second}

// BackoffFor returns the lock duration to apply for the given consecutive
// failure count (the count after incrementing for the current failure). It is
// zero for the first LockoutThreshold failures.
func BackoffFor(failedCount int) time.Duration {
	if failedCount <= LockoutThreshold {
		return 0
	}
	idx := failedCount - LockoutThreshold - 1
	if idx >= len(backoffs) {
		idx = len(backoffs) - 1
	}
	return backoffs[idx]
}

// RemainingLock returns how long the account stays locked from now, or zero if
// it is not currently locked.
func RemainingLock(lockedUntil *time.Time, now time.Time) time.Duration {
	if lockedUntil == nil {
		return 0
	}
	if d := lockedUntil.Sub(now); d > 0 {
		return d
	}
	return 0
}

// Throttle is a best-effort in-memory per-key (per-IP) sliding-window limiter.
// It complements the per-account lockout and resets on restart (technical/05 §6).
type Throttle struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string][]time.Time
}

// NewThrottle allows at most max events per key within window.
func NewThrottle(maxEvents int, window time.Duration) *Throttle {
	return &Throttle{window: window, max: maxEvents, hits: map[string][]time.Time{}}
}

// Allow records an attempt for key at now and reports whether it is within the
// limit. Entries older than the window are pruned.
func (t *Throttle) Allow(key string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := now.Add(-t.window)
	kept := t.hits[key][:0]
	for _, ts := range t.hits[key] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= t.max {
		t.hits[key] = kept
		return false
	}
	t.hits[key] = append(kept, now)
	return true
}
