package httpapi

import (
	"errors"
	"net/http"

	"ticketmaster/internal/store"
)

// --- bookings ---

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request) {
	u := s.auth(w, r)
	if u == nil {
		return
	}
	var req struct {
		EventID  string `json:"eventId"`
		Quantity int    `json:"quantity"`
	}
	if readJSON(w, r, &req) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	b, err := s.store.Book(u.ID, req.EventID, req.Quantity)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, b)
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "event not found")
	case errors.Is(err, store.ErrSoldOut):
		fail(w, http.StatusConflict, store.ErrSoldOut.Error())
	default:
		serverError(w, err) // unexpected DB error — don't leak internals
	}
}

func (s *Server) listBookings(w http.ResponseWriter, r *http.Request) {
	u := s.auth(w, r)
	if u == nil {
		return
	}
	bookings, err := s.store.UserBookings(u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookings)
}

func (s *Server) getBooking(w http.ResponseWriter, r *http.Request) {
	u := s.auth(w, r)
	if u == nil {
		return
	}
	b, err := s.store.Booking(r.PathValue("id"), u.ID)
	if err != nil {
		fail(w, http.StatusNotFound, "booking not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) cancelBooking(w http.ResponseWriter, r *http.Request) {
	u := s.auth(w, r)
	if u == nil {
		return
	}
	b, err := s.store.CancelBooking(r.PathValue("id"), u.ID)
	if err != nil {
		fail(w, http.StatusNotFound, "booking not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
}
