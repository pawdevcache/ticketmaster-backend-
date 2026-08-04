package user

import (
	"time"

	"ticketmaster/internal/store/core"
)

// defaultHold is how long an unpaid booking keeps its seats. Long enough to
// finish a card form without hurrying, short enough that an abandoned checkout
// does not keep a seat off sale for the evening.
const defaultHold = 15 * time.Minute

// Store is the customer-facing half of the data layer. It embeds the shared
// core, so a method here reaches collections and query helpers through its own
// receiver.
type Store struct {
	*core.Store
	// hold is the deadline placed on an unpaid booking. Configurable because
	// the right value depends on the checkout: a card form wants minutes, a
	// test wants seconds.
	hold time.Duration
}

// New wraps a core store for use by the user-facing handlers. A zero or
// negative hold falls back to defaultHold.
func New(c *core.Store, hold time.Duration) *Store {
	if hold <= 0 {
		hold = defaultHold
	}
	return &Store{Store: c, hold: hold}
}
