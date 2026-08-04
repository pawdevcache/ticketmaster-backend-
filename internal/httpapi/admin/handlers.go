// Package admin holds the endpoints that require an administrator: catalogue
// writes, account management, every user's bookings, and dashboard analytics.
//
// Each handler begins by calling AdminAuth, which rejects a missing token with
// 401 and a non-admin one with 403. Keeping these routes in their own package
// makes that requirement checkable by reading one file rather than auditing a
// mixed pile of handlers.
package admin

import (
	"ticketmaster/internal/httpapi/web"
	adminstore "ticketmaster/internal/store/admin"
)

// Handlers serves the administrative routes.
//
// Store (embedded from Deps) is the shared core, used to load a record before
// updating it; AdminStore holds the writes and the administrative queries.
type Handlers struct {
	*web.Deps
	AdminStore *adminstore.Store
}

// New builds the handler set around shared dependencies and the admin store.
func New(d *web.Deps, s *adminstore.Store) *Handlers { return &Handlers{d, s} }
