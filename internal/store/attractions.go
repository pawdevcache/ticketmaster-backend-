package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// --- Attractions ---

func (s *Store) Attractions(keyword string) ([]*models.Attraction, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, like("name", keyword))
	}
	return findAll[models.Attraction](s.attracts, f)
}
func (s *Store) Attraction(id string) (*models.Attraction, error) {
	return findOne[models.Attraction](s.attracts, bson.D{{Key: "_id", Value: id}})
}
func (s *Store) CreateAttraction(a *models.Attraction) error {
	a.ID = newID()
	return insert(s.attracts, a)
}
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
