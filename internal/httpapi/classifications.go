package httpapi

import (
	"net/http"

	"ticketmaster/internal/models"
)

// --- discovery: classifications ---

func (s *Server) searchClassifications(w http.ResponseWriter, r *http.Request) {
	classes, err := s.store.Classifications()
	if err != nil {
		serverError(w, err)
		return
	}
	paginate(w, "classifications", classes, r)
}

func (s *Server) getClassification(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.Classification(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "classification not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) createClassification(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	var c models.Classification
	if readJSON(w, r, &c) != nil || c.Segment == "" {
		fail(w, http.StatusBadRequest, "segment required")
		return
	}
	if err := s.store.CreateClassification(&c); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) updateClassification(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	c, err := s.store.Classification(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "classification not found")
		return
	}
	id := c.ID
	if readJSON(w, r, c) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	c.ID = id
	if err := s.store.UpdateClassification(c); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteClassification(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	s.deleted(w, s.store.DeleteClassification(r.PathValue("id")), "classification",
		"classification is still used by events or attractions: reassign those first")
}
