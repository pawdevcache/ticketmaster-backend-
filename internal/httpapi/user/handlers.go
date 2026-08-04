// Package user holds the endpoints any visitor or signed-in customer can
// reach: the public catalogue reads, registration and sign-in, password
// recovery, and a customer's own bookings.
//
// Nothing here grants access to another person's records. Handlers that touch
// bookings scope every query by the caller's own id, so the separation from
// the admin package is a boundary the compiler helps keep, not just a naming
// convention.
package user

import (
	"ticketmaster/internal/httpapi/web"
	userstore "ticketmaster/internal/store/user"
)

// Handlers serves the public and customer-facing routes.
//
// Two stores, because the tiers share a catalogue: Store (embedded from Deps)
// is the shared core used for reads, and UserStore holds the operations only a
// customer performs on their own records.
type Handlers struct {
	*web.Deps
	UserStore *userstore.Store
}

// New builds the handler set around shared dependencies and the user store.
func New(d *web.Deps, s *userstore.Store) *Handlers { return &Handlers{d, s} }
