package tm

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrSoldOut      = errors.New("not enough tickets available")
	ErrUnauthorized = errors.New("unauthorized")
)

// Store is a MongoDB-backed datastore.
type Store struct {
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(dbName)
	return &Store{
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

// --- Classifications ---

func (s *Store) Classifications() ([]*Classification, error) {
	return findAll[Classification](s.classes, bson.D{})
}
func (s *Store) Classification(id string) (*Classification, error) {
	return findOne[Classification](s.classes, bson.D{{Key: "_id", Value: id}})
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

// --- Users & auth ---

func (s *Store) Register(u *User) error {
	u.ID = newID()
	return insert(s.users, u)
}

func (s *Store) Login(email, password string) (string, error) {
	u, err := findOne[User](s.users, bson.D{{Key: "email", Value: email}, {Key: "password", Value: password}})
	if err != nil {
		return "", ErrUnauthorized
	}
	tok := newID()
	return tok, insert(s.tokens, bson.D{{Key: "_id", Value: tok}, {Key: "userId", Value: u.ID}})
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

func (s *Store) userByID(id string) (*User, error) {
	return findOne[User](s.users, bson.D{{Key: "_id", Value: id}})
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
	if b.Status == "confirmed" {
		cx, cancel := ctx()
		defer cancel()
		s.bookings.UpdateByID(cx, id, bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "cancelled"}}}})
		s.events.UpdateByID(cx, b.EventID, bson.D{{Key: "$inc", Value: bson.D{{Key: "ticketsSold", Value: -b.Quantity}}}})
		b.Status = "cancelled"
	}
	return b, nil
}
