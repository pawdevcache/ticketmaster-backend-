// Package admin holds the response shapes that only the administrative
// endpoints produce. They are dashboard projections, not domain records: no
// collection stores an Analytics, it is computed per request.

package admin

// Analytics is the admin dashboard payload: one request feeding both charts.
type Analytics struct {
	RevenueByEvent    []EventRevenue    `json:"revenueByEvent"`
	TicketsByCategory []CategoryTickets `json:"ticketsByCategory"`
}

// EventRevenue is one bar of the "revenue by event" chart. Figures cover
// confirmed bookings only.
type EventRevenue struct {
	EventID  string  `json:"eventId" bson:"eventId"`
	Name     string  `json:"name" bson:"name"`
	Revenue  float64 `json:"revenue" bson:"revenue"`
	Tickets  int     `json:"tickets" bson:"tickets"`
	Bookings int     `json:"bookings" bson:"bookings"`
}

// CategoryTickets is one bar of the "tickets sold by category" chart. Bookings
// whose event or classification is missing group under "Uncategorised" rather
// than being dropped, so the bars always sum to the tickets actually sold.
type CategoryTickets struct {
	ClassificationID string  `json:"classificationId" bson:"classificationId"`
	Segment          string  `json:"segment" bson:"segment"`
	Genre            string  `json:"genre" bson:"genre"`
	Tickets          int     `json:"tickets" bson:"tickets"`
	Revenue          float64 `json:"revenue" bson:"revenue"`
}
