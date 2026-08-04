// Package user holds the store operations a signed-in customer performs on
// their own records. Every query here is scoped to one user id.
package user

import (
	"time"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Book reserves qty tickets on an event and records the booking. The seat
// count is decremented with a single conditional update, so two buyers racing
// for the last tickets can't both succeed. Returns core.ErrNotFound for an unknown
// event and core.ErrSoldOut when it isn't on sale or hasn't enough tickets left.
func (s *Store) Book(userID, eventID string, qty int) (*models.Booking, error) {
	if qty < 1 {
		return nil, core.ErrSoldOut
	}
	if _, err := s.Event(eventID); err != nil {
		return nil, err // core.ErrNotFound
	}
	// Atomically reserve tickets only if enough remain and the event is on sale.
	cx, cancel := core.Ctx()
	defer cancel()
	var e models.Event
	err := s.EventsCol.FindOneAndUpdate(cx,
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
		return nil, core.ErrSoldOut
	}
	if err != nil {
		return nil, err
	}
	// The seats are already reserved at this point, so a failure here would
	// leave them held against no booking. Generating the code is local and
	// cannot fail short of the OS running out of entropy, but check anyway
	// rather than issue a ticket nobody can scan.
	code, err := core.NewTicketCode()
	if err != nil {
		return nil, err
	}
	b := &models.Booking{
		ID: core.NewID(), UserID: userID, EventID: eventID, Quantity: qty,
		Total: e.PriceMin * float64(qty), Status: "confirmed", CreatedAt: time.Now(),
		TicketCode: code,
	}
	if err := core.Insert(s.BookingsCol, b); err != nil {
		return nil, err
	}
	// Attach the event so a client can show the ticket straight from the
	// booking response, without a follow-up request.
	return b, s.WithEvents([]*models.Booking{b})
}

// Booking returns one of userID's bookings. The user id is part of the query
// rather than checked afterwards, so one user can never read another's booking.
func (s *Store) Booking(id, userID string) (*models.Booking, error) {
	b, err := core.FindOne[models.Booking](s.BookingsCol, bson.D{{Key: "_id", Value: id}, {Key: "userId", Value: userID}})
	if err != nil {
		return nil, err
	}
	return b, s.WithEvents([]*models.Booking{b})
}

// UserBookings lists every booking belonging to one user, cancelled included.
func (s *Store) UserBookings(userID string) ([]*models.Booking, error) {
	bs, err := core.FindAll[models.Booking](s.BookingsCol, bson.D{{Key: "userId", Value: userID}})
	if err != nil {
		return nil, err
	}
	return bs, s.WithEvents(bs)
}

// CancelBooking cancels a booking the user owns and returns its tickets to the
// event. Returns core.ErrNotFound if the booking isn't theirs.
func (s *Store) CancelBooking(id, userID string) (*models.Booking, error) {
	b, err := s.Booking(id, userID)
	if err != nil {
		return nil, err
	}
	return s.Cancel(b)
}
