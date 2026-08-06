package user

import (
	"net/http"

	"ticketmaster/internal/httpapi/web"
)

// --- search history ---
//
// History is recorded explicitly rather than as a side effect of searching.
// The discovery endpoints are public, unauthenticated and the busiest reads in
// the service; writing a row on each one would put a database write on the hot
// path and record every keystroke of a type-ahead. The client calls this when
// a search is actually submitted, which is also the only moment it knows the
// term was deliberate.

// RecordSearch remembers a term the caller searched for.
func (h *Handlers) RecordSearch(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	var req struct {
		Term string `json:"term"`
	}
	if web.ReadJSON(w, r, &req) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.UserStore.RecordSearch(u.ID, req.Term); err != nil {
		web.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Searches lists the caller's recent search terms, most recent first.
func (h *Handlers) Searches(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	list, err := h.UserStore.Searches(u.ID)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, map[string]any{"searches": list})
}

// ClearSearches empties the caller's history, or removes one term when an id
// is given.
func (h *Handlers) ClearSearches(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	var err error
	if id := r.PathValue("id"); id != "" {
		err = h.UserStore.ForgetSearch(u.ID, id)
	} else {
		err = h.UserStore.ClearSearches(u.ID)
	}
	if err != nil {
		web.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
