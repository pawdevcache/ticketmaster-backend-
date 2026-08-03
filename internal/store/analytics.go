package store

import (
	"ticketmaster/internal/models"

	"go.mongodb.org/mongo-driver/bson"
)

// Analytics bounds, applied by Analytics.
const (
	DefaultTopEvents = 10
	MaxTopEvents     = 50
)

// Analytics computes the admin dashboard figures.
//
// Both series are aggregated by MongoDB rather than assembled in Go. The
// alternative — read every booking and total it here — is the pattern that
// makes a dashboard slower every week the service runs, and it cannot work at
// all through the paged list endpoints.
//
// Only confirmed bookings count. A cancelled booking has had its money
// returned and its seats released, so counting it would overstate both
// revenue and demand.
func (s *Store) Analytics(topEvents int) (*models.Analytics, error) {
	if topEvents <= 0 {
		topEvents = DefaultTopEvents
	}
	if topEvents > MaxTopEvents {
		topEvents = MaxTopEvents
	}
	byEvent, err := s.revenueByEvent(topEvents)
	if err != nil {
		return nil, err
	}
	byCategory, err := s.ticketsByCategory()
	if err != nil {
		return nil, err
	}
	return &models.Analytics{RevenueByEvent: byEvent, TicketsByCategory: byCategory}, nil
}

// revenueByEvent totals confirmed revenue per event, highest first.
func (s *Store) revenueByEvent(limit int) ([]models.EventRevenue, error) {
	cx, cancel := ctx()
	defer cancel()
	cur, err := s.bookings.Aggregate(cx, []bson.D{
		{{Key: "$match", Value: bson.D{{Key: "status", Value: "confirmed"}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$eventId"},
			{Key: "revenue", Value: bson.D{{Key: "$sum", Value: "$total"}}},
			{Key: "tickets", Value: bson.D{{Key: "$sum", Value: "$quantity"}}},
			{Key: "bookings", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "revenue", Value: -1}}}},
		{{Key: "$limit", Value: limit}},
		// Left join: an event deleted after its bookings were cancelled can
		// still appear here, and must not vanish from the totals.
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "events"},
			{Key: "localField", Value: "_id"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "event"},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "eventId", Value: "$_id"},
			{Key: "revenue", Value: 1},
			{Key: "tickets", Value: 1},
			{Key: "bookings", Value: 1},
			{Key: "name", Value: bson.D{{Key: "$ifNull", Value: bson.A{
				bson.D{{Key: "$arrayElemAt", Value: bson.A{"$event.name", 0}}}, "(deleted event)",
			}}}},
		}}},
	})
	if err != nil {
		return nil, err
	}
	out := []models.EventRevenue{}
	return out, cur.All(cx, &out)
}

// ticketsByCategory totals confirmed tickets per classification. Bookings
// reach a category through their event, so this joins twice.
func (s *Store) ticketsByCategory() ([]models.CategoryTickets, error) {
	cx, cancel := ctx()
	defer cancel()
	cur, err := s.bookings.Aggregate(cx, []bson.D{
		{{Key: "$match", Value: bson.D{{Key: "status", Value: "confirmed"}}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "events"},
			{Key: "localField", Value: "eventId"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "event"},
		}}},
		// Keep bookings whose event is gone; they group under "Uncategorised"
		// rather than disappearing from the ticket count.
		{{Key: "$unwind", Value: bson.D{
			{Key: "path", Value: "$event"},
			{Key: "preserveNullAndEmptyArrays", Value: true},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$event.classificationId", ""}}}},
			{Key: "tickets", Value: bson.D{{Key: "$sum", Value: "$quantity"}}},
			{Key: "revenue", Value: bson.D{{Key: "$sum", Value: "$total"}}},
		}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "classifications"},
			{Key: "localField", Value: "_id"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "classification"},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "classificationId", Value: "$_id"},
			{Key: "tickets", Value: 1},
			{Key: "revenue", Value: 1},
			{Key: "segment", Value: bson.D{{Key: "$ifNull", Value: bson.A{
				bson.D{{Key: "$arrayElemAt", Value: bson.A{"$classification.segment", 0}}}, "Uncategorised",
			}}}},
			{Key: "genre", Value: bson.D{{Key: "$ifNull", Value: bson.A{
				bson.D{{Key: "$arrayElemAt", Value: bson.A{"$classification.genre", 0}}}, "",
			}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "tickets", Value: -1}}}},
	})
	if err != nil {
		return nil, err
	}
	out := []models.CategoryTickets{}
	return out, cur.All(cx, &out)
}
