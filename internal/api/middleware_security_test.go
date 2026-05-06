package api

import (
	"testing"
	"time"
)

// Quick sanity for the bucket math: a fresh visitor gets `burst` calls
// for free, then is shut out until the drip refills. We use tiny
// numbers + a sleep instead of mocking time because the math is
// straightforward and a real time.Now() round-trip is plenty fast.
func TestRateLimiter_BurstThenDeny(t *testing.T) {
	rl := newRateLimiter(1, 3, time.Minute) // 1 token/sec, burst 3
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("burst call %d should be allowed", i+1)
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Fatalf("4th call should be denied (bucket empty)")
	}
	// Different IP gets its own fresh bucket.
	if !rl.Allow("5.6.7.8") {
		t.Fatalf("new IP should start with full bucket")
	}
	// After ~1.1s we should have 1 token back.
	time.Sleep(1100 * time.Millisecond)
	if !rl.Allow("1.2.3.4") {
		t.Fatalf("after refill window, call should be allowed")
	}
}
