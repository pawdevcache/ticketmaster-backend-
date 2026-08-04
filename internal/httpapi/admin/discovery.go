package admin

import (
	"net/http"

	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/models"
)

func (h *Handlers) CreateEvent(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	var e models.Event
	if web.ReadJSON(w, r, &e) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.Store.CreateEvent(&e); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusCreated, e)
}

// updateEvent applies a partial update: the body is decoded over the stored
// event, so omitted fields keep their current values. ticketsSold belongs to
// the booking flow and is never taken from the request.
func (h *Handlers) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	e, err := h.Store.Event(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "event not found")
		return
	}
	id, sold := e.ID, e.TicketsSold
	if web.ReadJSON(w, r, e) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	e.ID, e.TicketsSold = id, sold
	if e.TicketsTotal < sold {
		web.Fail(w, http.StatusBadRequest, "ticketsTotal is below the number already sold")
		return
	}
	if err := h.Store.UpdateEvent(e); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, e)
}

func (h *Handlers) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	h.Deleted(w, h.Store.DeleteEvent(r.PathValue("id")), "event",
		"event has confirmed bookings: cancel those first")
}

func (h *Handlers) CreateVenue(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	var v models.Venue
	if web.ReadJSON(w, r, &v) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.Store.CreateVenue(&v); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusCreated, v)
}

func (h *Handlers) UpdateVenue(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	v, err := h.Store.Venue(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "venue not found")
		return
	}
	id := v.ID
	if web.ReadJSON(w, r, v) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	v.ID = id
	if err := h.Store.UpdateVenue(v); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, v)
}

func (h *Handlers) DeleteVenue(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	h.Deleted(w, h.Store.DeleteVenue(r.PathValue("id")), "venue",
		"venue still hosts events: move or delete those first")
}

func (h *Handlers) CreateAttraction(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	var a models.Attraction
	if web.ReadJSON(w, r, &a) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.Store.CreateAttraction(&a); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusCreated, a)
}

func (h *Handlers) UpdateAttraction(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	a, err := h.Store.Attraction(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "attraction not found")
		return
	}
	id := a.ID
	if web.ReadJSON(w, r, a) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	a.ID = id
	if err := h.Store.UpdateAttraction(a); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, a)
}

func (h *Handlers) DeleteAttraction(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	h.Deleted(w, h.Store.DeleteAttraction(r.PathValue("id")), "attraction",
		"attraction is still listed on events: remove it from those first")
}

func (h *Handlers) CreateClassification(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	var c models.Classification
	if web.ReadJSON(w, r, &c) != nil || c.Segment == "" {
		web.Fail(w, http.StatusBadRequest, "segment required")
		return
	}
	if err := h.Store.CreateClassification(&c); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handlers) UpdateClassification(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	c, err := h.Store.Classification(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "classification not found")
		return
	}
	id := c.ID
	if web.ReadJSON(w, r, c) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	c.ID = id
	if err := h.Store.UpdateClassification(c); err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, c)
}

func (h *Handlers) DeleteClassification(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	h.Deleted(w, h.Store.DeleteClassification(r.PathValue("id")), "classification",
		"classification is still used by events or attractions: reassign those first")
}
