package main

import "time"

// Seed loads demo data so the API is useful on first run.
func (s *Store) Seed() {
	music := &Classification{ID: "C001", Segment: "Music", Genre: "Rock"}
	sport := &Classification{ID: "C002", Segment: "Sports", Genre: "Basketball"}
	s.classes[music.ID], s.classes[sport.ID] = music, sport

	coldplay := &Attraction{ID: "K001", Name: "Coldplay", Type: "performer", ClassificationID: music.ID}
	lakers := &Attraction{ID: "K002", Name: "LA Lakers", Type: "team", ClassificationID: sport.ID}
	s.attracts[coldplay.ID], s.attracts[lakers.ID] = coldplay, lakers

	arena := &Venue{ID: "V001", Name: "O2 Arena", City: "London", Country: "GB", Address: "Peninsula Sq", Capacity: 20000}
	garden := &Venue{ID: "V002", Name: "Crypto.com Arena", City: "Los Angeles", State: "CA", Country: "US", Capacity: 19000}
	s.venues[arena.ID], s.venues[garden.ID] = arena, garden

	s.events["E001"] = &Event{
		ID: "E001", Name: "Coldplay: Music of the Spheres", Date: time.Date(2026, 8, 15, 19, 30, 0, 0, time.UTC),
		VenueID: arena.ID, AttractionIDs: []string{coldplay.ID}, ClassificationID: music.ID,
		PriceMin: 75, PriceMax: 250, TicketsTotal: 20000, Status: "onsale",
	}
	s.events["E002"] = &Event{
		ID: "E002", Name: "Lakers vs Celtics", Date: time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC),
		VenueID: garden.ID, AttractionIDs: []string{lakers.ID}, ClassificationID: sport.ID,
		PriceMin: 50, PriceMax: 500, TicketsTotal: 19000, Status: "onsale",
	}
	s.seq = 2
}
