// Package payment takes money for a booking.
//
// The service never sees a card number. A provider creates an intent, the
// browser completes it directly with the provider using the returned client
// secret, and the API then asks the provider whether that intent actually
// succeeded. Trusting the browser's word instead would let anyone confirm a
// booking by calling the pay endpoint.
//
// Three modes, selected by PAYMENTS:
//
//	off    — no payment step; a booking confirms immediately (the default,
//	         and the behaviour the service had before payments existed)
//	test   — holds and a payment step, with intents that always succeed;
//	         for local work and the end-to-end suite
//	stripe — real Stripe PaymentIntents
package payment

import (
	"errors"
	"fmt"
	"math"
)

// ErrNotSucceeded means the provider says this intent has not been paid.
var ErrNotSucceeded = errors.New("payment has not succeeded")

// Intent is a pending charge. ClientSecret is what the browser needs to
// complete it; it is deliberately the only secret shared outward, and it is
// scoped to this one charge.
type Intent struct {
	ID           string  `json:"id"`
	ClientSecret string  `json:"clientSecret,omitempty"`
	Amount       int64   `json:"amount"` // minor units, e.g. cents
	Currency     string  `json:"currency"`
	Provider     string  `json:"provider"`
	Total        float64 `json:"-"`
}

// Provider is a payment backend.
type Provider interface {
	// Name identifies the backend in responses and logs.
	Name() string
	// CreateIntent registers a charge to be completed by the buyer.
	CreateIntent(amountMinor int64, currency, reference string) (*Intent, error)
	// Verify asks the provider whether the intent has actually been paid. It
	// is the authority; the client's claim is not.
	Verify(intentID string) error
}

// Config selects and configures the backend.
type Config struct {
	Mode      string // off, test, stripe
	SecretKey string
	Currency  string
}

// New returns the configured provider, or nil when payments are off. A nil
// provider means callers keep the original behaviour: bookings confirm on
// creation with no payment step.
func New(c Config) (Provider, error) {
	if c.Currency == "" {
		c.Currency = "usd"
	}
	switch c.Mode {
	case "", "off", "disabled":
		return nil, nil
	case "test":
		return &testProvider{currency: c.Currency}, nil
	case "stripe":
		if c.SecretKey == "" {
			return nil, errors.New("PAYMENTS=stripe needs STRIPE_SECRET_KEY")
		}
		return &stripeProvider{key: c.SecretKey, currency: c.Currency}, nil
	default:
		return nil, fmt.Errorf("unknown PAYMENTS mode %q (want off, test or stripe)", c.Mode)
	}
}

// MinorUnits converts a decimal total to the integer minor units providers
// expect. Rounding rather than truncating, so 10.005 does not become 10.00 and
// quietly undercharge.
func MinorUnits(total float64) int64 {
	return int64(math.Round(total * 100))
}
