// Package admin holds the store operations that require an administrator:
// catalogue writes, account management, every user's bookings, and the
// dashboard aggregations.
//
// Nothing here enforces that the caller IS an admin — authorisation lives in
// the HTTP layer. This package marks intent, not permission.
package admin

import (
	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// CreateEvent assigns a fresh id to e and inserts it, defaulting an unset
// status to "onsale" so a new event is immediately bookable.
func (s *Store) CreateEvent(e *models.Event) error {
	e.ID = core.NewID()
	if e.Status == "" {
		e.Status = "onsale"
	}
	return core.Insert(s.EventsCol, e)
}

// UpdateEvent replaces the stored event matching e.ID, or returns core.ErrNotFound.
// It writes e verbatim: guarding derived fields such as ticketsSold is the
// caller's job.
func (s *Store) UpdateEvent(e *models.Event) error {
	return core.Replace(s.EventsCol, e.ID, e)
}

// DeleteEvent refuses while confirmed bookings exist — cancel them first, so
// nobody loses a ticket they paid for. Cancelled bookings don't block it.
func (s *Store) DeleteEvent(id string) error {
	booked, err := core.AnyMatch(s.BookingsCol, bson.D{
		{Key: "eventId", Value: id}, {Key: "status", Value: "confirmed"},
	})
	if err != nil {
		return err
	}
	if booked {
		return core.ErrInUse
	}
	return core.Remove(s.EventsCol, id)
}

// CreateVenue assigns a fresh id to v and inserts it.
func (s *Store) CreateVenue(v *models.Venue) error {
	v.ID = core.NewID()
	return core.Insert(s.VenuesCol, v)
}

// UpdateVenue replaces the stored venue matching v.ID, or returns core.ErrNotFound.
func (s *Store) UpdateVenue(v *models.Venue) error {
	return core.Replace(s.VenuesCol, v.ID, v)
}

// DeleteVenue refuses while any event is still scheduled there.
func (s *Store) DeleteVenue(id string) error {
	used, err := core.AnyMatch(s.EventsCol, bson.D{{Key: "venueId", Value: id}})
	if err != nil {
		return err
	}
	if used {
		return core.ErrInUse
	}
	return core.Remove(s.VenuesCol, id)
}

// CreateAttraction assigns a fresh id to a and inserts it.
func (s *Store) CreateAttraction(a *models.Attraction) error {
	a.ID = core.NewID()
	return core.Insert(s.AttractsCol, a)
}

// UpdateAttraction replaces the stored attraction matching a.ID, or returns
// core.ErrNotFound.
func (s *Store) UpdateAttraction(a *models.Attraction) error {
	return core.Replace(s.AttractsCol, a.ID, a)
}

// DeleteAttraction refuses while any event still lists it in attractionIds.
func (s *Store) DeleteAttraction(id string) error {
	used, err := core.AnyMatch(s.EventsCol, bson.D{{Key: "attractionIds", Value: id}})
	if err != nil {
		return err
	}
	if used {
		return core.ErrInUse
	}
	return core.Remove(s.AttractsCol, id)
}

// CreateClassification assigns a fresh id to c and inserts it.
func (s *Store) CreateClassification(c *models.Classification) error {
	c.ID = core.NewID()
	return core.Insert(s.ClassesCol, c)
}

// UpdateClassification replaces the stored classification matching c.ID, or
// returns core.ErrNotFound.
func (s *Store) UpdateClassification(c *models.Classification) error {
	return core.Replace(s.ClassesCol, c.ID, c)
}

// DeleteClassification refuses while events or attractions still classify
// themselves under it.
func (s *Store) DeleteClassification(id string) error {
	for _, c := range []*mongo.Collection{s.EventsCol, s.AttractsCol} {
		used, err := core.AnyMatch(c, bson.D{{Key: "classificationId", Value: id}})
		if err != nil {
			return err
		}
		if used {
			return core.ErrInUse
		}
	}
	return core.Remove(s.ClassesCol, id)
}
