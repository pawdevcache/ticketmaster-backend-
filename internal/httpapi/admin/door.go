package admin

import (
	"net/http"

	"ticketmaster/internal/httpapi/web"
)

// Door serves the gate dashboard: sold versus admitted, per event, computed
// fresh on every request so it can be polled during a performance.
func (h *Handlers) Door(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	rows, err := h.AdminStore.Door(r.URL.Query().Get("eventId"))
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]any{"events": rows})
}
