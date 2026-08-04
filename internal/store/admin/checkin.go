package admin

import (
	"time"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CheckInResult is why a scan was accepted or refused. The door needs to tell
// "let them in" apart from "this ticket already walked through" — refusing
// both the same way would make a duplicated ticket indistinguishable from a
// double scan of a genuine one.
type CheckInResult string

const (
	CheckInValid     CheckInResult = "valid"        // admitted, first scan
	CheckInUsed      CheckInResult = "already_used" // scanned before
	CheckInCancelled CheckInResult = "cancelled"    // booking was cancelled
	CheckInUnknown   CheckInResult = "not_found"    // no such ticket code
)

// CheckIn admits a ticket, returning the outcome and the booking it belongs to
// (nil when the code is unknown).
//
// The state change is a single conditional update rather than a read followed
// by a write: two doors scanning the same ticket at the same instant would
// otherwise both read "unused" and both admit. Only the update that matches
// checkedInAt: nil wins; the loser falls through to the already-used path.
func (s *Store) CheckIn(code string) (CheckInResult, *models.Booking, error) {
	if code == "" {
		return CheckInUnknown, nil, nil
	}
	cx, cancel := core.Ctx()
	defer cancel()

	now := time.Now()
	var b models.Booking
	err := s.BookingsCol.FindOneAndUpdate(cx,
		bson.D{
			{Key: "ticketCode", Value: code},
			{Key: "status", Value: "confirmed"},
			{Key: "checkedInAt", Value: nil},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "checkedInAt", Value: now}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&b)
	if err == nil {
		return CheckInValid, &b, s.WithEvents([]*models.Booking{&b})
	}
	if err != mongo.ErrNoDocuments {
		return "", nil, err
	}

	// The update matched nothing. Load the ticket to say why, so the door sees
	// "already used at 19:42" rather than a bare refusal.
	existing, err := core.FindOne[models.Booking](s.BookingsCol,
		bson.D{{Key: "ticketCode", Value: code}})
	if err != nil {
		return CheckInUnknown, nil, nil // ErrNotFound: unknown code
	}
	if err := s.WithEvents([]*models.Booking{existing}); err != nil {
		return "", nil, err
	}
	if existing.Status != "confirmed" {
		return CheckInCancelled, existing, nil
	}
	return CheckInUsed, existing, nil
}
