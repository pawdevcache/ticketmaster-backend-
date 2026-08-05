package core

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword produces the stored form of a password. Exported so handlers
// can hash a replacement password during an update.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

// CheckPassword reports whether plain matches the stored hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// UpdateUser expects u.Password to already hold a bcrypt hash.
func (s *Store) UpdateUser(u *models.User) error {
	if err := Replace(s.UsersCol, u.ID, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

// RevokeOtherSessions signs the account out everywhere except the token making
// the request. Changing a password should not leave a stolen session alive,
// but logging the user out of the device they are using to change it is a
// gratuitous annoyance.
func (s *Store) RevokeOtherSessions(userID, keep string) error {
	cx, cancel := Ctx()
	defer cancel()
	_, err := s.TokensCol.DeleteMany(cx, bson.D{
		{Key: "userId", Value: userID},
		{Key: "_id", Value: bson.D{{Key: "$ne", Value: keep}}},
	})
	return err
}
