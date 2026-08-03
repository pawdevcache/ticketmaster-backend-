package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// --- Venues ---

// Venues returns one page of venues narrowed by a case-insensitive substring
// match on name and/or city, plus the total number that matched. Empty
// arguments are skipped, so no filter means "all".
func (s *Store) Venues(keyword, city string, p Page) ([]*models.Venue, int64, error) {
	f := bson.D{}
	if keyword != "" {
		f = append(f, like("name", keyword))
	}
	if city != "" {
		f = append(f, like("city", city))
	}
	return findPage[models.Venue](s.venues, f, p)
}

// Venue returns a single venue, or ErrNotFound.
func (s *Store) Venue(id string) (*models.Venue, error) {
	return findOne[models.Venue](s.venues, bson.D{{Key: "_id", Value: id}})
}

// CreateVenue assigns a fresh id to v and inserts it.
func (s *Store) CreateVenue(v *models.Venue) error {
	v.ID = newID()
	return insert(s.venues, v)
}

// UpdateVenue replaces the stored venue matching v.ID, or returns ErrNotFound.
func (s *Store) UpdateVenue(v *models.Venue) error {
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
