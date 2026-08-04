// Catalogue reads. These live in core because both tiers need them: the
// user package serves them to the public, and the admin package loads a
// record before updating it.
package core

import (
	"time"

	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Events returns one page of events matching f, plus the total number that
// matched. City is resolved through the venues collection first, since events
// store only a venueId — meaning a city filter costs one extra query.
func (s *Store) Events(f EventFilter, p Page) ([]*models.Event, int64, error) {
	q := bson.D{}
	if f.Keyword != "" {
		q = append(q, Like("name", f.Keyword))
	}
	if f.ClassificationID != "" {
		q = append(q, bson.E{Key: "classificationId", Value: f.ClassificationID})
	}
	if !f.StartAfter.IsZero() {
		q = append(q, bson.E{Key: "date", Value: bson.D{{Key: "$gte", Value: f.StartAfter}}})
	}
	if f.City != "" {
		ids, err := s.venueIDsInCity(f.City)
		if err != nil {
			return nil, 0, err
		}
		q = append(q, bson.E{Key: "venueId", Value: bson.D{{Key: "$in", Value: ids}}})
	}
	return FindPage[models.Event](s.EventsCol, q, p)
}

// venueIDsInCity collects every venue id in a city. This one deliberately is
// not paged — it feeds an $in clause, so a partial list would silently drop
// events. Only the _id is read back, so the documents themselves never load.
func (s *Store) venueIDsInCity(city string) ([]string, error) {
	cx, cancel := Ctx()
	defer cancel()
	cur, err := s.VenuesCol.Find(cx, bson.D{Like("city", city)},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID string `bson:"_id"`
	}
	if err := cur.All(cx, &rows); err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids, nil
}

// Event returns a single event, or ErrNotFound.
func (s *Store) Event(id string) (*models.Event, error) {
	return FindOne[models.Event](s.EventsCol, bson.D{{Key: "_id", Value: id}})
}

// Venues returns one page of venues narrowed by a case-insensitive substring
// match on name and/or city, plus the total number that matched. Empty
// arguments are skipped, so no filter means "all".
func (s *Store) Venues(keyword, city string, p Page) ([]*models.Venue, int64, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, Like("name", keyword))
	}
	if city != "" {
		f = append(f, Like("city", city))
	}
	return FindPage[models.Venue](s.VenuesCol, f, p)
}

// Venue returns a single venue, or ErrNotFound.
func (s *Store) Venue(id string) (*models.Venue, error) {
	return FindOne[models.Venue](s.VenuesCol, bson.D{{Key: "_id", Value: id}})
}

// Attractions returns one page of attractions whose name contains keyword,
// case-insensitively, plus the total number that matched. An empty keyword
// matches all of them.
func (s *Store) Attractions(keyword string, p Page) ([]*models.Attraction, int64, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, Like("name", keyword))
	}
	return FindPage[models.Attraction](s.AttractsCol, f, p)
}

// Attraction returns a single attraction, or ErrNotFound.
func (s *Store) Attraction(id string) (*models.Attraction, error) {
	return FindOne[models.Attraction](s.AttractsCol, bson.D{{Key: "_id", Value: id}})
}

// Classifications returns one page of classifications plus the total count.
// The set is small and fixed in practice (segments and genres), so there is no
// filter — but it is paged like everything else so the envelope stays uniform.
func (s *Store) Classifications(p Page) ([]*models.Classification, int64, error) {
	return FindPage[models.Classification](s.ClassesCol, bson.D{}, p)
}

// Classification returns a single classification, or ErrNotFound.
func (s *Store) Classification(id string) (*models.Classification, error) {
	return FindOne[models.Classification](s.ClassesCol, bson.D{{Key: "_id", Value: id}})
}

// EventFilter narrows an event search. Zero-valued fields are ignored, so the
// empty filter matches everything.
type EventFilter struct {
	Keyword, City, ClassificationID string
	StartAfter                      time.Time
}
