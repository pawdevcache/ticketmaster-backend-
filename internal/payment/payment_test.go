package payment

import (
	"errors"
	"strings"
	"testing"
)

func TestNewSelectsProvider(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want string // "" means nil provider
	}{
		{"", ""}, {"off", ""}, {"disabled", ""},
		{"test", "test"},
	} {
		p, err := New(Config{Mode: tc.mode})
		if err != nil {
			t.Fatalf("New(%q) errored: %v", tc.mode, err)
		}
		if tc.want == "" {
			if p != nil {
				t.Errorf("New(%q) returned a provider, want nil so bookings confirm immediately", tc.mode)
			}
			continue
		}
		if p == nil || p.Name() != tc.want {
			t.Errorf("New(%q) = %v, want a %s provider", tc.mode, p, tc.want)
		}
	}
}

// Stripe without a key must fail loudly at startup rather than silently
// downgrade to no payments — a service that quietly stops charging is worse
// than one that refuses to boot.
func TestNewRejectsStripeWithoutAKey(t *testing.T) {
	if _, err := New(Config{Mode: "stripe"}); err == nil {
		t.Error("New(stripe) with no secret key succeeded, want an error")
	}
	if _, err := New(Config{Mode: "stripe", SecretKey: "sk_test_x"}); err != nil {
		t.Errorf("New(stripe) with a key errored: %v", err)
	}
}

func TestNewRejectsAnUnknownMode(t *testing.T) {
	if _, err := New(Config{Mode: "paypal"}); err == nil {
		t.Error("an unknown PAYMENTS mode was accepted, want an error")
	}
}

// Providers charge in integer minor units. Truncating instead of rounding
// would undercharge by a cent on values that cannot be represented exactly.
func TestMinorUnits(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want int64
	}{
		{0, 0}, {1, 100}, {10.5, 1050}, {110, 11000},
		{0.1 + 0.2, 30}, // 0.30000000000000004
		{19.99, 1999},   // classic float representation case
		{10.005, 1001},  // rounds up rather than dropping the half cent
	} {
		if got := MinorUnits(tc.in); got != tc.want {
			t.Errorf("MinorUnits(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestTestProviderIssuesAndVerifies(t *testing.T) {
	p, _ := New(Config{Mode: "test", Currency: "eur"})
	i, err := p.CreateIntent(1050, "", "booking-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(i.ID, "pi_test_") {
		t.Errorf("intent id %q lacks the test prefix", i.ID)
	}
	if i.ClientSecret == "" {
		t.Error("intent has no client secret, so a browser could not complete it")
	}
	if i.Amount != 1050 || i.Currency != "eur" {
		t.Errorf("intent = %d %s, want 1050 eur", i.Amount, i.Currency)
	}
	if err := p.Verify(i.ID); err != nil {
		t.Errorf("Verify of a freshly issued intent failed: %v", err)
	}
}

// Two intents must never collide: the id is what links a booking to money.
func TestTestProviderIntentsAreUnique(t *testing.T) {
	p, _ := New(Config{Mode: "test"})
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		in, err := p.CreateIntent(100, "usd", "b")
		if err != nil {
			t.Fatal(err)
		}
		if seen[in.ID] {
			t.Fatalf("duplicate intent id %q", in.ID)
		}
		seen[in.ID] = true
	}
}

func TestTestProviderRejectsForeignIntents(t *testing.T) {
	p, _ := New(Config{Mode: "test"})
	if err := p.Verify("pi_live_something"); !errors.Is(err, ErrNotSucceeded) {
		t.Errorf("Verify of a non-test intent = %v, want ErrNotSucceeded", err)
	}
	if err := p.Verify(""); !errors.Is(err, ErrNotSucceeded) {
		t.Errorf("Verify of an empty intent = %v, want ErrNotSucceeded", err)
	}
}
