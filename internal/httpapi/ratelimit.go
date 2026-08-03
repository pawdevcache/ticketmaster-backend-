package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ticketmaster/internal/config"
)

// Rate-limit defaults, overridable with RATE_LIMIT_ATTEMPTS and
// RATE_LIMIT_WINDOW. Ten attempts per quarter hour leaves a forgetful human
// plenty of room while making password guessing useless.
const (
	defaultRateLimit  = 10
	defaultRateWindow = 15 * time.Minute
)

// limiter is a sliding-window counter keyed by caller.
//
// State lives in this process only. Behind a single server that is exactly
// right; across several instances — Vercel spins one up per concurrent
// request — each keeps its own counter, so the effective limit multiplies by
// the instance count. It still bounds an attacker, but a deployment that needs
// a hard global limit wants the counters in MongoDB instead.
type limiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
	swept  time.Time
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}

// allow records an attempt and reports whether it is permitted. When it is
// not, it also returns how long the caller should wait — the time until the
// oldest attempt in the window ages out.
func (l *limiter) allow(key string, now time.Time) (bool, time.Duration) {
	if l.limit <= 0 {
		return true, 0 // disabled
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)

	cutoff := now.Add(-l.window)
	recent := l.hits[key][:0] // reuse the backing array
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.limit {
		l.hits[key] = recent
		// A rejected attempt is not recorded, so a blocked caller cannot push
		// their own unlock time further away by hammering the endpoint.
		return false, recent[0].Sub(cutoff)
	}
	l.hits[key] = append(recent, now)
	return true, 0
}

// sweep drops keys whose attempts have all aged out. Without it the map grows
// with every distinct address seen, which would turn the defence into a slow
// memory leak. Called under the mutex, at most once per window.
func (l *limiter) sweep(now time.Time) {
	if now.Sub(l.swept) < l.window {
		return
	}
	l.swept = now
	cutoff := now.Add(-l.window)
	for key, times := range l.hits {
		if len(times) == 0 || !times[len(times)-1].After(cutoff) {
			delete(l.hits, key)
		}
	}
}

// rateLimited wraps a handler so repeated calls from one caller are rejected
// with 429. The name separates buckets, so exhausting login attempts does not
// also block registration.
func (s *Server) rateLimited(name string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := s.limiter.allow(name+"|"+clientIP(r), time.Now())
		if !ok {
			secs := int(retryAfter.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			fail(w, http.StatusTooManyRequests,
				"too many attempts, try again in "+strconv.Itoa(secs)+" seconds")
			return
		}
		h(w, r)
	}
}

// clientIP identifies the caller.
//
// Render and Vercel both terminate TLS in front of the application, so
// RemoteAddr is the proxy and every user would share one bucket. The
// X-Forwarded-For header carries the real address and is preferred.
//
// That header is caller-supplied, so an attacker who can reach the service
// directly — not through the platform's proxy — can rotate it and evade the
// limit. Exposing this service without a proxy in front weakens the defence to
// that extent.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if ip := strings.TrimSpace(first); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rateLimitSettings reads the configured limit and window, falling back to the
// defaults. A limit of 0 disables rate limiting entirely.
func rateLimitSettings() (int, time.Duration) {
	limit := defaultRateLimit
	if n, err := strconv.Atoi(config.Get("RATE_LIMIT_ATTEMPTS", "")); err == nil && n >= 0 {
		limit = n
	}
	window := defaultRateWindow
	if d, err := time.ParseDuration(config.Get("RATE_LIMIT_WINDOW", "")); err == nil && d > 0 {
		window = d
	}
	return limit, window
}
