package tm

import "time"

type Classification struct {
	ID      string `json:"id" bson:"_id"`
	Segment string `json:"segment" bson:"segment"` // e.g. Music, Sports, Arts & Theatre
	Genre   string `json:"genre" bson:"genre"`     // e.g. Rock, Basketball
}

type Attraction struct {
	ID               string `json:"id" bson:"_id"`
	Name             string `json:"name" bson:"name"`
	Type             string `json:"type" bson:"type"` // performer, team, ...
	ClassificationID string `json:"classificationId" bson:"classificationId"`
}

type Venue struct {
	ID       string `json:"id" bson:"_id"`
	Name     string `json:"name" bson:"name"`
	City     string `json:"city" bson:"city"`
	State    string `json:"state" bson:"state"`
	Country  string `json:"country" bson:"country"`
	Address  string `json:"address" bson:"address"`
	Capacity int    `json:"capacity" bson:"capacity"`
}

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

type User struct {
	ID    string `json:"id" bson:"_id"`
	Name  string `json:"name" bson:"name"`
	Email string `json:"email" bson:"email"`
	// Role is set by the server from the endpoint used to register, never
	// from the request body — otherwise anyone could self-promote.
	Role     string `json:"role" bson:"role"`
	Password string `json:"password,omitempty" bson:"password"`
}

func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

type Booking struct {
	ID        string    `json:"id" bson:"_id"`
	UserID    string    `json:"userId" bson:"userId"`
	EventID   string    `json:"eventId" bson:"eventId"`
	Quantity  int       `json:"quantity" bson:"quantity"`
	Total     float64   `json:"total" bson:"total"`
	Status    string    `json:"status" bson:"status"` // confirmed, cancelled
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
}
