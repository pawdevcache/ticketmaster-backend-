package httpapi

import (
	"net/http"

	"ticketmaster/internal/store"
)

// --- admin: dashboard analytics ---

// adminAnalytics returns both dashboard series in one response, so the
// Overview tab renders from a single request rather than one per chart.
//
// The top-events cut is a query parameter because a chart shows a handful of
// bars; sending every event and letting the client slice would be the same
// unbounded read that paging exists to avoid.
func (s *Server) adminAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	top := atoiDefault(r.URL.Query().Get("topEvents"), store.DefaultTopEvents)
	if top > store.MaxTopEvents {
		top = store.MaxTopEvents
	}
	a, err := s.store.Analytics(top)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}
