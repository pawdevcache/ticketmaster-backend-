package web

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

// Deps carries the dependencies every handler needs. The user and admin
// packages embed it, so a handler reaches the store and the rate limiter
// through its own receiver rather than through package-level state — which is
// also what lets a test build one against a different store.
type Deps struct {
	Store   *store.Store
	Limiter *Limiter
}

const MaxBodyBytes = 1 << 20 // 1 MiB cap on request bodies

// --- helpers ---

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func Fail(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ServerError logs the real cause but returns a generic message, so internal
// database details are never exposed to clients.
func ServerError(w http.ResponseWriter, err error) {
	log.Println("server error:", err)
	Fail(w, http.StatusInternalServerError, "internal server error")
}

func ReadJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

// PageParams reads the page and size query parameters. Size is capped here as
// well as in the store: this is where an over-large request is a client error
// worth clamping quietly, rather than a last line of defence.
func PageParams(r *http.Request) store.Page {
	size := AtoiDefault(r.URL.Query().Get("size"), store.DefaultPageSize)
	if size > store.MaxPageSize {
		size = store.MaxPageSize
	}
	return store.Page{Number: AtoiDefault(r.URL.Query().Get("page"), 0), Size: size}
}

// WritePage wraps one page of items in the Discovery-style envelope. The page
// has already been sliced by the database, so total comes from a separate
// count rather than from len(items).
func WritePage[T any](w http.ResponseWriter, key string, items []T, total int64, p store.Page) {
	pages := int64(0)
	if p.Size > 0 {
		pages = (total + int64(p.Size) - 1) / int64(p.Size)
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"_embedded": map[string]any{key: items},
		"page": map[string]int64{
			"size": int64(p.Size), "totalElements": total,
			"totalPages": pages, "number": int64(p.Number),
		},
	})
}

func AtoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return d
}

// BearerToken extracts the token from the Authorization header. Shared by auth
// and logout, which needs the raw token in order to revoke it.
func BearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// Auth resolves the bearer token to a user, or writes 401 and returns nil.
func (d *Deps) Auth(w http.ResponseWriter, r *http.Request) *models.User {
	u, err := d.Store.UserByToken(BearerToken(r))
	if err != nil {
		Fail(w, http.StatusUnauthorized, "unauthorized")
		return nil
	}
	return u
}

// AdminAuth resolves the bearer token and requires an admin account, or writes
// 401/403 and returns nil.
func (d *Deps) AdminAuth(w http.ResponseWriter, r *http.Request) *models.User {
	u := d.Auth(w, r)
	if u == nil {
		return nil
	}
	if !u.IsAdmin() {
		Fail(w, http.StatusForbidden, "admin access required")
		return nil
	}
	return u
}

// Deleted writes the standard response for a delete: 204 on success, or the
// mapped error. inUse is the message for a record other records still point at.
func (d *Deps) Deleted(w http.ResponseWriter, err error, what, inUse string) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrNotFound):
		Fail(w, http.StatusNotFound, what+" not found")
	case errors.Is(err, store.ErrInUse):
		Fail(w, http.StatusConflict, inUse)
	default:
		ServerError(w, err)
	}
}
