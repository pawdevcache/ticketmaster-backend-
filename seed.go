package main

import (
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// Seed inserts demo data only when the events collection is empty, so it is
// safe to run on every startup. Enable with SEED=true. Nothing is hardcoded
// in the running store — this just bootstraps an empty database.
func (s *Store) Seed() error {
	cx, cancel := ctx()
	defer cancel()
	if n, err := s.events.CountDocuments(cx, map[string]any{}); err != nil || n > 0 {
		return err
	}

	classes := []any{
		Classification{ID: "C001", Segment: "Music", Genre: "Rock"},
		Classification{ID: "C002", Segment: "Sports", Genre: "Basketball"},
	}
	venues := []any{
		Venue{ID: "V001", Name: "O2 Arena", City: "London", Country: "GB", Address: "Peninsula Sq", Capacity: 20000},
		Venue{ID: "V002", Name: "Crypto.com Arena", City: "Los Angeles", State: "CA", Country: "US", Capacity: 19000},
	}
	attractions := []any{
		Attraction{ID: "K001", Name: "Coldplay", Type: "performer", ClassificationID: "C001"},
		Attraction{ID: "K002", Name: "LA Lakers", Type: "team", ClassificationID: "C002"},
	}
	events := []any{
		Event{ID: "E001", Name: "Coldplay: Music of the Spheres", Date: time.Date(2026, 8, 15, 19, 30, 0, 0, time.UTC),
			VenueID: "V001", AttractionIDs: []string{"K001"}, ClassificationID: "C001",
			PriceMin: 75, PriceMax: 250, TicketsTotal: 20000, Status: "onsale"},
		Event{ID: "E002", Name: "Lakers vs Celtics", Date: time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC),
			VenueID: "V002", AttractionIDs: []string{"K002"}, ClassificationID: "C002",
			PriceMin: 50, PriceMax: 500, TicketsTotal: 19000, Status: "onsale"},
	}

	for coll, docs := range map[*mongo.Collection][]any{
		s.classes: classes, s.venues: venues, s.attracts: attractions, s.events: events,
	} {
		if _, err := coll.InsertMany(cx, docs); err != nil {
			return err
		}
	}
	return nil
}
