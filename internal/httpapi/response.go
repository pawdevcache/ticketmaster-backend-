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
type Server struct {
	store   *store.Store
	limiter *limiter
}

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

// pageParams reads the page and size query parameters. Size is capped here as
// well as in the store: this is where an over-large request is a client error
// worth clamping quietly, rather than a last line of defence.
func pageParams(r *http.Request) store.Page {
	size := atoiDefault(r.URL.Query().Get("size"), store.DefaultPageSize)
	if size > store.MaxPageSize {
		size = store.MaxPageSize
	}
	return store.Page{Number: atoiDefault(r.URL.Query().Get("page"), 0), Size: size}
}

// writePage wraps one page of items in the Discovery-style envelope. The page
// has already been sliced by the database, so total comes from a separate
// count rather than from len(items).
func writePage[T any](w http.ResponseWriter, key string, items []T, total int64, p store.Page) {
	pages := int64(0)
	if p.Size > 0 {
		pages = (total + int64(p.Size) - 1) / int64(p.Size)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"_embedded": map[string]any{key: items},
		"page": map[string]int64{
			"size": int64(p.Size), "totalElements": total,
			"totalPages": pages, "number": int64(p.Number),
		},
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
