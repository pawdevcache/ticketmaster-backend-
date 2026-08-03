package store

import (
	"context"
	"time"

	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- Bookings ---

// Book reserves qty tickets on an event and records the booking. The seat
// count is decremented with a single conditional update, so two buyers racing
// for the last tickets can't both succeed. Returns ErrNotFound for an unknown
// event and ErrSoldOut when it isn't on sale or hasn't enough tickets left.
func (s *Store) Book(userID, eventID string, qty int) (*models.Booking, error) {
	if qty < 1 {
		return nil, ErrSoldOut
	}
	if _, err := s.Event(eventID); err != nil {
		return nil, err // ErrNotFound
	}
	// Atomically reserve tickets only if enough remain and the event is on sale.
	cx, cancel := ctx()
	defer cancel()
	var e models.Event
	err := s.events.FindOneAndUpdate(cx,
		bson.D{
			{Key: "_id", Value: eventID},
			{Key: "status", Value: "onsale"},
			{Key: "$expr", Value: bson.D{{Key: "$gte", Value: bson.A{
				bson.D{{Key: "$subtract", Value: bson.A{"$ticketsTotal", "$ticketsSold"}}}, qty}}}},
		},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "ticketsSold", Value: qty}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&e)
	if err == mongo.ErrNoDocuments {
		return nil, ErrSoldOut
	}
	if err != nil {
		return nil, err
	}
	b := &models.Booking{
		ID: newID(), UserID: userID, EventID: eventID, Quantity: qty,
		Total: e.PriceMin * float64(qty), Status: "confirmed", CreatedAt: time.Now(),
	}
	if err := insert(s.bookings, b); err != nil {
		return nil, err
	}
	// Attach the event so a client can show the ticket straight from the
	// booking response, without a follow-up request.
	return b, s.withEvents([]*models.Booking{b})
}

// Booking returns one of userID's bookings. The user id is part of the query
// rather than checked afterwards, so one user can never read another's booking.
func (s *Store) Booking(id, userID string) (*models.Booking, error) {
	b, err := findOne[models.Booking](s.bookings, bson.D{{Key: "_id", Value: id}, {Key: "userId", Value: userID}})
	if err != nil {
		return nil, err
	}
	return b, s.withEvents([]*models.Booking{b})
}

// UserBookings lists every booking belonging to one user, cancelled included.
func (s *Store) UserBookings(userID string) ([]*models.Booking, error) {
	bs, err := findAll[models.Booking](s.bookings, bson.D{{Key: "userId", Value: userID}})
	if err != nil {
		return nil, err
	}
	return bs, s.withEvents(bs)
}

// CancelBooking cancels a booking the user owns and returns its tickets to the
// event. Returns ErrNotFound if the booking isn't theirs.
func (s *Store) CancelBooking(id, userID string) (*models.Booking, error) {
	b, err := s.Booking(id, userID)
	if err != nil {
		return nil, err
	}
	return s.cancel(b)
}

// cancel marks a booking cancelled and returns its tickets to the event. It is
// a no-op on an already-cancelled booking, so retries stay safe.
func (s *Store) cancel(b *models.Booking) (*models.Booking, error) {
	if b.Status != "confirmed" {
		return b, nil
	}
	cx, cancel := ctx()
	defer cancel()
	if _, err := s.bookings.UpdateByID(cx, b.ID,
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "cancelled"}}}}); err != nil {
		return nil, err
	}
	if err := s.releaseTickets(cx, b); err != nil {
		return nil, err
	}
	b.Status = "cancelled"
	return b, nil
}

// withEvents fills in the Event summary on each booking.
//
// It runs two queries no matter how many bookings are passed — one for the
// events, one for their venues — rather than a lookup per booking, which is
// the difference between a ticket list costing 2 round trips and costing 2N.
// A booking whose event has been deleted keeps a nil Event rather than failing
// the whole read.
func (s *Store) withEvents(bookings []*models.Booking) error {
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

	events, err := findAll[models.Event](s.events, bson.D{
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
		found, err := findAll[models.Venue](s.venues, bson.D{
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
			sum.VenueName, sum.VenueCity = v.Name, v.City
		}
		summaries[e.ID] = sum
	}
	for _, b := range bookings {
		b.Event = summaries[b.EventID]
	}
	return nil
}

// releaseTickets hands a booking's seats back to its event.
func (s *Store) releaseTickets(cx context.Context, b *models.Booking) error {
	_, err := s.events.UpdateByID(cx, b.EventID,
		bson.D{{Key: "$inc", Value: bson.D{{Key: "ticketsSold", Value: -b.Quantity}}}})
	return err
}
