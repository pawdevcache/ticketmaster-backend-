package main

import (
	"log"
	"net/http"
)

func main() {
	loadEnv(".env")
	store, err := NewStore(env("MONGO_URI", "mongodb://localhost:27017"), env("DB_NAME", "ticketmaster"))
	if err != nil {
		log.Fatal("mongo connect: ", err)
	}
	s := &Server{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	// Discovery API (read + admin create)
	mux.HandleFunc("GET /discovery/v2/events", s.searchEvents)
	mux.HandleFunc("GET /discovery/v2/events/{id}", s.getEvent)
	mux.HandleFunc("POST /discovery/v2/events", s.createEvent)
	mux.HandleFunc("GET /discovery/v2/venues", s.searchVenues)
	mux.HandleFunc("GET /discovery/v2/venues/{id}", s.getVenue)
	mux.HandleFunc("POST /discovery/v2/venues", s.createVenue)
	mux.HandleFunc("GET /discovery/v2/attractions", s.searchAttractions)
	mux.HandleFunc("GET /discovery/v2/attractions/{id}", s.getAttraction)
	mux.HandleFunc("POST /discovery/v2/attractions", s.createAttraction)
	mux.HandleFunc("GET /discovery/v2/classifications", s.searchClassifications)
	mux.HandleFunc("GET /discovery/v2/classifications/{id}", s.getClassification)

	// Ticketing / commerce
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/bookings", s.createBooking)
	mux.HandleFunc("GET /api/bookings", s.listBookings)
	mux.HandleFunc("GET /api/bookings/{id}", s.getBooking)
	mux.HandleFunc("DELETE /api/bookings/{id}", s.cancelBooking)

	addr := ":" + env("PORT", "8080")
	log.Println("Ticketmaster API listening on", addr)
	log.Fatal(http.ListenAndServe(addr, cors(mux)))
}

// cors allows browser-based API clients (Postman web, Thunder Client, frontends)
// and answers preflight OPTIONS requests so POSTs with auth headers aren't blocked.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
