// Package models holds the domain types shared by the storage and HTTP layers.
//
// Each type carries both json and bson tags, so the same struct is the wire
// format and the database document. That keeps the API and the collections in
// step, at the cost of one rule: a field the client must not set (an id, or a
// derived count such as Event.TicketsSold) has to be restored by the handler
// after decoding a request, because decoding alone will happily overwrite it.
package models

import "time"

// Classification is the genre taxonomy an event or attraction is filed under.
type Classification struct {
	ID      string `json:"id" bson:"_id"`
	Segment string `json:"segment" bson:"segment"` // e.g. Music, Sports, Arts & Theatre
	Genre   string `json:"genre" bson:"genre"`     // e.g. Rock, Basketball
}

// Attraction is a performer, team or other act that appears at events.
type Attraction struct {
	ID               string `json:"id" bson:"_id"`
	Name             string `json:"name" bson:"name"`
	Type             string `json:"type" bson:"type"` // performer, team, ...
	ClassificationID string `json:"classificationId" bson:"classificationId"`
}

// Venue is a physical location that hosts events.
type Venue struct {
	ID       string `json:"id" bson:"_id"`
	Name     string `json:"name" bson:"name"`
	City     string `json:"city" bson:"city"`
	State    string `json:"state" bson:"state"`
	Country  string `json:"country" bson:"country"`
	Address  string `json:"address" bson:"address"`
	Capacity int    `json:"capacity" bson:"capacity"`
}

// Event is a single dated performance at a venue, and the thing tickets are
// sold against. TicketsSold is owned by the booking flow — it is adjusted when
// bookings are made and cancelled, never written from a request body.
type Event struct {
	ID               string   `json:"id" bson:"_id"`
	Name             string   `json:"name" bson:"name"`
	Date             Date     `json:"date" bson:"date"`
	VenueID          string   `json:"venueId" bson:"venueId"`
	AttractionIDs    []string `json:"attractionIds" bson:"attractionIds"`
	ClassificationID string   `json:"classificationId" bson:"classificationId"`
	PriceMin         float64  `json:"priceMin" bson:"priceMin"`
	PriceMax         float64  `json:"priceMax" bson:"priceMax"`
	TicketsTotal     int      `json:"ticketsTotal" bson:"ticketsTotal"`
	TicketsSold      int      `json:"ticketsSold" bson:"ticketsSold"`
	Status           string   `json:"status" bson:"status"` // onsale, offsale, cancelled
	Description      string   `json:"description"`
	Title            string   `json:"title"`
}

// Roles. Anything that isn't RoleAdmin is treated as an ordinary user, so
// documents written before roles existed keep working.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// User is an account. Password holds a bcrypt hash once stored, never the
// plaintext; the json tag omits it when empty so handlers can blank the field
// to keep the hash out of a response.
type User struct {
	ID    string `json:"id" bson:"_id"`
	Name  string `json:"name" bson:"name"`
	Email string `json:"email" bson:"email"`
	// Role is set by the server from the endpoint used to register, never
	// from the request body — otherwise anyone could self-promote.
	Role     string `json:"role" bson:"role"`
	Password string `json:"password,omitempty" bson:"password"`
}

// IsAdmin reports whether the account may use the admin endpoints.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// Booking is a user's claim on some of an event's tickets. Cancelling sets
// Status rather than deleting the row, so the history survives.
type Booking struct {
	ID        string    `json:"id" bson:"_id"`
	UserID    string    `json:"userId" bson:"userId"`
	EventID   string    `json:"eventId" bson:"eventId"`
	Quantity  int       `json:"quantity" bson:"quantity"`
	Total     float64   `json:"total" bson:"total"`
	Status    string    `json:"status" bson:"status"` // confirmed, cancelled
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`

	// Event is filled in when a booking is read, so a client can render a
	// ticket without fetching each event separately. bson:"-" keeps it out of
	// the stored document — it is a view of the events collection, not a copy,
	// so an edited event shows through rather than going stale.
	Event *EventSummary `json:"event,omitempty" bson:"-"`
}

// EventSummary is the slice of an event a ticket needs: what it is, when, and
// where. Enough to print a ticket and build a calendar entry.
type EventSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Date      Date   `json:"date"`
	Status    string `json:"status"`
	VenueID   string `json:"venueId,omitempty"`
	VenueName string `json:"venueName,omitempty"`
	VenueCity string `json:"venueCity,omitempty"`
}
