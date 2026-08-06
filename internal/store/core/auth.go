package core

import (
	"context"
	"errors"
	"time"

	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// --- Users & auth ---

// Register creates an account, replacing u.Password with its bcrypt hash. Any
// role other than admin is normalised to RoleUser. Returns ErrDuplicate when
// the email is already registered.
func (s *Store) Register(u *models.User) error {
	// Store only a bcrypt hash, never the plaintext password.
	hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.ID = NewID()
	u.Password = string(hash)
	if u.Role != models.RoleAdmin {
		u.Role = models.RoleUser
	}
	if err := Insert(s.UsersCol, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate // unique index on email (see EnsureIndexes)
		}
		return err
	}
	return nil
}

// Login verifies credentials and issues a token. The authenticated user is
// returned as well so callers can check the role before handing the token out.
func (s *Store) Login(email, password string) (string, *models.User, error) {
	u, err := FindOne[models.User](s.UsersCol, bson.D{{Key: "email", Value: email}})
	if err != nil {
		return "", nil, ErrUnauthorized
	}
	// Constant-time hash comparison; wrong password and unknown user look alike.
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) != nil {
		return "", nil, ErrUnauthorized
	}
	tok := NewID()
	if err := Insert(s.TokensCol, bson.D{
		{Key: "_id", Value: tok}, {Key: "userId", Value: u.ID}, {Key: "createdAt", Value: time.Now()},
	}); err != nil {
		return "", nil, err
	}
	u.Password = ""
	return tok, u, nil
}

// UserByToken resolves a bearer token to its account. Every failure — empty,
// unknown or expired token, missing user — comes back as ErrUnauthorized, so
// callers can't accidentally leak which one it was.
func (s *Store) UserByToken(tok string) (*models.User, error) {
	if tok == "" {
		return nil, ErrUnauthorized
	}
	t, err := FindOne[struct {
		UserID string `bson:"userId"`
	}](s.TokensCol, bson.D{{Key: "_id", Value: tok}})
	if err != nil {
		return nil, ErrUnauthorized
	}
	return s.UserByID(t.UserID)
}

// Logout revokes one session by deleting its token, leaving the account's
// other sessions alone — signing out of a laptop should not sign out a phone.
//
// Deleting a token that is already gone is not an error: signing out twice, or
// racing two logout requests, should both succeed.
func (s *Store) Logout(token string) error {
	if token == "" {
		return nil
	}
	cx, cancel := Ctx()
	defer cancel()
	_, err := s.TokensCol.DeleteOne(cx, bson.D{{Key: "_id", Value: token}})
	return err
}

// EnsureIndexes enforces a unique email and auto-expiring login tokens.
// Best-effort: called once at startup.
func (s *Store) EnsureIndexes() error {
	cx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.UsersCol.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}
	if _, err := s.TokensCol.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "createdAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(tokenTTL.Seconds())),
	}); err != nil {
		return err
	}
	if _, err := s.ResetsCol.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "createdAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(ResetTTL.Seconds())),
	}); err != nil {
		return err
	}
	// One row per user per term, so repeating a search updates its time
	// instead of filling the history with near-duplicates.
	if _, err := s.SearchesCol.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "term", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}
	// History is a convenience, not a record worth keeping forever.
	if _, err := s.SearchesCol.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "searchedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(searchTTL.Seconds())),
	}); err != nil {
		return err
	}
	// Ticket codes are looked up on every scan, and a duplicate would let one
	// scan admit the wrong booking. Sparse, because bookings made before
	// ticket codes existed have no value to index.
	_, err := s.BookingsCol.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "ticketCode", Value: 1}},
		Options: options.Index().SetUnique(true).SetSparse(true),
	})
	return err
}

// EnsureTicketCodes gives a ticket code to bookings created before codes
// existed, so their QR codes still scan. Best-effort and called once at
// startup, like the other Ensure funcs; after the first run it matches
// nothing and costs a single indexed query.
func (s *Store) EnsureTicketCodes() (int, error) {
	cx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cur, err := s.BookingsCol.Find(cx,
		bson.D{{Key: "ticketCode", Value: bson.D{{Key: "$exists", Value: false}}}},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return 0, err
	}
	var rows []struct {
		ID string `bson:"_id"`
	}
	if err := cur.All(cx, &rows); err != nil {
		return 0, err
	}
	for _, r := range rows {
		code, err := NewTicketCode()
		if err != nil {
			return 0, err
		}
		if _, err := s.BookingsCol.UpdateByID(cx, r.ID,
			bson.D{{Key: "$set", Value: bson.D{{Key: "ticketCode", Value: code}}}}); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// EnsureAdmin seeds a bootstrap admin so there is someone to sign in as before
// any admin exists. It creates the account when the email is unknown, and only
// promotes the role when it already exists — an existing password is never
// overwritten, so rotating the env var can't hijack a live account.
func (s *Store) EnsureAdmin(name, email, password string) error {
	if email == "" || password == "" {
		return nil // seeding not configured
	}
	u, err := FindOne[models.User](s.UsersCol, bson.D{{Key: "email", Value: email}})
	if err == nil {
		if u.IsAdmin() {
			return nil
		}
		cx, cancel := Ctx()
		defer cancel()
		_, err = s.UsersCol.UpdateByID(cx, u.ID, bson.D{{Key: "$set", Value: bson.D{{Key: "role", Value: models.RoleAdmin}}}})
		return err
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	if name == "" {
		name = "Admin"
	}
	admin := &models.User{Name: name, Email: email, Password: password, Role: models.RoleAdmin}
	if err := s.Register(admin); err != nil && !errors.Is(err, ErrDuplicate) {
		return err // a duplicate means a concurrent start won the race — fine
	}
	return nil
}

func (s *Store) UserByID(id string) (*models.User, error) {
	return FindOne[models.User](s.UsersCol, bson.D{{Key: "_id", Value: id}})
}
