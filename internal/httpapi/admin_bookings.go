package httpapi

import (
	"errors"
	"net/http"

	"ticketmaster/internal/store"
)

// --- admin: bookings across all users ---

func (s *Server) adminListBookings(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	q := r.URL.Query()
	p := pageParams(r)
	bookings, total, err := s.store.AllBookings(store.BookingFilter{
		UserID: q.Get("userId"), EventID: q.Get("eventId"), Status: q.Get("status"),
	}, p)
	if err != nil {
		serverError(w, err)
		return
	}
	writePage(w, "bookings", bookings, total, p)
}

func (s *Server) adminGetBooking(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	b, err := s.store.BookingByID(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "booking not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// adminCancelBooking cancels anyone's booking and returns the tickets to the
// event. Deleting instead would lose the audit trail, so cancel is a separate
// action from DELETE.
func (s *Server) adminCancelBooking(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	b, err := s.store.AdminCancelBooking(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, http.StatusNotFound, "booking not found")
			return
		}
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) adminDeleteBooking(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	s.deleted(w, s.store.DeleteBooking(r.PathValue("id")), "booking", "")
}

// adminMe returns the signed-in admin, letting a client verify a stored token.
func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	u := s.adminAuth(w, r)
	if u == nil {
		return
	}
	u.Password = ""
	writeJSON(w, http.StatusOK, u)
}
