package admin

import "ticketmaster/internal/store/core"

// Store is the administrative half of the data layer, embedding the shared
// core the same way the user half does.
type Store struct{ *core.Store }

// New wraps a core store for use by the admin handlers.
func New(c *core.Store) *Store { return &Store{c} }
