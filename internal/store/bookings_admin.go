package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// --- booking administration (no per-user scoping) ---

type BookingFilter struct{ UserID, EventID, Status string }

func (s *Store) AllBookings(f BookingFilter) ([]*models.Booking, error) {
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
	return findAll[models.Booking](s.bookings, q)
}

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
