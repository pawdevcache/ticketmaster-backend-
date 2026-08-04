package admin

import (
	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Users returns one page of accounts, optionally filtered by a name/email
// substring and by role, plus the total number that matched.
func (s *Store) Users(keyword, role string, p core.Page) ([]*models.User, int64, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, bson.E{Key: "$or", Value: bson.A{
			bson.D{core.Like("name", keyword)}, bson.D{core.Like("email", keyword)},
		}})
	}
	if role != "" {
		f = append(f, bson.E{Key: "role", Value: role})
	}
	return core.FindPage[models.User](s.UsersCol, f, p)
}

// User returns a single account, or core.ErrNotFound. The returned User still holds
// the password hash, so callers must blank it before it reaches a response.
func (s *Store) User(id string) (*models.User, error) { return s.UserByID(id) }

// UpdateUser expects u.Password to already hold a bcrypt hash.
func (s *Store) UpdateUser(u *models.User) error {
	if err := core.Replace(s.UsersCol, u.ID, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return core.ErrDuplicate
		}
		return err
	}
	return nil
}

// DeleteUser removes an account and revokes its login tokens. It refuses while
// the user holds confirmed bookings, so ticket holders can't vanish silently.
func (s *Store) DeleteUser(id string) error {
	booked, err := core.AnyMatch(s.BookingsCol, bson.D{
		{Key: "userId", Value: id},
		activelyHeld(),
	})
	if err != nil {
		return err
	}
	if booked {
		return core.ErrInUse
	}
	if err := core.Remove(s.UsersCol, id); err != nil {
		return err
	}
	cx, cancel := core.Ctx()
	defer cancel()
	_, err = s.TokensCol.DeleteMany(cx, bson.D{{Key: "userId", Value: id}})
	return err
}
