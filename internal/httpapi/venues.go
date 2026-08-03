package httpapi

import (
	"net/http"

	"ticketmaster/internal/models"
)

// --- discovery: venues ---

func (s *Server) searchVenues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := pageParams(r)
	venues, total, err := s.store.Venues(q.Get("keyword"), q.Get("city"), p)
	if err != nil {
		serverError(w, err)
		return
	}
	writePage(w, "venues", venues, total, p)
}

func (s *Server) getVenue(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Venue(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "venue not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createVenue(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	var v models.Venue
	if readJSON(w, r, &v) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.CreateVenue(&v); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateVenue(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	v, err := s.store.Venue(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "venue not found")
		return
	}
	id := v.ID
	if readJSON(w, r, v) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	v.ID = id
	if err := s.store.UpdateVenue(v); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteVenue(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	s.deleted(w, s.store.DeleteVenue(r.PathValue("id")), "venue",
		"venue still hosts events: move or delete those first")
}
