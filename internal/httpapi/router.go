package tm

import (
	"log"
	"net/http"
)

// New builds the fully-wired API handler. Used by both the local dev server
// (main.go) and the Vercel serverless entrypoint (api/index.go).
func New() (http.Handler, error) {
	loadEnv(".env") // no-op when the file is absent (e.g. on Vercel)
	store, err := NewStore(env("MONGO_URI", "mongodb://localhost:27017"), env("DB_NAME", "ticketmaster"))
	if err != nil {
		return nil, err
	}
	// Best-effort: enforce unique email + token expiry. Don't fail startup if
	// the DB is briefly unreachable — health will surface that separately.
	if err := store.EnsureIndexes(); err != nil {
		log.Println("warning: could not create indexes:", err)
	}
	// Same deal for the optional bootstrap admin: log and carry on.
	if err := store.EnsureAdmin(env("ADMIN_NAME", "Admin"), env("ADMIN_EMAIL", ""), env("ADMIN_PASSWORD", "")); err != nil {
		log.Println("warning: could not seed admin user:", err)
	}
	s := &Server{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		if err := store.Ping(); err != nil {
			writeJSON(w, 200, map[string]string{"status": "degraded", "db": "disconnected", "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok", "db": "connected"})
	})

	// Root: {$} matches only the exact "/" path, so real unknown routes still 404.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"service": "Ticketmaster API",
			"status":  "running",
			"endpoints": map[string][]string{
				"public": {
					"GET /health",
					"GET /discovery/v2/{events|venues|attractions|classifications}",
					"GET /discovery/v2/{events|venues|attractions|classifications}/{id}",
					"POST /api/register", "POST /api/login",
					"POST /api/forgot-password", "POST /api/reset-password",
				},
				"user": {
					"POST /api/bookings", "GET /api/bookings",
					"GET /api/bookings/{id}", "DELETE /api/bookings/{id}",
				},
				"admin": {
					"POST /api/admin/register", "POST /api/admin/login", "GET /api/admin/me",
					"POST /discovery/v2/{events|venues|attractions|classifications}",
					"PUT|PATCH /discovery/v2/{events|venues|attractions|classifications}/{id}",
					"DELETE /discovery/v2/{events|venues|attractions|classifications}/{id}",
					"GET /api/admin/users", "GET /api/admin/users/{id}",
					"PUT|PATCH /api/admin/users/{id}", "DELETE /api/admin/users/{id}",
					"GET /api/admin/bookings", "GET /api/admin/bookings/{id}",
					"POST /api/admin/bookings/{id}/cancel", "DELETE /api/admin/bookings/{id}",
				},
			},
		})
	})

	// Discovery API. Reads are public; every write requires an admin token.
	// PUT and PATCH share a handler: both apply a partial update, so a body
	// only needs the fields that actually change.
	for _, res := range []struct {
		path                             string
		search, get, create, update, del http.HandlerFunc
	}{
		{"events", s.searchEvents, s.getEvent, s.createEvent, s.updateEvent, s.deleteEvent},
		{"venues", s.searchVenues, s.getVenue, s.createVenue, s.updateVenue, s.deleteVenue},
		{"attractions", s.searchAttractions, s.getAttraction, s.createAttraction, s.updateAttraction, s.deleteAttraction},
		{"classifications", s.searchClassifications, s.getClassification, s.createClassification, s.updateClassification, s.deleteClassification},
	} {
		base := "/discovery/v2/" + res.path
		mux.HandleFunc("GET "+base, res.search)
		mux.HandleFunc("GET "+base+"/{id}", res.get)
		mux.HandleFunc("POST "+base, res.create)
		mux.HandleFunc("PUT "+base+"/{id}", res.update)
		mux.HandleFunc("PATCH "+base+"/{id}", res.update)
		mux.HandleFunc("DELETE "+base+"/{id}", res.del)
	}

	// Ticketing / commerce
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("POST /api/login", s.login)
	// Forgotten-password flow: request a token, then trade it for a new password.
	mux.HandleFunc("POST /api/forgot-password", s.forgotPassword)
	mux.HandleFunc("POST /api/reset-password", s.resetPassword)

	// Admin accounts. Registration needs ADMIN_REGISTRATION_KEY; the resulting token
	// is what the POST /discovery/v2/* create routes require.
	mux.HandleFunc("POST /api/admin/register", s.adminRegister)
	mux.HandleFunc("POST /api/admin/login", s.adminLogin)
	mux.HandleFunc("GET /api/admin/me", s.adminMe)

	// Admin management of accounts and of every user's bookings.
	mux.HandleFunc("GET /api/admin/users", s.adminListUsers)
	mux.HandleFunc("GET /api/admin/users/{id}", s.adminGetUser)
	mux.HandleFunc("PUT /api/admin/users/{id}", s.adminUpdateUser)
	mux.HandleFunc("PATCH /api/admin/users/{id}", s.adminUpdateUser)
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.adminDeleteUser)
	mux.HandleFunc("GET /api/admin/bookings", s.adminListBookings)
	mux.HandleFunc("GET /api/admin/bookings/{id}", s.adminGetBooking)
	mux.HandleFunc("POST /api/admin/bookings/{id}/cancel", s.adminCancelBooking)
	mux.HandleFunc("DELETE /api/admin/bookings/{id}", s.adminDeleteBooking)

	mux.HandleFunc("POST /api/bookings", s.createBooking)
	mux.HandleFunc("GET /api/bookings", s.listBookings)
	mux.HandleFunc("GET /api/bookings/{id}", s.getBooking)
	mux.HandleFunc("DELETE /api/bookings/{id}", s.cancelBooking)

	return cors(mux), nil
}

// Port returns the configured listen port for the local dev server.
func Port() string { return env("PORT", "8080") }

// cors allows browser-based API clients and answers preflight OPTIONS requests.
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
