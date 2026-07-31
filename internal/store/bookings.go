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
	return b, insert(s.bookings, b)
}

func (s *Store) Booking(id, userID string) (*models.Booking, error) {
	return findOne[models.Booking](s.bookings, bson.D{{Key: "_id", Value: id}, {Key: "userId", Value: userID}})
}

func (s *Store) UserBookings(userID string) ([]*models.Booking, error) {
	return findAll[models.Booking](s.bookings, bson.D{{Key: "userId", Value: userID}})
}

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

// releaseTickets hands a booking's seats back to its event.
func (s *Store) releaseTickets(cx context.Context, b *models.Booking) error {
	_, err := s.events.UpdateByID(cx, b.EventID,
		bson.D{{Key: "$inc", Value: bson.D{{Key: "ticketsSold", Value: -b.Quantity}}}})
	return err
}
