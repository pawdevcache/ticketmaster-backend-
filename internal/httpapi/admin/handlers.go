// Package admin holds the endpoints that require an administrator: catalogue
// writes, account management, every user's bookings, and dashboard analytics.
//
// Each handler begins by calling AdminAuth, which rejects a missing token with
// 401 and a non-admin one with 403. Keeping these routes in their own package
// makes that requirement checkable by reading one file rather than auditing a
// mixed pile of handlers.
package admin

import "ticketmaster/internal/httpapi/web"

// Handlers serves the administrative routes.
type Handlers struct{ *web.Deps }

// New builds the handler set around shared dependencies.
func New(d *web.Deps) *Handlers { return &Handlers{d} }
