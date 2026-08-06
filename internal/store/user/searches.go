package user

import (
	"strings"
	"time"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// maxHistory is how many terms a user's history keeps. A dropdown shows a
// handful; beyond that it is clutter nobody scrolls.
const maxHistory = 20

// RecordSearch remembers a term for a user.
//
// Upsert on (userId, term): searching the same thing twice moves it back to
// the top rather than filling the list with the same word. Blank terms are
// ignored and long ones are truncated, so a stray keystroke or a pasted essay
// cannot bloat the history.
func (s *Store) RecordSearch(userID, term string) error {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	if r := []rune(term); len(r) > 128 {
		term = string(r[:128])
	}
	cx, cancel := core.Ctx()
	defer cancel()
	_, err := s.SearchesCol.UpdateOne(cx,
		bson.D{{Key: "userId", Value: userID}, {Key: "term", Value: term}},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "searchedAt", Value: time.Now()}}},
			{Key: "$setOnInsert", Value: bson.D{{Key: "_id", Value: core.NewID()}}},
		},
		options.Update().SetUpsert(true))
	return err
}

// Searches returns a user's recent terms, most recent first.
func (s *Store) Searches(userID string) ([]*models.Search, error) {
	cx, cancel := core.Ctx()
	defer cancel()
	cur, err := s.SearchesCol.Find(cx,
		bson.D{{Key: "userId", Value: userID}},
		options.Find().SetSort(bson.D{{Key: "searchedAt", Value: -1}}).SetLimit(maxHistory))
	if err != nil {
		return nil, err
	}
	out := []*models.Search{}
	return out, cur.All(cx, &out)
}

// ForgetSearch removes one term. The user id is part of the filter, so one
// user can never delete another's history.
func (s *Store) ForgetSearch(userID, id string) error {
	cx, cancel := core.Ctx()
	defer cancel()
	_, err := s.SearchesCol.DeleteOne(cx,
		bson.D{{Key: "_id", Value: id}, {Key: "userId", Value: userID}})
	return err
}

// ClearSearches empties a user's history.
func (s *Store) ClearSearches(userID string) error {
	cx, cancel := core.Ctx()
	defer cancel()
	_, err := s.SearchesCol.DeleteMany(cx, bson.D{{Key: "userId", Value: userID}})
	return err
}
