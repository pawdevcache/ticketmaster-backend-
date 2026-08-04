package admin

import (
	"net/http"

	"ticketmaster/internal/httpapi/web"
	adminstore "ticketmaster/internal/store/admin"
)

// adminAnalytics returns both dashboard series in one response, so the
// Overview tab renders from a single request rather than one per chart.
//
// The top-events cut is a query parameter because a chart shows a handful of
// bars; sending every event and letting the client slice would be the same
// unbounded read that paging exists to avoid.
func (h *Handlers) Analytics(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	top := web.AtoiDefault(r.URL.Query().Get("topEvents"), adminstore.DefaultTopEvents)
	if top > adminstore.MaxTopEvents {
		top = adminstore.MaxTopEvents
	}
	a, err := h.AdminStore.Analytics(top)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, a)
}
