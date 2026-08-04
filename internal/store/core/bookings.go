// Booking machinery shared by both tiers: the user cancels their own
// booking, an admin cancels or deletes anyone's, and both must release
// seats the same way.
package core

import (
	"context"

	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// Cancel marks a booking cancelled and returns its tickets to the event. It is
// a no-op on an already-cancelled booking, so retries stay safe.
func (s *Store) Cancel(b *models.Booking) (*models.Booking, error) {
	if b.Status != "confirmed" {
		return b, nil
	}
	cx, cancel := Ctx()
	defer cancel()
	if _, err := s.BookingsCol.UpdateByID(cx, b.ID,
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "cancelled"}}}}); err != nil {
		return nil, err
	}
	if err := s.ReleaseTickets(cx, b); err != nil {
		return nil, err
	}
	b.Status = "cancelled"
	return b, nil
}

// WithEvents fills in the Event summary on each booking.
//
// It runs two queries no matter how many bookings are passed — one for the
// events, one for their venues — rather than a lookup per booking, which is
// the difference between a ticket list costing 2 round trips and costing 2N.
// A booking whose event has been deleted keeps a nil Event rather than failing
// the whole read.
func (s *Store) WithEvents(bookings []*models.Booking) error {
	if len(bookings) == 0 {
		return nil
	}
	eventIDs := make([]string, 0, len(bookings))
	seen := map[string]bool{}
	for _, b := range bookings {
		if b.EventID != "" && !seen[b.EventID] {
			seen[b.EventID] = true
			eventIDs = append(eventIDs, b.EventID)
		}
	}
	if len(eventIDs) == 0 {
		return nil
	}

	events, err := FindAll[models.Event](s.EventsCol, bson.D{
		{Key: "_id", Value: bson.D{{Key: "$in", Value: eventIDs}}},
	})
	if err != nil {
		return err
	}

	venueIDs := make([]string, 0, len(events))
	for _, e := range events {
		if e.VenueID != "" {
			venueIDs = append(venueIDs, e.VenueID)
		}
	}
	venues := map[string]*models.Venue{}
	if len(venueIDs) > 0 {
		found, err := FindAll[models.Venue](s.VenuesCol, bson.D{
			{Key: "_id", Value: bson.D{{Key: "$in", Value: venueIDs}}},
		})
		if err != nil {
			return err
		}
		for _, v := range found {
			venues[v.ID] = v
		}
	}

	summaries := make(map[string]*models.EventSummary, len(events))
	for _, e := range events {
		sum := &models.EventSummary{
			ID: e.ID, Name: e.Name, Date: e.Date, Status: e.Status, VenueID: e.VenueID,
		}
		if v := venues[e.VenueID]; v != nil {
			sum.VenueName, sum.VenueAddress, sum.VenueCity = v.Name, v.Address, v.City
		}
		summaries[e.ID] = sum
	}
	for _, b := range bookings {
		b.Event = summaries[b.EventID]
	}
	return nil
}

// ReleaseTickets hands a booking's seats back to its event.
func (s *Store) ReleaseTickets(cx context.Context, b *models.Booking) error {
	_, err := s.EventsCol.UpdateByID(cx, b.EventID,
		bson.D{{Key: "$inc", Value: bson.D{{Key: "ticketsSold", Value: -b.Quantity}}}})
	return err
}

// BookingByID returns any booking regardless of owner, or ErrNotFound. Use
// Booking instead when acting on behalf of a specific user.
func (s *Store) BookingByID(id string) (*models.Booking, error) {
	b, err := FindOne[models.Booking](s.BookingsCol, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return nil, err
	}
	return b, s.WithEvents([]*models.Booking{b})
}
