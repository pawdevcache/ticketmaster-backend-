package httpapi

import (
	"net/http"

	"ticketmaster/internal/models"
)

// --- discovery: attractions ---

func (s *Server) searchAttractions(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r)
	attractions, total, err := s.store.Attractions(r.URL.Query().Get("keyword"), p)
	if err != nil {
		serverError(w, err)
		return
	}
	writePage(w, "attractions", attractions, total, p)
}

func (s *Server) getAttraction(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.Attraction(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "attraction not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) createAttraction(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	var a models.Attraction
	if readJSON(w, r, &a) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.CreateAttraction(&a); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) updateAttraction(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	a, err := s.store.Attraction(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "attraction not found")
		return
	}
	id := a.ID
	if readJSON(w, r, a) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	a.ID = id
	if err := s.store.UpdateAttraction(a); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) deleteAttraction(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	s.deleted(w, s.store.DeleteAttraction(r.PathValue("id")), "attraction",
		"attraction is still listed on events: remove it from those first")
}
