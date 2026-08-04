package admin

import (
	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
)

// BookingFilter narrows an admin booking search. Empty fields are ignored.
type BookingFilter struct{ UserID, EventID, Status string }

// AllBookings returns one page of bookings across every user, plus the total
// number that matched. Unlike UserBookings there is no ownership constraint,
// so callers must confirm the requester is an admin.
func (s *Store) AllBookings(f BookingFilter, p core.Page) ([]*models.Booking, int64, error) {
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
	bs, total, err := core.FindPage[models.Booking](s.BookingsCol, q, p)
	if err != nil {
		return nil, 0, err
	}
	return bs, total, s.WithEvents(bs)
}

// AdminCancelBooking cancels any booking regardless of who owns it.
func (s *Store) AdminCancelBooking(id string) (*models.Booking, error) {
	b, err := s.BookingByID(id)
	if err != nil {
		return nil, err
	}
	return s.Cancel(b)
}

// DeleteBooking erases a booking outright. A confirmed one releases its
// tickets first, so deleting never leaves an event overcounted.
func (s *Store) DeleteBooking(id string) error {
	b, err := s.BookingByID(id)
	if err != nil {
		return err
	}
	if b.Status == "confirmed" {
		cx, cancel := core.Ctx()
		defer cancel()
		if err := s.ReleaseTickets(cx, b); err != nil {
			return err
		}
	}
	return core.Remove(s.BookingsCol, id)
}
