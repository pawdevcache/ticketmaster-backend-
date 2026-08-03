// Package httpapi is the HTTP layer: routing, request decoding, authorisation
// and response shaping. It is the only package that knows about status codes.
//
// Routes fall into three tiers, which is the thing to keep straight when
// adding one:
//
//   - public — discovery reads, register, login, the forgotten-password pair
//   - user   — needs a bearer token; a user only ever sees their own records
//   - admin  — needs a token belonging to an admin account (see adminAuth)
//
// Every write to a discovery resource is admin-only, including the create
// routes that share a path with a public read.
//
// Updates are deliberately partial. PUT and PATCH share a handler that decodes
// the request over the record already in the database, so a body needs only
// the fields that change — and any field the client must not control is put
// back afterwards.
package httpapi

import (
	"log"
	"net/http"

	"ticketmaster/internal/config"
	"ticketmaster/internal/store"
)

// New builds the fully-wired API handler. Used by both the local dev server
// (main.go) and the Vercel serverless entrypoint (api/index.go).
func New() (http.Handler, error) {
	config.Load(".env") // no-op when the file is absent (e.g. on Vercel)
	st, err := store.NewStore(config.Get("MONGO_URI", "mongodb://localhost:27017"), config.Get("DB_NAME", "ticketmaster"))
	if err != nil {
		return nil, err
	}
	// Best-effort: enforce unique email + token expiry. Don't fail startup if
	// the DB is briefly unreachable — health will surface that separately.
	if err := st.EnsureIndexes(); err != nil {
		log.Println("warning: could not create indexes:", err)
	}
	// Same deal for the optional bootstrap admin: log and carry on.
	if err := st.EnsureAdmin(config.Get("ADMIN_NAME", "Admin"), config.Get("ADMIN_EMAIL", ""), config.Get("ADMIN_PASSWORD", "")); err != nil {
		log.Println("warning: could not seed admin user:", err)
	}
	// Rate limiting protects the endpoints where a guess is worth something:
	// credentials and password recovery.
	limit, window := rateLimitSettings()
	if limit <= 0 {
		log.Println("warning: rate limiting is disabled (RATE_LIMIT_ATTEMPTS=0)")
	}
	s := &Server{store: st, limiter: newLimiter(limit, window)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		if err := st.Ping(); err != nil {
			writeJSON(w, 200, map[string]string{"status": "degraded", "db": "disconnected", "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok", "db": "connected"})
	})

	// API reference. /docs is the human-readable page; /openapi.yaml is the
	// spec any OpenAPI tool can consume.
	mux.HandleFunc("GET /docs", swaggerUI)
	mux.HandleFunc("GET /openapi.yaml", openAPI)

	// Root: {$} matches only the exact "/" path, so real unknown routes still 404.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"service": "Ticketmaster API",
			"status":  "running",
			"endpoints": map[string][]string{
				"public": {
					"GET /health", "GET /docs", "GET /openapi.yaml",
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

	// Ticketing / commerce. Each rate-limited route gets its own bucket, so
	// exhausting login attempts does not also lock out registration.
	mux.HandleFunc("POST /api/register", s.rateLimited("register", s.register))
	mux.HandleFunc("POST /api/login", s.rateLimited("login", s.login))
	// Forgotten-password flow: request a token, then trade it for a new password.
	mux.HandleFunc("POST /api/forgot-password", s.rateLimited("forgot-password", s.forgotPassword))
	mux.HandleFunc("POST /api/reset-password", s.rateLimited("reset-password", s.resetPassword))

	// Admin accounts. Registration needs ADMIN_REGISTRATION_KEY; the resulting token
	// is what the POST /discovery/v2/* create routes require.
	mux.HandleFunc("POST /api/admin/register", s.rateLimited("admin-register", s.adminRegister))
	mux.HandleFunc("POST /api/admin/login", s.rateLimited("admin-login", s.adminLogin))
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
