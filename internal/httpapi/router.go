// Package httpapi wires the HTTP layer together. It owns the route table and
// nothing else: the handlers live in sub-packages, split by who may call them.
//
//   - web   — shared plumbing: dependencies, auth, responses, paging, limiting
//   - user  — public catalogue reads, sign-up and sign-in, a customer's own
//     bookings
//   - admin — catalogue writes, account management, all bookings, analytics
//
// The split is the access tier made structural. Every route registered from
// the admin package requires an administrator, so reading this file tells you
// who can reach what without opening a single handler.
//
// Two conventions run through the table below. Every write to a discovery
// resource is admin-only, even where it shares a path with a public read. And
// PUT and PATCH share a handler: both apply a partial update, decoding the
// request over the stored record, so a body carries only the fields that
// change.
package httpapi

import (
	"log"
	"net/http"

	"ticketmaster/internal/config"
	"ticketmaster/internal/httpapi/admin"
	"ticketmaster/internal/httpapi/user"
	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/mail"
	adminstore "ticketmaster/internal/store/admin"
	"ticketmaster/internal/store/core"
	userstore "ticketmaster/internal/store/user"
)

// New builds the fully-wired API handler. Used by both the local dev server
// (main.go) and the Vercel serverless entrypoint (api/index.go).
func New() (http.Handler, error) {
	config.Load(".env") // no-op when the file is absent (e.g. on Vercel)
	st, err := core.NewStore(config.Get("MONGO_URI", "mongodb://localhost:27017"), config.Get("DB_NAME", "ticketmaster"))
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
	// Tickets issued before check-in existed need a code, or their QR will not
	// scan. Matches nothing after the first run.
	if n, err := st.EnsureTicketCodes(); err != nil {
		log.Println("warning: could not backfill ticket codes:", err)
	} else if n > 0 {
		log.Printf("assigned ticket codes to %d existing bookings", n)
	}
	// Rate limiting protects the endpoints where a guess is worth something:
	// credentials and password recovery.
	limit, window := web.RateLimitSettings()
	if limit <= 0 {
		log.Println("warning: rate limiting is disabled (RATE_LIMIT_ATTEMPTS=0)")
	}
	mailer := mail.New(mail.Config{
		Host:     config.Get("SMTP_HOST", ""),
		Port:     config.Get("SMTP_PORT", "587"),
		Username: config.Get("SMTP_USERNAME", ""),
		Password: config.Get("SMTP_PASSWORD", ""),
		From:     config.Get("SMTP_FROM", "no-reply@ticketmaster.local"),
		Implicit: config.Get("SMTP_TLS", "starttls") == "implicit",
		AppURL:   config.Get("APP_URL", ""),
	})
	if !mailer.Live() {
		log.Println("warning: SMTP_HOST is unset — emails will be written to the log, not sent")
	}
	deps := &web.Deps{Store: st, Limiter: web.NewLimiter(limit, window), Mail: mailer}
	u := user.New(deps, userstore.New(st))
	a := admin.New(deps, adminstore.New(st))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		if err := st.Ping(); err != nil {
			web.WriteJSON(w, 200, map[string]string{"status": "degraded", "db": "disconnected", "error": err.Error()})
			return
		}
		web.WriteJSON(w, 200, map[string]string{"status": "ok", "db": "connected"})
	})

	// API reference. /docs is the human-readable page; /openapi.yaml is the
	// spec any OpenAPI tool can consume.
	mux.HandleFunc("GET /docs", swaggerUI)
	mux.HandleFunc("GET /openapi.yaml", openAPI)

	// Root: {$} matches only the exact "/" path, so real unknown routes still 404.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		web.WriteJSON(w, 200, map[string]any{
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
					"POST /api/logout",
					"POST /api/bookings", "GET /api/bookings",
					"GET /api/bookings/{id}", "DELETE /api/bookings/{id}",
				},
				"admin": {
					"POST /api/admin/register", "POST /api/admin/login", "GET /api/admin/me",
					"POST /discovery/v2/{events|venues|attractions|classifications}",
					"PUT|PATCH /discovery/v2/{events|venues|attractions|classifications}/{id}",
					"DELETE /discovery/v2/{events|venues|attractions|classifications}/{id}",
					"GET /api/admin/analytics", "POST /api/admin/tickets/check-in",
					"GET /api/admin/users", "GET /api/admin/users/{id}",
					"PUT|PATCH /api/admin/users/{id}", "DELETE /api/admin/users/{id}",
					"GET /api/admin/bookings", "GET /api/admin/bookings/{id}",
					"POST /api/admin/bookings/{id}/cancel", "DELETE /api/admin/bookings/{id}",
				},
			},
		})
	})

	// Discovery API. The reads come from the user package, the writes from the
	// admin package — the tier of every route is visible in this table.
	for _, res := range []struct {
		path                             string
		search, get, create, update, del http.HandlerFunc
	}{
		{"events", u.SearchEvents, u.GetEvent, a.CreateEvent, a.UpdateEvent, a.DeleteEvent},
		{"venues", u.SearchVenues, u.GetVenue, a.CreateVenue, a.UpdateVenue, a.DeleteVenue},
		{"attractions", u.SearchAttractions, u.GetAttraction, a.CreateAttraction, a.UpdateAttraction, a.DeleteAttraction},
		{"classifications", u.SearchClassifications, u.GetClassification, a.CreateClassification, a.UpdateClassification, a.DeleteClassification},
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
	mux.HandleFunc("POST /api/register", deps.RateLimited("register", u.Register))
	mux.HandleFunc("POST /api/login", deps.RateLimited("login", u.Login))
	// Not rate-limited: ending your own session must always be possible.
	mux.HandleFunc("POST /api/logout", u.Logout)
	// Forgotten-password flow: request a token, then trade it for a new password.
	mux.HandleFunc("POST /api/forgot-password", deps.RateLimited("forgot-password", u.ForgotPassword))
	mux.HandleFunc("POST /api/reset-password", deps.RateLimited("reset-password", u.ResetPassword))

	mux.HandleFunc("POST /api/bookings", u.CreateBooking)
	mux.HandleFunc("GET /api/bookings", u.ListBookings)
	mux.HandleFunc("GET /api/bookings/{id}", u.GetBooking)
	mux.HandleFunc("DELETE /api/bookings/{id}", u.CancelBooking)

	// Admin accounts. Registration needs ADMIN_REGISTRATION_KEY; the resulting token
	// is what the POST /discovery/v2/* create routes require.
	mux.HandleFunc("POST /api/admin/register", deps.RateLimited("admin-register", a.Register))
	mux.HandleFunc("POST /api/admin/login", deps.RateLimited("admin-login", a.Login))
	mux.HandleFunc("GET /api/admin/me", a.Me)

	// Dashboard figures for the admin Overview tab.
	mux.HandleFunc("GET /api/admin/analytics", a.Analytics)

	// Gate check-in: scan a ticket QR code and admit its holder.
	mux.HandleFunc("POST /api/admin/tickets/check-in", a.CheckIn)

	// Admin management of accounts and of every user's bookings.
	mux.HandleFunc("GET /api/admin/users", a.ListUsers)
	mux.HandleFunc("GET /api/admin/users/{id}", a.GetUser)
	mux.HandleFunc("PUT /api/admin/users/{id}", a.UpdateUser)
	mux.HandleFunc("PATCH /api/admin/users/{id}", a.UpdateUser)
	mux.HandleFunc("DELETE /api/admin/users/{id}", a.DeleteUser)
	mux.HandleFunc("GET /api/admin/bookings", a.ListBookings)
	mux.HandleFunc("GET /api/admin/bookings/{id}", a.GetBooking)
	mux.HandleFunc("POST /api/admin/bookings/{id}/cancel", a.CancelBooking)
	mux.HandleFunc("DELETE /api/admin/bookings/{id}", a.DeleteBooking)

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
