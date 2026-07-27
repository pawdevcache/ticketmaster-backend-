package tm

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct{ store *Store }

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
func (s *Server) auth(w http.ResponseWriter, r *http.Request) *User {
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
func (s *Server) adminAuth(w http.ResponseWriter, r *http.Request) *User {
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

// --- discovery: events ---

func (s *Server) searchEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := EventFilter{Keyword: q.Get("keyword"), City: q.Get("city"), ClassificationID: q.Get("classificationId")}
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
	var e Event
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

// --- discovery: venues ---

func (s *Server) searchVenues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	venues, err := s.store.Venues(q.Get("keyword"), q.Get("city"))
	if err != nil {
		serverError(w, err)
		return
	}
	paginate(w, "venues", venues, r)
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
	var v Venue
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

// --- discovery: attractions ---

func (s *Server) searchAttractions(w http.ResponseWriter, r *http.Request) {
	attractions, err := s.store.Attractions(r.URL.Query().Get("keyword"))
	if err != nil {
		serverError(w, err)
		return
	}
	paginate(w, "attractions", attractions, r)
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
	var a Attraction
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

// --- users & auth ---

func (s *Server) register(w http.ResponseWriter, r *http.Request) { s.doRegister(w, r, RoleUser) }

// adminRegister creates an admin account. Because an admin can create events,
// venues and attractions, sign-up is gated behind ADMIN_REGISTRATION_KEY: the caller
// must present it in the X-Admin-Key header (or "adminKey" in the body).
// Registration is disabled outright while that variable is unset.
func (s *Server) adminRegister(w http.ResponseWriter, r *http.Request) {
	want := env("ADMIN_REGISTRATION_KEY", "")
	if want == "" {
		fail(w, http.StatusServiceUnavailable, "admin registration is disabled: set ADMIN_REGISTRATION_KEY")
		return
	}
	got := r.Header.Get("X-Admin-Key")
	if got == "" {
		// Fall back to the body so clients that can't set headers still work.
		// Read it from a copy — doRegister decodes the body again below.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			fail(w, http.StatusBadRequest, "invalid body")
			return
		}
		var k struct {
			AdminKey string `json:"adminKey"`
		}
		json.Unmarshal(body, &k) // a malformed body is reported by doRegister
		got = k.AdminKey
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		fail(w, http.StatusForbidden, "invalid admin key")
		return
	}
	s.doRegister(w, r, RoleAdmin)
}

// doRegister creates an account with the given role. The role always comes from
// the caller, never from the request body, so "role":"admin" in a payload to
// POST /api/register is ignored.
func (s *Server) doRegister(w http.ResponseWriter, r *http.Request, role string) {
	var u User
	if readJSON(w, r, &u) != nil || u.Name == "" || u.Email == "" || u.Password == "" {
		fail(w, http.StatusBadRequest, "name, email, password required")
		return
	}
	u.Role = role
	if err := s.store.Register(&u); err != nil {
		if errors.Is(err, ErrDuplicate) {
			fail(w, http.StatusConflict, "email already registered")
			return
		}
		serverError(w, err)
		return
	}
	u.Password = "" // hide from response
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) { s.doLogin(w, r, false) }

// adminLogin issues a token only for admin accounts. Ordinary users get the
// same "invalid credentials" response as a wrong password, so the endpoint
// doesn't reveal which emails belong to admins.
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) { s.doLogin(w, r, true) }

func (s *Server) doLogin(w http.ResponseWriter, r *http.Request, adminOnly bool) {
	var c struct{ Email, Password string }
	if readJSON(w, r, &c) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	tok, u, err := s.store.Login(c.Email, c.Password)
	if err != nil || (adminOnly && !u.IsAdmin()) {
		fail(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "user": u})
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
	case errors.Is(err, ErrNotFound):
		fail(w, http.StatusNotFound, "event not found")
	case errors.Is(err, ErrSoldOut):
		fail(w, http.StatusConflict, ErrSoldOut.Error())
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
