package main

import "time"

type Classification struct {
	ID      string `json:"id"`
	Segment string `json:"segment"` // e.g. Music, Sports, Arts & Theatre
	Genre   string `json:"genre"`   // e.g. Rock, Basketball
}

type Attraction struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"` // performer, team, ...
	ClassificationID string `json:"classificationId"`
}

type Venue struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	City     string `json:"city"`
	State    string `json:"state"`
	Country  string `json:"country"`
	Address  string `json:"address"`
	Capacity int    `json:"capacity"`
}

type Event struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Date             time.Time `json:"date"`
	VenueID          string    `json:"venueId"`
	AttractionIDs    []string  `json:"attractionIds"`
	ClassificationID string    `json:"classificationId"`
	PriceMin         float64   `json:"priceMin"`
	PriceMax         float64   `json:"priceMax"`
	TicketsTotal     int       `json:"ticketsTotal"`
	TicketsSold      int       `json:"ticketsSold"`
	Status           string    `json:"status"` // onsale, offsale, cancelled
}

func (e Event) Available() int { return e.TicketsTotal - e.TicketsSold }

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
}

type Booking struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	EventID   string    `json:"eventId"`
	Quantity  int       `json:"quantity"`
	Total     float64   `json:"total"`
	Status    string    `json:"status"` // confirmed, cancelled
	CreatedAt time.Time `json:"createdAt"`
}
