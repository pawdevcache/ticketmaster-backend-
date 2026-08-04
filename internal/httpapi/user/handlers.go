// Package user holds the endpoints any visitor or signed-in customer can
// reach: the public catalogue reads, registration and sign-in, password
// recovery, and a customer's own bookings.
//
// Nothing here grants access to another person's records. Handlers that touch
// bookings scope every query by the caller's own id, so the separation from
// the admin package is a boundary the compiler helps keep, not just a naming
// convention.
package user

import "ticketmaster/internal/httpapi/web"

// Handlers serves the public and customer-facing routes.
type Handlers struct{ *web.Deps }

// New builds the handler set around shared dependencies.
func New(d *web.Deps) *Handlers { return &Handlers{d} }
