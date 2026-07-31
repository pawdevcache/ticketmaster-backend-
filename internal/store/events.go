package store

import (
	"time"

	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// --- Events ---

type EventFilter struct {
	Keyword, City, ClassificationID string
	StartAfter                      time.Time
}

func (s *Store) Events(f EventFilter) ([]*models.Event, error) {
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
	return findAll[models.Event](s.events, q)
}
func (s *Store) Event(id string) (*models.Event, error) {
	return findOne[models.Event](s.events, bson.D{{Key: "_id", Value: id}})
}
func (s *Store) CreateEvent(e *models.Event) error {
	e.ID = newID()
	if e.Status == "" {
		e.Status = "onsale"
	}
	return insert(s.events, e)
}
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
