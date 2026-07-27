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

// deleted writes the standard response for a delete: 204 on success, or the
// mapped error. inUse is the message for a record other records still point at.
func (s *Server) deleted(w http.ResponseWriter, err error, what, inUse string) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrNotFound):
		fail(w, http.StatusNotFound, what+" not found")
	case errors.Is(err, ErrInUse):
		fail(w, http.StatusConflict, inUse)
	default:
		serverError(w, err)
	}
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
	var c Classification
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

// --- admin: user accounts ---

// scrub blanks password hashes before users go out in a response.
func scrub(users []*User) []*User {
	for _, u := range users {
		u.Password = ""
	}
	return users
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	q := r.URL.Query()
	users, err := s.store.Users(q.Get("keyword"), q.Get("role"))
	if err != nil {
		serverError(w, err)
		return
	}
	paginate(w, "users", scrub(users), r)
}

func (s *Server) adminGetUser(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	u, err := s.store.User(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	u.Password = ""
	writeJSON(w, http.StatusOK, u)
}

// adminUpdateUser edits any account, including its role. A password in the body
// is plaintext and gets hashed; leaving it out keeps the existing one.
func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	admin := s.adminAuth(w, r)
	if admin == nil {
		return
	}
	u, err := s.store.User(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	id, hash := u.ID, u.Password
	if readJSON(w, r, u) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	u.ID = id
	if u.Email == "" {
		fail(w, http.StatusBadRequest, "email required")
		return
	}
	// The stored value is a bcrypt hash, so anything different came from the
	// request body as plaintext and needs hashing.
	if u.Password != "" && u.Password != hash {
		if u.Password, err = HashPassword(u.Password); err != nil {
			serverError(w, err)
			return
		}
	} else {
		u.Password = hash
	}
	if u.Role != RoleAdmin {
		u.Role = RoleUser
	}
	// Guard against an admin locking themselves out of the admin routes.
	if u.ID == admin.ID && !u.IsAdmin() {
		fail(w, http.StatusBadRequest, "you cannot remove your own admin role")
		return
	}
	if err := s.store.UpdateUser(u); err != nil {
		if errors.Is(err, ErrDuplicate) {
			fail(w, http.StatusConflict, "email already registered")
			return
		}
		serverError(w, err)
		return
	}
	u.Password = ""
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	admin := s.adminAuth(w, r)
	if admin == nil {
		return
	}
	if r.PathValue("id") == admin.ID {
		fail(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	s.deleted(w, s.store.DeleteUser(r.PathValue("id")), "user",
		"user holds confirmed bookings: cancel those first")
}

// --- admin: bookings across all users ---

func (s *Server) adminListBookings(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	q := r.URL.Query()
	bookings, err := s.store.AllBookings(BookingFilter{
		UserID: q.Get("userId"), EventID: q.Get("eventId"), Status: q.Get("status"),
	})
	if err != nil {
		serverError(w, err)
		return
	}
	paginate(w, "bookings", bookings, r)
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
		if errors.Is(err, ErrNotFound) {
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
