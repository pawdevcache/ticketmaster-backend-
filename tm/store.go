package tm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrSoldOut      = errors.New("not enough tickets available")
	ErrUnauthorized = errors.New("unauthorized")
	ErrDuplicate    = errors.New("email already registered")
	// ErrInUse blocks deleting a record other records still point at, so an
	// admin can't silently orphan events, bookings or attractions.
	ErrInUse = errors.New("still referenced by other records")
)

// tokenTTL is how long a login token stays valid before Mongo's TTL index
// deletes it (enforced by EnsureIndexes).
const tokenTTL = 24 * time.Hour

// redactURI strips credentials so a connection string is safe to log or return.
func redactURI(uri string) string {
	scheme := strings.Index(uri, "://")
	at := strings.LastIndex(uri, "@")
	if scheme >= 0 && at > scheme {
		return uri[:scheme+3] + "***@" + uri[at+1:]
	}
	return uri
}

// Store is a MongoDB-backed datastore.
type Store struct {
	uri      string
	client   *mongo.Client
	db       *mongo.Database
	classes  *mongo.Collection
	attracts *mongo.Collection
	venues   *mongo.Collection
	events   *mongo.Collection
	users    *mongo.Collection
	bookings *mongo.Collection
	tokens   *mongo.Collection
}

func NewStore(uri, dbName string) (*Store, error) {
	opts := options.Client().ApplyURI(uri).
		SetServerSelectionTimeout(5 * time.Second).
		SetConnectTimeout(5 * time.Second)
	// Connect is non-blocking (it starts background monitoring) and does NOT
	// verify the DB is reachable. We deliberately skip Ping here so the process
	// always starts — a down database must not kill the server on startup, only
	// make individual requests fail. Use Ping() for health checks.
	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("mongo config %s: %w", redactURI(uri), err)
	}
	db := client.Database(dbName)
	return &Store{
		uri:      uri,
		client:   client,
		db:       db,
		classes:  db.Collection("classifications"),
		attracts: db.Collection("attractions"),
		venues:   db.Collection("venues"),
		events:   db.Collection("events"),
		users:    db.Collection("users"),
		bookings: db.Collection("bookings"),
		tokens:   db.Collection("tokens"),
	}, nil
}

// Ping verifies the database is actually reachable. Used by /health.
func (s *Store) Ping() error {
	cx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.client.Ping(cx, nil); err != nil {
		return fmt.Errorf("mongo unreachable at %s: %w", redactURI(s.uri), err)
	}
	return nil
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func newID() string { return primitive.NewObjectID().Hex() }

// like builds a case-insensitive "contains" regex filter, or nil to skip.
func like(field, value string) bson.E {
	return bson.E{Key: field, Value: primitive.Regex{Pattern: value, Options: "i"}}
}

func findAll[T any](c *mongo.Collection, filter bson.D) ([]*T, error) {
	cx, cancel := ctx()
	defer cancel()
	cur, err := c.Find(cx, filter)
	if err != nil {
		return nil, err
	}
	out := []*T{}
	if err := cur.All(cx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func findOne[T any](c *mongo.Collection, filter bson.D) (*T, error) {
	cx, cancel := ctx()
	defer cancel()
	var v T
	err := c.FindOne(cx, filter).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &v, err
}

func insert(c *mongo.Collection, doc any) error {
	cx, cancel := ctx()
	defer cancel()
	_, err := c.InsertOne(cx, doc)
	return err
}

// replace overwrites the document with the given id. Callers build the new
// document by decoding the request over the stored one, so fields the client
// left out keep their current values.
func replace(c *mongo.Collection, id string, doc any) error {
	cx, cancel := ctx()
	defer cancel()
	res, err := c.ReplaceOne(cx, bson.D{{Key: "_id", Value: id}}, doc)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func remove(c *mongo.Collection, id string) error {
	cx, cancel := ctx()
	defer cancel()
	res, err := c.DeleteOne(cx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// anyMatch reports whether at least one document matches — used by the delete
// guards to detect records that are still referenced.
func anyMatch(c *mongo.Collection, filter bson.D) (bool, error) {
	cx, cancel := ctx()
	defer cancel()
	n, err := c.CountDocuments(cx, filter, options.Count().SetLimit(1))
	return n > 0, err
}

// --- Classifications ---

func (s *Store) Classifications() ([]*Classification, error) {
	return findAll[Classification](s.classes, bson.D{})
}
func (s *Store) Classification(id string) (*Classification, error) {
	return findOne[Classification](s.classes, bson.D{{Key: "_id", Value: id}})
}
func (s *Store) CreateClassification(c *Classification) error {
	c.ID = newID()
	return insert(s.classes, c)
}
func (s *Store) UpdateClassification(c *Classification) error {
	return replace(s.classes, c.ID, c)
}

// DeleteClassification refuses while events or attractions still classify
// themselves under it.
func (s *Store) DeleteClassification(id string) error {
	for _, c := range []*mongo.Collection{s.events, s.attracts} {
		used, err := anyMatch(c, bson.D{{Key: "classificationId", Value: id}})
		if err != nil {
			return err
		}
		if used {
			return ErrInUse
		}
	}
	return remove(s.classes, id)
}

// --- Attractions ---

func (s *Store) Attractions(keyword string) ([]*Attraction, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, like("name", keyword))
	}
	return findAll[Attraction](s.attracts, f)
}
func (s *Store) Attraction(id string) (*Attraction, error) {
	return findOne[Attraction](s.attracts, bson.D{{Key: "_id", Value: id}})
}
func (s *Store) CreateAttraction(a *Attraction) error {
	a.ID = newID()
	return insert(s.attracts, a)
}
func (s *Store) UpdateAttraction(a *Attraction) error {
	return replace(s.attracts, a.ID, a)
}

// DeleteAttraction refuses while any event still lists it in attractionIds.
func (s *Store) DeleteAttraction(id string) error {
	used, err := anyMatch(s.events, bson.D{{Key: "attractionIds", Value: id}})
	if err != nil {
		return err
	}
	if used {
		return ErrInUse
	}
	return remove(s.attracts, id)
}

// --- Venues ---

func (s *Store) Venues(keyword, city string) ([]*Venue, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, like("name", keyword))
	}
	if city != "" {
		f = append(f, like("city", city))
	}
	return findAll[Venue](s.venues, f)
}
func (s *Store) Venue(id string) (*Venue, error) {
	return findOne[Venue](s.venues, bson.D{{Key: "_id", Value: id}})
}
func (s *Store) CreateVenue(v *Venue) error {
	v.ID = newID()
	return insert(s.venues, v)
}
func (s *Store) UpdateVenue(v *Venue) error {
	return replace(s.venues, v.ID, v)
}

// DeleteVenue refuses while any event is still scheduled there.
func (s *Store) DeleteVenue(id string) error {
	used, err := anyMatch(s.events, bson.D{{Key: "venueId", Value: id}})
	if err != nil {
		return err
	}
	if used {
		return ErrInUse
	}
	return remove(s.venues, id)
}

// --- Events ---

type EventFilter struct {
	Keyword, City, ClassificationID string
	StartAfter                      time.Time
}

func (s *Store) Events(f EventFilter) ([]*Event, error) {
	q := bson.D{}
	if f.Keyword != "" {
		q = append(q, like("name", f.Keyword))
	}
	if f.ClassificationID != "" {
		q = append(q, bson.E{Key: "classificationId", Value: f.ClassificationID})
	}
	if !f.StartAfter.IsZero() {
		q = append(q, bson.E{Key: "date", Value: bson.D{{Key: "$gte", Value: f.StartAfter}}})
	}
	if f.City != "" {
		vs, err := s.Venues("", f.City)
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(vs))
		for i, v := range vs {
			ids[i] = v.ID
		}
		q = append(q, bson.E{Key: "venueId", Value: bson.D{{Key: "$in", Value: ids}}})
	}
	return findAll[Event](s.events, q)
}
func (s *Store) Event(id string) (*Event, error) {
	return findOne[Event](s.events, bson.D{{Key: "_id", Value: id}})
}
func (s *Store) CreateEvent(e *Event) error {
	e.ID = newID()
	if e.Status == "" {
		e.Status = "onsale"
	}
	return insert(s.events, e)
}
func (s *Store) UpdateEvent(e *Event) error {
	return replace(s.events, e.ID, e)
}

// DeleteEvent refuses while confirmed bookings exist — cancel them first, so
// nobody loses a ticket they paid for. Cancelled bookings don't block it.
func (s *Store) DeleteEvent(id string) error {
	booked, err := anyMatch(s.bookings, bson.D{
		{Key: "eventId", Value: id}, {Key: "status", Value: "confirmed"},
	})
	if err != nil {
		return err
	}
	if booked {
		return ErrInUse
	}
	return remove(s.events, id)
}

// --- Users & auth ---

func (s *Store) Register(u *User) error {
	// Store only a bcrypt hash, never the plaintext password.
	hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.ID = newID()
	u.Password = string(hash)
	if u.Role != RoleAdmin {
		u.Role = RoleUser
	}
	if err := insert(s.users, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate // unique index on email (see EnsureIndexes)
		}
		return err
	}
	return nil
}

// Login verifies credentials and issues a token. The authenticated user is
// returned as well so callers can check the role before handing the token out.
func (s *Store) Login(email, password string) (string, *User, error) {
	u, err := findOne[User](s.users, bson.D{{Key: "email", Value: email}})
	if err != nil {
		return "", nil, ErrUnauthorized
	}
	// Constant-time hash comparison; wrong password and unknown user look alike.
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) != nil {
		return "", nil, ErrUnauthorized
	}
	tok := newID()
	if err := insert(s.tokens, bson.D{
		{Key: "_id", Value: tok}, {Key: "userId", Value: u.ID}, {Key: "createdAt", Value: time.Now()},
	}); err != nil {
		return "", nil, err
	}
	u.Password = ""
	return tok, u, nil
}

func (s *Store) UserByToken(tok string) (*User, error) {
	if tok == "" {
		return nil, ErrUnauthorized
	}
	t, err := findOne[struct {
		UserID string `bson:"userId"`
	}](s.tokens, bson.D{{Key: "_id", Value: tok}})
	if err != nil {
		return nil, ErrUnauthorized
	}
	return s.userByID(t.UserID)
}

// EnsureIndexes enforces a unique email and auto-expiring login tokens.
// Best-effort: called once at startup.
func (s *Store) EnsureIndexes() error {
	cx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.users.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}
	_, err := s.tokens.Indexes().CreateOne(cx, mongo.IndexModel{
		Keys:    bson.D{{Key: "createdAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(tokenTTL.Seconds())),
	})
	return err
}

// EnsureAdmin seeds a bootstrap admin so there is someone to sign in as before
// any admin exists. It creates the account when the email is unknown, and only
// promotes the role when it already exists — an existing password is never
// overwritten, so rotating the env var can't hijack a live account.
func (s *Store) EnsureAdmin(name, email, password string) error {
	if email == "" || password == "" {
		return nil // seeding not configured
	}
	u, err := findOne[User](s.users, bson.D{{Key: "email", Value: email}})
	if err == nil {
		if u.IsAdmin() {
			return nil
		}
		cx, cancel := ctx()
		defer cancel()
		_, err = s.users.UpdateByID(cx, u.ID, bson.D{{Key: "$set", Value: bson.D{{Key: "role", Value: RoleAdmin}}}})
		return err
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	if name == "" {
		name = "Admin"
	}
	admin := &User{Name: name, Email: email, Password: password, Role: RoleAdmin}
	if err := s.Register(admin); err != nil && !errors.Is(err, ErrDuplicate) {
		return err // a duplicate means a concurrent start won the race — fine
	}
	return nil
}

func (s *Store) userByID(id string) (*User, error) {
	return findOne[User](s.users, bson.D{{Key: "_id", Value: id}})
}

// --- user administration ---

// HashPassword produces the stored form of a password. Exported so handlers
// can hash a replacement password during an update.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

// Users lists accounts, optionally filtered by name/email substring and role.
func (s *Store) Users(keyword, role string) ([]*User, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, bson.E{Key: "$or", Value: bson.A{
			bson.D{like("name", keyword)}, bson.D{like("email", keyword)},
		}})
	}
	if role != "" {
		f = append(f, bson.E{Key: "role", Value: role})
	}
	return findAll[User](s.users, f)
}

func (s *Store) User(id string) (*User, error) { return s.userByID(id) }

// UpdateUser expects u.Password to already hold a bcrypt hash.
func (s *Store) UpdateUser(u *User) error {
	if err := replace(s.users, u.ID, u); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

// ResetPassword sets a new password for the account with the given email.
// Admin accounts are refused (change those from the admin console).
//
// DEMO ONLY: this verifies the email exists but NOT that the caller owns it —
// there is no emailed reset token. Do not use as-is in production.
func (s *Store) ResetPassword(email, newPassword string) error {
	u, err := findOne[User](s.users, bson.D{{Key: "email", Value: email}})
	if err != nil {
		return err // ErrNotFound
	}
	if u.IsAdmin() {
		return ErrUnauthorized
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	cx, cancel := ctx()
	defer cancel()
	_, err = s.users.UpdateByID(cx, u.ID, bson.D{{Key: "$set", Value: bson.D{{Key: "password", Value: hash}}}})
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

// --- Bookings ---

func (s *Store) Book(userID, eventID string, qty int) (*Booking, error) {
	if qty < 1 {
		return nil, ErrSoldOut
	}
	if _, err := s.Event(eventID); err != nil {
		return nil, err // ErrNotFound
	}
	// Atomically reserve tickets only if enough remain and the event is on sale.
	cx, cancel := ctx()
	defer cancel()
	var e Event
	err := s.events.FindOneAndUpdate(cx,
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
		return nil, ErrSoldOut
	}
	if err != nil {
		return nil, err
	}
	b := &Booking{
		ID: newID(), UserID: userID, EventID: eventID, Quantity: qty,
		Total: e.PriceMin * float64(qty), Status: "confirmed", CreatedAt: time.Now(),
	}
	return b, insert(s.bookings, b)
}

func (s *Store) Booking(id, userID string) (*Booking, error) {
	return findOne[Booking](s.bookings, bson.D{{Key: "_id", Value: id}, {Key: "userId", Value: userID}})
}

func (s *Store) UserBookings(userID string) ([]*Booking, error) {
	return findAll[Booking](s.bookings, bson.D{{Key: "userId", Value: userID}})
}

func (s *Store) CancelBooking(id, userID string) (*Booking, error) {
	b, err := s.Booking(id, userID)
	if err != nil {
		return nil, err
	}
	return s.cancel(b)
}

// cancel marks a booking cancelled and returns its tickets to the event. It is
// a no-op on an already-cancelled booking, so retries stay safe.
func (s *Store) cancel(b *Booking) (*Booking, error) {
	if b.Status != "confirmed" {
		return b, nil
	}
	cx, cancel := ctx()
	defer cancel()
	if _, err := s.bookings.UpdateByID(cx, b.ID,
		bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "cancelled"}}}}); err != nil {
		return nil, err
	}
	if err := s.releaseTickets(cx, b); err != nil {
		return nil, err
	}
	b.Status = "cancelled"
	return b, nil
}

// releaseTickets hands a booking's seats back to its event.
func (s *Store) releaseTickets(cx context.Context, b *Booking) error {
	_, err := s.events.UpdateByID(cx, b.EventID,
		bson.D{{Key: "$inc", Value: bson.D{{Key: "ticketsSold", Value: -b.Quantity}}}})
	return err
}

// --- booking administration (no per-user scoping) ---

type BookingFilter struct{ UserID, EventID, Status string }

func (s *Store) AllBookings(f BookingFilter) ([]*Booking, error) {
	q := bson.D{}
	if f.UserID != "" {
		q = append(q, bson.E{Key: "userId", Value: f.UserID})
	}
	if f.EventID != "" {
		q = append(q, bson.E{Key: "eventId", Value: f.EventID})
	}
	if f.Status != "" {
		q = append(q, bson.E{Key: "status", Value: f.Status})
	}
	return findAll[Booking](s.bookings, q)
}

func (s *Store) BookingByID(id string) (*Booking, error) {
	return findOne[Booking](s.bookings, bson.D{{Key: "_id", Value: id}})
}

// AdminCancelBooking cancels any booking regardless of who owns it.
func (s *Store) AdminCancelBooking(id string) (*Booking, error) {
	b, err := s.BookingByID(id)
	if err != nil {
		return nil, err
	}
	return s.cancel(b)
}

// DeleteBooking erases a booking outright. A confirmed one releases its
// tickets first, so deleting never leaves an event overcounted.
func (s *Store) DeleteBooking(id string) error {
	b, err := s.BookingByID(id)
	if err != nil {
		return err
	}
	if b.Status == "confirmed" {
		cx, cancel := ctx()
		defer cancel()
		if err := s.releaseTickets(cx, b); err != nil {
			return err
		}
	}
	return remove(s.bookings, id)
}
