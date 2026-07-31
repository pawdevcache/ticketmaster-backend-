package httpapi

import (
	"net/http"
	"time"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store"
)

// --- discovery: events ---

func (s *Server) searchEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.EventFilter{Keyword: q.Get("keyword"), City: q.Get("city"), ClassificationID: q.Get("classificationId")}
	if t, err := time.Parse(time.RFC3339, q.Get("startDateTime")); err == nil {
		f.StartAfter = t
	}
	events, err := s.store.Events(f)
	if err != nil {
		serverError(w, err)
		return
	}
	paginate(w, "events", events, r)
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	e, err := s.store.Event(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "event not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	var e models.Event
	if readJSON(w, r, &e) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.CreateEvent(&e); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// updateEvent applies a partial update: the body is decoded over the stored
// event, so omitted fields keep their current values. ticketsSold belongs to
// the booking flow and is never taken from the request.
func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	e, err := s.store.Event(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "event not found")
		return
	}
	id, sold := e.ID, e.TicketsSold
	if readJSON(w, r, e) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	e.ID, e.TicketsSold = id, sold
	if e.TicketsTotal < sold {
		fail(w, http.StatusBadRequest, "ticketsTotal is below the number already sold")
		return
	}
	if err := s.store.UpdateEvent(e); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	s.deleted(w, s.store.DeleteEvent(r.PathValue("id")), "event",
		"event has confirmed bookings: cancel those first")
}
