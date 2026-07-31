package store

import (
	"time"

	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// --- forgotten passwords ---

// passwordReset is a single-use ticket proving the bearer asked for a reset.
type passwordReset struct {
	Token     string    `bson:"_id"`
	UserID    string    `bson:"userId"`
	CreatedAt time.Time `bson:"createdAt"`
}

// CreateReset issues a reset token for the account with the given email.
// Admin accounts are refused: an admin password is changed from the admin
// console, so a public endpoint can never be a route into one.
//
// Callers must not tell the client which error came back — that would turn
// this into a way to test which email addresses have accounts.
func (s *Store) CreateReset(email string) (string, error) {
	u, err := findOne[models.User](s.users, bson.D{{Key: "email", Value: email}})
	if err != nil {
		return "", err // ErrNotFound
	}
	if u.IsAdmin() {
		return "", ErrUnauthorized
	}
	tok, err := newSecret()
	if err != nil {
		return "", err
	}
	cx, cancel := ctx()
	defer cancel()
	// One live token per account: asking again invalidates the earlier one.
	if _, err := s.resets.DeleteMany(cx, bson.D{{Key: "userId", Value: u.ID}}); err != nil {
		return "", err
	}
	_, err = s.resets.InsertOne(cx, passwordReset{Token: tok, UserID: u.ID, CreatedAt: time.Now()})
	return tok, err
}

// ResetPassword consumes a reset token and sets the new password. The token
// works once; any existing login sessions are revoked, so a reset also kicks
// out whoever might already be signed in as that user.
func (s *Store) ResetPassword(token, newPassword string) error {
	if token == "" {
		return ErrUnauthorized
	}
	pr, err := findOne[passwordReset](s.resets, bson.D{{Key: "_id", Value: token}})
	if err != nil {
		return ErrUnauthorized
	}
	// Mongo's TTL sweeper only runs about once a minute, so an expired token
	// can still be present. Enforce the deadline here rather than trust it.
	if time.Since(pr.CreatedAt) > ResetTTL {
		return ErrUnauthorized
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	cx, cancel := ctx()
	defer cancel()
	if _, err := s.users.UpdateByID(cx, pr.UserID,
		bson.D{{Key: "$set", Value: bson.D{{Key: "password", Value: hash}}}}); err != nil {
		return err
	}
	if _, err := s.resets.DeleteOne(cx, bson.D{{Key: "_id", Value: token}}); err != nil {
		return err
	}
	_, err = s.tokens.DeleteMany(cx, bson.D{{Key: "userId", Value: pr.UserID}})
	return err
}

// DeleteUser removes an account and revokes its login tokens. It refuses while
// the user holds confirmed bookings, so ticket holders can't vanish silently.
func (s *Store) DeleteUser(id string) error {
	booked, err := anyMatch(s.bookings, bson.D{
		{Key: "userId", Value: id}, {Key: "status", Value: "confirmed"},
	})
	if err != nil {
		return err
	}
	if booked {
		return ErrInUse
	}
	if err := remove(s.users, id); err != nil {
		return err
	}
	cx, cancel := ctx()
	defer cancel()
	_, err = s.tokens.DeleteMany(cx, bson.D{{Key: "userId", Value: id}})
	return err
}
