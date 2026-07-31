package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store"
)

// Server carries the dependencies shared by every handler. Handlers hang off
// this type rather than reading package-level state, so tests can build one
// against a different store.
type Server struct{ store *store.Store }

const maxBodyBytes = 1 << 20 // 1 MiB cap on request bodies

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError logs the real cause but returns a generic message, so internal
// database details are never exposed to clients.
func serverError(w http.ResponseWriter, err error) {
	log.Println("server error:", err)
	fail(w, http.StatusInternalServerError, "internal server error")
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

// paginate slices items and wraps them in a Discovery-style envelope.
func paginate[T any](w http.ResponseWriter, key string, items []T, r *http.Request) {
	size := atoiDefault(r.URL.Query().Get("size"), 20)
	if size > 100 {
		size = 100 // cap page size to avoid abusive large responses
	}
	page := atoiDefault(r.URL.Query().Get("page"), 0)
	total := len(items)
	start := page * size
	end := start + size
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pages := (total + size - 1) / size
	writeJSON(w, http.StatusOK, map[string]any{
		"_embedded": map[string]any{key: items[start:end]},
		"page":      map[string]int{"size": size, "totalElements": total, "totalPages": pages, "number": page},
	})
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return d
}

// auth resolves the bearer token to a user, or writes 401 and returns nil.
func (s *Server) auth(w http.ResponseWriter, r *http.Request) *models.User {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	u, err := s.store.UserByToken(tok)
	if err != nil {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return nil
	}
	return u
}

// adminAuth resolves the bearer token and requires an admin account, or writes
// 401/403 and returns nil.
func (s *Server) adminAuth(w http.ResponseWriter, r *http.Request) *models.User {
	u := s.auth(w, r)
	if u == nil {
		return nil
	}
	if !u.IsAdmin() {
		fail(w, http.StatusForbidden, "admin access required")
		return nil
	}
	return u
}

// deleted writes the standard response for a delete: 204 on success, or the
// mapped error. inUse is the message for a record other records still point at.
func (s *Server) deleted(w http.ResponseWriter, err error, what, inUse string) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, what+" not found")
	case errors.Is(err, store.ErrInUse):
		fail(w, http.StatusConflict, inUse)
	default:
		serverError(w, err)
	}
}
