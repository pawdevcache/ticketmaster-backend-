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

// Users lists accounts, optionally filtered by name/email substring and role.
func (s *Store) Users(keyword, role string) ([]*models.User, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, bson.E{Key: "$or", Value: bson.A{
			bson.D{like("name", keyword)}, bson.D{like("email", keyword)},
		}})
	}
	if role != "" {
		f = append(f, bson.E{Key: "role", Value: role})
	}
	return findAll[models.User](s.users, f)
}

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
