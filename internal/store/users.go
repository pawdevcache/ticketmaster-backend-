package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

// --- user administration ---

// HashPassword produces the stored form of a password. Exported so handlers
// can hash a replacement password during an update.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

// Users returns one page of accounts, optionally filtered by a name/email
// substring and by role, plus the total number that matched.
func (s *Store) Users(keyword, role string, p Page) ([]*models.User, int64, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, bson.E{Key: "$or", Value: bson.A{
			bson.D{like("name", keyword)}, bson.D{like("email", keyword)},
		}})
	}
	if role != "" {
		f = append(f, bson.E{Key: "role", Value: role})
	}
	return findPage[models.User](s.users, f, p)
}

// User returns a single account, or ErrNotFound. The returned User still holds
// the password hash, so callers must blank it before it reaches a response.
func (s *Store) User(id string) (*models.User, error) { return s.userByID(id) }

// UpdateUser expects u.Password to already hold a bcrypt hash.
func (s *Store) UpdateUser(u *models.User) error {
	if err := replace(s.users, u.ID, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}
