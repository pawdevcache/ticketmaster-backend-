package user

import "ticketmaster/internal/store/core"

// Store is the customer-facing half of the data layer. It embeds the shared
// core, so a method here reaches collections and query helpers through its own
// receiver.
type Store struct{ *core.Store }

// New wraps a core store for use by the user-facing handlers.
func New(c *core.Store) *Store { return &Store{c} }
