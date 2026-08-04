package user

import (
	"time"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
)

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
	u, err := core.FindOne[models.User](s.UsersCol, bson.D{{Key: "email", Value: email}})
	if err != nil {
		return "", err // core.ErrNotFound
	}
	if u.IsAdmin() {
		return "", core.ErrUnauthorized
	}
	tok, err := core.NewSecret()
	if err != nil {
		return "", err
	}
	cx, cancel := core.Ctx()
	defer cancel()
	// One live token per account: asking again invalidates the earlier one.
	if _, err := s.ResetsCol.DeleteMany(cx, bson.D{{Key: "userId", Value: u.ID}}); err != nil {
		return "", err
	}
	_, err = s.ResetsCol.InsertOne(cx, passwordReset{Token: tok, UserID: u.ID, CreatedAt: time.Now()})
	return tok, err
}

// ResetPassword consumes a reset token and sets the new password. The token
// works once; any existing login sessions are revoked, so a reset also kicks
// out whoever might already be signed in as that user.
func (s *Store) ResetPassword(token, newPassword string) error {
	if token == "" {
		return core.ErrUnauthorized
	}
	pr, err := core.FindOne[passwordReset](s.ResetsCol, bson.D{{Key: "_id", Value: token}})
	if err != nil {
		return core.ErrUnauthorized
	}
	// Mongo's TTL sweeper only runs about once a minute, so an expired token
	// can still be present. Enforce the deadline here rather than trust it.
	if time.Since(pr.CreatedAt) > core.ResetTTL {
		return core.ErrUnauthorized
	}
	hash, err := core.HashPassword(newPassword)
	if err != nil {
		return err
	}
	cx, cancel := core.Ctx()
	defer cancel()
	if _, err := s.UsersCol.UpdateByID(cx, pr.UserID,
		bson.D{{Key: "$set", Value: bson.D{{Key: "password", Value: hash}}}}); err != nil {
		return err
	}
	if _, err := s.ResetsCol.DeleteOne(cx, bson.D{{Key: "_id", Value: token}}); err != nil {
		return err
	}
	_, err = s.TokensCol.DeleteMany(cx, bson.D{{Key: "userId", Value: pr.UserID}})
	return err
}
