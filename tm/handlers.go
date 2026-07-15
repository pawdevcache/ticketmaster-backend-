package tm

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct{ store *Store }

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// paginate slices items and wraps them in a Discovery-style envelope.
func paginate[T any](w http.ResponseWriter, key string, items []T, r *http.Request) {
	size := atoiDefault(r.URL.Query().Get("size"), 20)
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

// --- discovery: events ---

func (s *Server) searchEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := EventFilter{Keyword: q.Get("keyword"), City: q.Get("city"), ClassificationID: q.Get("classificationId")}
	if t, err := time.Parse(time.RFC3339, q.Get("startDateTime")); err == nil {
		f.StartAfter = t
	}
	events, err := s.store.Events(f)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
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
	var e Event
	if readJSON(r, &e) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.CreateEvent(&e); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// --- discovery: venues ---

func (s *Server) searchVenues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	venues, err := s.store.Venues(q.Get("keyword"), q.Get("city"))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
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
	var v Venue
	if readJSON(r, &v) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.CreateVenue(&v); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// --- discovery: attractions ---

func (s *Server) searchAttractions(w http.ResponseWriter, r *http.Request) {
	attractions, err := s.store.Attractions(r.URL.Query().Get("keyword"))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
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
	var a Attraction
	if readJSON(r, &a) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.CreateAttraction(&a); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// --- discovery: classifications ---

func (s *Server) searchClassifications(w http.ResponseWriter, r *http.Request) {
	classes, err := s.store.Classifications()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
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

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var u User
	if readJSON(r, &u) != nil || u.Email == "" || u.Password == "" {
		fail(w, http.StatusBadRequest, "name, email, password required")
		return
	}
	if err := s.store.Register(&u); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	u.Password = "" // hide from response
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c struct{ Email, Password string }
	if readJSON(r, &c) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	tok, err := s.store.Login(c.Email, c.Password)
	if err != nil {
		fail(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
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
	if readJSON(r, &req) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	b, err := s.store.Book(u.ID, req.EventID, req.Quantity)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) listBookings(w http.ResponseWriter, r *http.Request) {
	u := s.auth(w, r)
	if u == nil {
		return
	}
	bookings, err := s.store.UserBookings(u.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
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
