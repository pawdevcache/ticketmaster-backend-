package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// --- Classifications ---

// Classifications returns one page of classifications plus the total count.
// The set is small and fixed in practice (segments and genres), so there is no
// filter — but it is paged like everything else so the envelope stays uniform.
func (s *Store) Classifications(p Page) ([]*models.Classification, int64, error) {
	return findPage[models.Classification](s.classes, bson.D{}, p)
}

// Classification returns a single classification, or ErrNotFound.
func (s *Store) Classification(id string) (*models.Classification, error) {
	return findOne[models.Classification](s.classes, bson.D{{Key: "_id", Value: id}})
}

// CreateClassification assigns a fresh id to c and inserts it.
func (s *Store) CreateClassification(c *models.Classification) error {
	c.ID = newID()
	return insert(s.classes, c)
}

// UpdateClassification replaces the stored classification matching c.ID, or
// returns ErrNotFound.
func (s *Store) UpdateClassification(c *models.Classification) error {
	return replace(s.classes, c.ID, c)
}

// DeleteClassification refuses while events or attractions still classify
// themselves under it.
func (s *Store) DeleteClassification(id string) error {
	for _, c := range []*mongo.Collection{s.events, s.attracts} {
		used, err := anyMatch(c, bson.D{{Key: "classificationId", Value: id}})
		if err != nil {
			return err
		}
		if used {
			return ErrInUse
		}
	}
	return remove(s.classes, id)
}
