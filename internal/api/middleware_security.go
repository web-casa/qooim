package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/web-casa/qooim/internal/auth"
)

// requireAdmin gates SK admin routes on the "admin" role code. Anyone
// without it gets a SK-shape 403 instead of a 200; that matches what
// the SK frontend expects from a permission denial (`code === 403`).
//
// The check is intentionally coarse — role-based, not authority-based —
// because SK's seed model and the umi-access plugin both pivot on the
// admin role. Per-authority gating is a follow-up.
func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := auth.FromContext(c)
		if !ok {
			skErr(c, http.StatusUnauthorized, "not authenticated")
			return
		}
		for _, r := range p.Roles {
			if r == "admin" {
				c.Next()
				return
			}
		}
		skErr(c, http.StatusForbidden, "permission denied")
	}
}

// ------ public-route rate limiter ----------------------------------------

// rateLimiter is a tiny per-IP token bucket. We don't reach for a
// dependency because the only knob that matters for the bug class
// "credential stuffing / answer spam" is "burst N then drip at R/sec",
// which time.Tick + a map gives us in 30 lines.
//
// The implementation is intentionally simple:
//
//   - one map[ip] → state, guarded by a single mutex
//   - idle entries reaped after `idle` to keep memory bounded
//   - token math is lazy (recomputed on each request, no goroutine)
//
// For Qoo.IM-scale traffic this is plenty; behind a CDN/proxy the
// caller IP comes from the X-Forwarded-For chain via gin's ClientIP.
type rateState struct {
	tokens float64
	last   time.Time
}

type RateLimiter = rateLimiter

type rateLimiter struct {
	mu    sync.Mutex
	state map[string]*rateState
	rate  float64       // tokens added per second
	burst float64       // bucket size
	idle  time.Duration // reap after no request for this long
}

func newRateLimiter(rate, burst float64, idle time.Duration) *rateLimiter {
	return &rateLimiter{state: map[string]*rateState{}, rate: rate, burst: burst, idle: idle}
}

// NewRateLimiter is the exported counterpart used by sub-packages
// (e.g. internal/api/console) that want to share the same in-memory
// token bucket rather than maintain their own.
func NewRateLimiter(rate, burst float64, idle time.Duration) *RateLimiter {
	return newRateLimiter(rate, burst, idle)
}

func (rl *rateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	st, ok := rl.state[ip]
	if !ok {
		// New visitor starts with a full bucket so single-shot calls
		// always pass; only the abusers that drain it fast get 429s.
		st = &rateState{tokens: rl.burst, last: now}
		rl.state[ip] = st
	}
	// Refill.
	elapsed := now.Sub(st.last).Seconds()
	st.tokens = clampFloat(st.tokens+elapsed*rl.rate, 0, rl.burst)
	st.last = now
	if st.tokens < 1 {
		return false
	}
	st.tokens--
	// Opportunistic GC.
	if len(rl.state) > 1024 {
		for k, v := range rl.state {
			if now.Sub(v.last) > rl.idle {
				delete(rl.state, k)
			}
		}
	}
	return true
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// rateLimit returns a gin middleware that consults the supplied
// rateLimiter, keyed by the request's ClientIP. Over-budget callers
// get a SK-shape 429 with a Retry-After header.
func rateLimit(rl *rateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.Allow(c.ClientIP()) {
			c.Header("Retry-After", strconv.Itoa(1))
			skErr(c, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		c.Next()
	}
}
