package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// --- booking administration (no per-user scoping) ---

// BookingFilter narrows an admin booking search. Empty fields are ignored.
type BookingFilter struct{ UserID, EventID, Status string }

// AllBookings returns one page of bookings across every user, plus the total
// number that matched. Unlike UserBookings there is no ownership constraint,
// so callers must confirm the requester is an admin.
func (s *Store) AllBookings(f BookingFilter, p Page) ([]*models.Booking, int64, error) {
	q := bson.D{}
	if f.UserID != "" {
		q = append(q, bson.E{Key: "userId", Value: f.UserID})
	}
	if f.EventID != "" {
		q = append(q, bson.E{Key: "eventId", Value: f.EventID})
	}
	if f.Status != "" {
		q = append(q, bson.E{Key: "status", Value: f.Status})
	}
	return findPage[models.Booking](s.bookings, q, p)
}

// BookingByID returns any booking regardless of owner, or ErrNotFound. Use
// Booking instead when acting on behalf of a specific user.
func (s *Store) BookingByID(id string) (*models.Booking, error) {
	return findOne[models.Booking](s.bookings, bson.D{{Key: "_id", Value: id}})
}

// AdminCancelBooking cancels any booking regardless of who owns it.
func (s *Store) AdminCancelBooking(id string) (*models.Booking, error) {
	b, err := s.BookingByID(id)
	if err != nil {
		return nil, err
	}
	return s.cancel(b)
}

// DeleteBooking erases a booking outright. A confirmed one releases its
// tickets first, so deleting never leaves an event overcounted.
func (s *Store) DeleteBooking(id string) error {
	b, err := s.BookingByID(id)
	if err != nil {
		return err
	}
	if b.Status == "confirmed" {
		cx, cancel := ctx()
		defer cancel()
		if err := s.releaseTickets(cx, b); err != nil {
			return err
		}
	}
	return remove(s.bookings, id)
}
