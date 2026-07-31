package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// --- Attractions ---

// Attractions lists attractions whose name contains keyword, case-insensitively.
// An empty keyword returns all of them.
func (s *Store) Attractions(keyword string) ([]*models.Attraction, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, like("name", keyword))
	}
	return findAll[models.Attraction](s.attracts, f)
}

// Attraction returns a single attraction, or ErrNotFound.
func (s *Store) Attraction(id string) (*models.Attraction, error) {
	return findOne[models.Attraction](s.attracts, bson.D{{Key: "_id", Value: id}})
}

// CreateAttraction assigns a fresh id to a and inserts it.
func (s *Store) CreateAttraction(a *models.Attraction) error {
	a.ID = newID()
	return insert(s.attracts, a)
}

// UpdateAttraction replaces the stored attraction matching a.ID, or returns
// ErrNotFound.
func (s *Store) UpdateAttraction(a *models.Attraction) error {
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
