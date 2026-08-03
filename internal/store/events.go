package store

import (
	"time"

	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- Events ---

// EventFilter narrows an event search. Zero-valued fields are ignored, so the
// empty filter matches everything.
type EventFilter struct {
	Keyword, City, ClassificationID string
	StartAfter                      time.Time
}

// Events returns one page of events matching f, plus the total number that
// matched. City is resolved through the venues collection first, since events
// store only a venueId — meaning a city filter costs one extra query.
func (s *Store) Events(f EventFilter, p Page) ([]*models.Event, int64, error) {
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
		ids, err := s.venueIDsInCity(f.City)
		if err != nil {
			return nil, 0, err
		}
		q = append(q, bson.E{Key: "venueId", Value: bson.D{{Key: "$in", Value: ids}}})
	}
	return findPage[models.Event](s.events, q, p)
}

// venueIDsInCity collects every venue id in a city. This one deliberately is
// not paged — it feeds an $in clause, so a partial list would silently drop
// events. Only the _id is read back, so the documents themselves never load.
func (s *Store) venueIDsInCity(city string) ([]string, error) {
	cx, cancel := ctx()
	defer cancel()
	cur, err := s.venues.Find(cx, bson.D{like("city", city)},
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
	return findOne[models.Event](s.events, bson.D{{Key: "_id", Value: id}})
}

// CreateEvent assigns a fresh id to e and inserts it, defaulting an unset
// status to "onsale" so a new event is immediately bookable.
func (s *Store) CreateEvent(e *models.Event) error {
	e.ID = newID()
	if e.Status == "" {
		e.Status = "onsale"
	}
	return insert(s.events, e)
}

// UpdateEvent replaces the stored event matching e.ID, or returns ErrNotFound.
// It writes e verbatim: guarding derived fields such as ticketsSold is the
// caller's job.
func (s *Store) UpdateEvent(e *models.Event) error {
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
