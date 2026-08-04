package user

import (
	"net/http"
	"time"

	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/store"
)

func (h *Handlers) SearchEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.EventFilter{Keyword: q.Get("keyword"), City: q.Get("city"), ClassificationID: q.Get("classificationId")}
	if t, err := time.Parse(time.RFC3339, q.Get("startDateTime")); err == nil {
		f.StartAfter = t
	}
	p := web.PageParams(r)
	events, total, err := h.Store.Events(f, p)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WritePage(w, "events", events, total, p)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	e, err := h.Store.Event(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "event not found")
		return
	}
	web.WriteJSON(w, http.StatusOK, e)
}

func (h *Handlers) SearchVenues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := web.PageParams(r)
	venues, total, err := h.Store.Venues(q.Get("keyword"), q.Get("city"), p)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WritePage(w, "venues", venues, total, p)
}

func (h *Handlers) GetVenue(w http.ResponseWriter, r *http.Request) {
	v, err := h.Store.Venue(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "venue not found")
		return
	}
	web.WriteJSON(w, http.StatusOK, v)
}

func (h *Handlers) SearchAttractions(w http.ResponseWriter, r *http.Request) {
	p := web.PageParams(r)
	attractions, total, err := h.Store.Attractions(r.URL.Query().Get("keyword"), p)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WritePage(w, "attractions", attractions, total, p)
}

func (h *Handlers) GetAttraction(w http.ResponseWriter, r *http.Request) {
	a, err := h.Store.Attraction(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "attraction not found")
		return
	}
	web.WriteJSON(w, http.StatusOK, a)
}

func (h *Handlers) SearchClassifications(w http.ResponseWriter, r *http.Request) {
	p := web.PageParams(r)
	classes, total, err := h.Store.Classifications(p)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WritePage(w, "classifications", classes, total, p)
}

func (h *Handlers) GetClassification(w http.ResponseWriter, r *http.Request) {
	c, err := h.Store.Classification(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "classification not found")
		return
	}
	web.WriteJSON(w, http.StatusOK, c)
}
