package user

import (
	"time"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ExpireHolds returns the seats of any lapsed unpaid booking to its event and
// marks the booking expired. Pass an empty eventID to sweep everything.
//
// Each booking is claimed with a conditional update before its seats are
// released, so two concurrent sweeps — or a sweep racing a payment — cannot
// both act on the same booking and release its seats twice. A booking that
// slips through is picked up by the next sweep; one released twice would
// corrupt the event's ticket count permanently.
func (s *Store) ExpireHolds(eventID string) (int, error) {
	cx, cancel := core.Ctx()
	defer cancel()

	q := bson.D{
		{Key: "status", Value: models.BookingPending},
		{Key: "holdExpiresAt", Value: bson.D{{Key: "$lt", Value: time.Now()}}},
	}
	if eventID != "" {
		q = append(q, bson.E{Key: "eventId", Value: eventID})
	}
	cur, err := s.BookingsCol.Find(cx, q, options.Find().SetLimit(500))
	if err != nil {
		return 0, err
	}
	var stale []*models.Booking
	if err := cur.All(cx, &stale); err != nil {
		return 0, err
	}

	expired := 0
	for _, b := range stale {
		// Claim it: only the update that finds it still pending proceeds.
		res, err := s.BookingsCol.UpdateOne(cx,
			bson.D{{Key: "_id", Value: b.ID}, {Key: "status", Value: models.BookingPending}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: models.BookingExpired}}},
				{Key: "$unset", Value: bson.D{{Key: "holdExpiresAt", Value: ""}}}})
		if err != nil {
			return expired, err
		}
		if res.ModifiedCount == 0 {
			continue // someone else got there first
		}
		if err := s.ReleaseTickets(cx, b); err != nil {
			return expired, err
		}
		expired++
	}
	return expired, nil
}

// MarkPaid turns a paid hold into a confirmed booking.
//
// The status change is conditional on the booking still being pending, so a
// duplicate call — a double-clicked pay button, or a retry after a dropped
// response — cannot double-confirm or resurrect a hold that already expired.
// Returns core.ErrNotFound when the booking is not the caller's, and
// core.ErrSoldOut when it is no longer awaiting payment.
func (s *Store) MarkPaid(bookingID, userID, intentID string) (*models.Booking, error) {
	cx, cancel := core.Ctx()
	defer cancel()

	now := time.Now()
	var b models.Booking
	err := s.BookingsCol.FindOneAndUpdate(cx,
		bson.D{
			{Key: "_id", Value: bookingID},
			{Key: "userId", Value: userID},
			{Key: "status", Value: models.BookingPending},
		},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "status", Value: models.BookingConfirmed},
				{Key: "paidAt", Value: now},
				{Key: "paymentIntentId", Value: intentID},
			}},
			{Key: "$unset", Value: bson.D{{Key: "holdExpiresAt", Value: ""}}},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&b)
	if err == nil {
		return &b, s.WithEvents([]*models.Booking{&b})
	}
	if err != mongo.ErrNoDocuments {
		return nil, err
	}

	// Say why it did not apply: the booking may not exist, may belong to
	// someone else, or may simply no longer be pending.
	existing, ferr := core.FindOne[models.Booking](s.BookingsCol,
		bson.D{{Key: "_id", Value: bookingID}, {Key: "userId", Value: userID}})
	if ferr != nil {
		return nil, core.ErrNotFound
	}
	return existing, core.ErrSoldOut // wrong state, not a missing record
}

// PendingBooking returns one of the caller's bookings awaiting payment.
func (s *Store) PendingBooking(bookingID, userID string) (*models.Booking, error) {
	return core.FindOne[models.Booking](s.BookingsCol, bson.D{
		{Key: "_id", Value: bookingID},
		{Key: "userId", Value: userID},
		{Key: "status", Value: models.BookingPending},
	})
}

// SetPaymentIntent records the intent a hold is waiting on, so a resumed
// checkout reuses the same charge instead of creating a second one.
func (s *Store) SetPaymentIntent(bookingID, intentID string) error {
	cx, cancel := core.Ctx()
	defer cancel()
	_, err := s.BookingsCol.UpdateByID(cx, bookingID,
		bson.D{{Key: "$set", Value: bson.D{{Key: "paymentIntentId", Value: intentID}}}})
	return err
}
