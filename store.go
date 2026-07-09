package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrSoldOut      = errors.New("not enough tickets available")
	ErrUnauthorized = errors.New("unauthorized")
)

// Store is a thread-safe in-memory datastore. Swap the maps for a DB layer
// without touching the handlers.
type Store struct {
	mu       sync.RWMutex
	seq      int
	classes  map[string]*Classification
	attracts map[string]*Attraction
	venues   map[string]*Venue
	events   map[string]*Event
	users    map[string]*User
	bookings map[string]*Booking
	tokens   map[string]string // token -> userID
}

func NewStore() *Store {
	return &Store{
		classes:  map[string]*Classification{},
		attracts: map[string]*Attraction{},
		venues:   map[string]*Venue{},
		events:   map[string]*Event{},
		users:    map[string]*User{},
		bookings: map[string]*Booking{},
		tokens:   map[string]string{},
	}
}

func (s *Store) id(prefix string) string { s.seq++; return fmt.Sprintf("%s%03d", prefix, s.seq) }

func contains(hay, needle string) bool {
	return needle == "" || strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}

// --- Classifications ---

func (s *Store) Classifications() []*Classification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Classification{}
	for _, c := range s.classes {
		out = append(out, c)
	}
	return out
}

func (s *Store) Classification(id string) (*Classification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.classes[id]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}

// --- Attractions ---

func (s *Store) Attractions(keyword string) []*Attraction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Attraction{}
	for _, a := range s.attracts {
		if contains(a.Name, keyword) {
			out = append(out, a)
		}
	}
	return out
}

func (s *Store) Attraction(id string) (*Attraction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if a, ok := s.attracts[id]; ok {
		return a, nil
	}
	return nil, ErrNotFound
}

func (s *Store) CreateAttraction(a *Attraction) *Attraction {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.ID = s.id("K")
	s.attracts[a.ID] = a
	return a
}

// --- Venues ---

func (s *Store) Venues(keyword, city string) []*Venue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Venue{}
	for _, v := range s.venues {
		if contains(v.Name, keyword) && contains(v.City, city) {
			out = append(out, v)
		}
	}
	return out
}

func (s *Store) Venue(id string) (*Venue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.venues[id]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}

func (s *Store) CreateVenue(v *Venue) *Venue {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.ID = s.id("V")
	s.venues[v.ID] = v
	return v
}

// --- Events ---

type EventFilter struct {
	Keyword, City, ClassificationID string
	StartAfter                      time.Time
}

func (s *Store) Events(f EventFilter) []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Event{}
	for _, e := range s.events {
		if !contains(e.Name, f.Keyword) {
			continue
		}
		if f.ClassificationID != "" && e.ClassificationID != f.ClassificationID {
			continue
		}
		if !f.StartAfter.IsZero() && e.Date.Before(f.StartAfter) {
			continue
		}
		if f.City != "" {
			if v, ok := s.venues[e.VenueID]; !ok || !contains(v.City, f.City) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

func (s *Store) Event(id string) (*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.events[id]; ok {
		return e, nil
	}
	return nil, ErrNotFound
}

func (s *Store) CreateEvent(e *Event) *Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = s.id("E")
	if e.Status == "" {
		e.Status = "onsale"
	}
	s.events[e.ID] = e
	return e
}

// --- Users & auth ---

func (s *Store) Register(u *User) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	u.ID = s.id("U")
	s.users[u.ID] = u
	return u
}

func (s *Store) Login(email, password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Email == email && u.Password == password {
			tok := s.id("T")
			s.tokens[tok] = u.ID
			return tok, nil
		}
	}
	return "", ErrUnauthorized
}

func (s *Store) UserByToken(tok string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if uid, ok := s.tokens[tok]; ok {
		return s.users[uid], nil
	}
	return nil, ErrUnauthorized
}

// --- Bookings ---

func (s *Store) Book(userID, eventID string, qty int) (*Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[eventID]
	if !ok {
		return nil, ErrNotFound
	}
	if e.Status != "onsale" || qty < 1 || e.Available() < qty {
		return nil, ErrSoldOut
	}
	e.TicketsSold += qty
	b := &Booking{
		ID:        s.id("B"),
		UserID:    userID,
		EventID:   eventID,
		Quantity:  qty,
		Total:     e.PriceMin * float64(qty),
		Status:    "confirmed",
		CreatedAt: time.Now(),
	}
	s.bookings[b.ID] = b
	return b, nil
}

func (s *Store) Booking(id, userID string) (*Booking, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bookings[id]
	if !ok || b.UserID != userID {
		return nil, ErrNotFound
	}
	return b, nil
}

func (s *Store) UserBookings(userID string) []*Booking {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Booking{}
	for _, b := range s.bookings {
		if b.UserID == userID {
			out = append(out, b)
		}
	}
	return out
}

func (s *Store) CancelBooking(id, userID string) (*Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bookings[id]
	if !ok || b.UserID != userID {
		return nil, ErrNotFound
	}
	if b.Status == "confirmed" {
		b.Status = "cancelled"
		if e, ok := s.events[b.EventID]; ok {
			e.TicketsSold -= b.Quantity
		}
	}
	return b, nil
}
