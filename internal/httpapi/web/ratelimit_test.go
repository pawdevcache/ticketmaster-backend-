package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	now := time.Unix(1_700_000_000, 0)
	for i := 1; i <= 3; i++ {
		if ok, _ := l.Allow("k", now); !ok {
			t.Fatalf("attempt %d was blocked, want allowed", i)
		}
	}
	ok, retry := l.Allow("k", now)
	if ok {
		t.Fatal("attempt 4 was allowed, want blocked")
	}
	if retry <= 0 || retry > time.Minute {
		t.Errorf("retryAfter = %v, want between 0 and the window", retry)
	}
}

// A blocked caller must not be able to push their own unlock time further away
// by continuing to hammer the endpoint.
func TestLimiterRejectedAttemptsDoNotExtendTheWindow(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	start := time.Unix(1_700_000_000, 0)
	l.Allow("k", start)
	l.Allow("k", start)

	_, first := l.Allow("k", start.Add(10*time.Second))
	_, second := l.Allow("k", start.Add(20*time.Second))
	if second >= first {
		t.Errorf("retryAfter grew from %v to %v while being hammered", first, second)
	}
}

func TestLimiterWindowExpires(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	start := time.Unix(1_700_000_000, 0)
	l.Allow("k", start)
	l.Allow("k", start)
	if ok, _ := l.Allow("k", start.Add(time.Second)); ok {
		t.Fatal("blocked attempt was allowed inside the window")
	}
	if ok, _ := l.Allow("k", start.Add(61*time.Second)); !ok {
		t.Error("attempt after the window was blocked, want allowed")
	}
}

// Buckets are per route and per caller: exhausting one must not affect another.
func TestLimiterKeysAreIndependent(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	now := time.Unix(1_700_000_000, 0)
	l.Allow("login|1.1.1.1", now)
	if ok, _ := l.Allow("login|1.1.1.1", now); ok {
		t.Error("same key was allowed twice with limit 1")
	}
	if ok, _ := l.Allow("login|2.2.2.2", now); !ok {
		t.Error("a different caller was blocked")
	}
	if ok, _ := l.Allow("register|1.1.1.1", now); !ok {
		t.Error("a different route was blocked for the same caller")
	}
}

func TestLimiterDisabledWhenLimitIsZero(t *testing.T) {
	l := NewLimiter(0, time.Minute)
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("k", now); !ok {
			t.Fatalf("attempt %d blocked while rate limiting is disabled", i)
		}
	}
}

// Without pruning, the map grows with every distinct address ever seen — the
// defence would become a slow memory leak.
func TestLimiterSweepsExpiredKeys(t *testing.T) {
	l := NewLimiter(5, time.Minute)
	start := time.Unix(1_700_000_000, 0)
	for _, k := range []string{"a", "b", "c"} {
		l.Allow(k, start)
	}
	if len(l.hits) != 3 {
		t.Fatalf("map holds %d keys, want 3", len(l.hits))
	}
	// One new attempt a full window later should clear the stale keys.
	l.Allow("d", start.Add(2*time.Minute))
	if len(l.hits) != 1 {
		t.Errorf("map holds %d keys after sweep, want 1", len(l.hits))
	}
}

func TestClientIP(t *testing.T) {
	for _, tc := range []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{"no proxy header", "", "203.0.113.9:54321", "203.0.113.9"},
		{"single forwarded", "198.51.100.7", "10.0.0.1:443", "198.51.100.7"},
		{"proxy chain takes the client", "198.51.100.7, 10.0.0.1", "10.0.0.1:443", "198.51.100.7"},
		{"chain with spacing", "  198.51.100.7 , 10.0.0.1 ", "10.0.0.1:443", "198.51.100.7"},
		{"blank header falls back", "   ", "203.0.113.9:1234", "203.0.113.9"},
		{"remote addr without a port", "", "203.0.113.9", "203.0.113.9"},
		{"ipv6 remote addr", "", "[2001:db8::1]:8080", "2001:db8::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/login", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := ClientIP(r); got != tc.want {
				t.Errorf("ClientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRateLimitedHandlerReturns429WithRetryAfter(t *testing.T) {
	s := &Deps{Limiter: NewLimiter(1, time.Minute)}
	calls := 0
	h := s.RateLimited("login", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})

	req := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/login", nil)
		r.RemoteAddr = "203.0.113.9:1111"
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	if rec := req(); rec.Code != http.StatusOK {
		t.Fatalf("first call = %d, want 200", rec.Code)
	}
	rec := req()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second call = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 response is missing a Retry-After header")
	}
	if calls != 1 {
		t.Errorf("wrapped handler ran %d times, want 1 — a blocked request must not reach it", calls)
	}
}

func TestRateLimitSettingsDefaults(t *testing.T) {
	t.Setenv("RATE_LIMIT_ATTEMPTS", "")
	t.Setenv("RATE_LIMIT_WINDOW", "")
	if limit, window := RateLimitSettings(); limit != defaultRateLimit || window != defaultRateWindow {
		t.Errorf("defaults = %d/%v, want %d/%v", limit, window, defaultRateLimit, defaultRateWindow)
	}

	t.Setenv("RATE_LIMIT_ATTEMPTS", "3")
	t.Setenv("RATE_LIMIT_WINDOW", "30s")
	if limit, window := RateLimitSettings(); limit != 3 || window != 30*time.Second {
		t.Errorf("overrides = %d/%v, want 3/30s", limit, window)
	}

	t.Setenv("RATE_LIMIT_ATTEMPTS", "nonsense")
	t.Setenv("RATE_LIMIT_WINDOW", "nonsense")
	if limit, window := RateLimitSettings(); limit != defaultRateLimit || window != defaultRateWindow {
		t.Errorf("unparseable values = %d/%v, want the defaults", limit, window)
	}

	t.Setenv("RATE_LIMIT_ATTEMPTS", "0")
	if limit, _ := RateLimitSettings(); limit != 0 {
		t.Errorf("limit = %d, want 0 so rate limiting can be turned off", limit)
	}
}
