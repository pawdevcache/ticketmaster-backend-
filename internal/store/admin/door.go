package admin

import (
	"ticketmaster/internal/models"
	adminmodels "ticketmaster/internal/models/admin"
	"ticketmaster/internal/store/core"

	"go.mongodb.org/mongo-driver/bson"
)

// Door reports sold-versus-admitted per event, busiest first. Pass an empty
// eventID for every event with sales.
//
// Only confirmed bookings count: an unpaid hold has sold nothing, and a
// cancelled one gave its seats back.
func (s *Store) Door(eventID string) ([]adminmodels.DoorStats, error) {
	cx, cancel := core.Ctx()
	defer cancel()
	match := bson.D{{Key: "status", Value: models.BookingConfirmed}}
	if eventID != "" {
		match = append(match, bson.E{Key: "eventId", Value: eventID})
	}
	cur, err := s.BookingsCol.Aggregate(cx, []bson.D{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$eventId"},
			{Key: "sold", Value: bson.D{{Key: "$sum", Value: "$quantity"}}},
			// checkedInAt is absent until a ticket is scanned, so its presence
			// is the admitted flag.
			{Key: "admitted", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$ifNull", Value: bson.A{"$checkedInAt", false}}}, "$quantity", 0,
			}}}}}},
		}}},
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "events"}, {Key: "localField", Value: "_id"},
			{Key: "foreignField", Value: "_id"}, {Key: "as", Value: "event"},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "eventId", Value: "$_id"},
			{Key: "sold", Value: 1}, {Key: "admitted", Value: 1},
			{Key: "name", Value: bson.D{{Key: "$ifNull", Value: bson.A{
				bson.D{{Key: "$arrayElemAt", Value: bson.A{"$event.name", 0}}}, "(deleted event)"}}}},
			{Key: "capacity", Value: bson.D{{Key: "$ifNull", Value: bson.A{
				bson.D{{Key: "$arrayElemAt", Value: bson.A{"$event.ticketsTotal", 0}}}, 0}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "sold", Value: -1}}}},
	})
	if err != nil {
		return nil, err
	}
	out := []adminmodels.DoorStats{}
	return out, cur.All(cx, &out)
}
