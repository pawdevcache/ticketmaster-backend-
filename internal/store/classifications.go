package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// --- Classifications ---

func (s *Store) Classifications() ([]*models.Classification, error) {
	return findAll[models.Classification](s.classes, bson.D{})
}
func (s *Store) Classification(id string) (*models.Classification, error) {
	return findOne[models.Classification](s.classes, bson.D{{Key: "_id", Value: id}})
}
func (s *Store) CreateClassification(c *models.Classification) error {
	c.ID = newID()
	return insert(s.classes, c)
}
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
