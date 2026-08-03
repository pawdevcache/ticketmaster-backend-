// Package store is the MongoDB persistence layer: every database read and
// write in the service goes through it.
//
// It knows nothing about HTTP. Failures come back as the sentinel errors
// declared here — ErrNotFound, ErrUnauthorized, ErrDuplicate, ErrSoldOut,
// ErrInUse — and the HTTP layer maps those to status codes, so changing a
// status code never means editing this package.
//
// Two rules are enforced here rather than left to callers, because getting
// them wrong corrupts data: ticket counts move only through Book and the
// cancel path, and a record still referenced by another record cannot be
// deleted (ErrInUse).
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
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
	ErrDuplicate    = errors.New("email already registered")
	// ErrInUse blocks deleting a record other records still point at, so an
	// admin can't silently orphan events, bookings or attractions.
	ErrInUse = errors.New("still referenced by other records")
)

// tokenTTL is how long a login token stays valid before Mongo's TTL index
// deletes it (enforced by EnsureIndexes).
const tokenTTL = 24 * time.Hour

// ResetTTL is how long a password-reset token stays usable. Short on purpose:
// a reset token is a temporary key to somebody's account.
const ResetTTL = time.Hour

// MinPasswordLen is the shortest password accepted when setting a new one.
const MinPasswordLen = 6

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
	resets   *mongo.Collection
}

// NewStore prepares a store against the given MongoDB URI and database. It
// does not contact the server: see the comment inside on why startup must
// survive an unreachable database, and use Ping to test connectivity.
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
		resets:   db.Collection("passwordResets"),
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

// newSecret returns an unguessable 256-bit token. Reset tokens must not be
// predictable, so this uses crypto/rand rather than the ObjectID scheme that
// newID uses for document ids (ObjectIDs embed a timestamp and a counter).
func newSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// maxSearchLen bounds a search term. Escaping stops a pattern from
// backtracking, but a megabyte-long literal still makes Mongo compare a
// megabyte against every document in the collection.
const maxSearchLen = 128

// like builds a case-insensitive "contains" filter on a single field.
//
// The value is escaped before it becomes a regex. It arrives straight from a
// query string on endpoints that need no authentication, so an unescaped
// pattern such as "(a+)+$" would make the server backtrack for an unbounded
// time — a denial of service anyone could trigger with one request. Escaping
// makes the term a literal substring, which is the only thing a search box
// ever meant.
func like(field, value string) bson.E {
	// Slice runes rather than bytes: cutting a multi-byte character in half
	// would put invalid UTF-8 into the pattern.
	if r := []rune(value); len(r) > maxSearchLen {
		value = string(r[:maxSearchLen])
	}
	return bson.E{Key: field, Value: primitive.Regex{Pattern: regexp.QuoteMeta(value), Options: "i"}}
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
