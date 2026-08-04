package admin

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"

	"ticketmaster/internal/config"
	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/models"
)

// adminRegister creates an admin account. Because an admin can create events,
// venues and attractions, sign-up is gated behind ADMIN_REGISTRATION_KEY: the caller
// must present it in the X-Admin-Key header (or "adminKey" in the body).
// Registration is disabled outright while that variable is unset.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	want := config.Get("ADMIN_REGISTRATION_KEY", "")
	if want == "" {
		web.Fail(w, http.StatusServiceUnavailable, "admin registration is disabled: set ADMIN_REGISTRATION_KEY")
		return
	}
	got := r.Header.Get("X-Admin-Key")
	if got == "" {
		// Fall back to the body so clients that can't set headers still work.
		// Read it from a copy — doRegister decodes the body again below.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, web.MaxBodyBytes))
		if err != nil {
			web.Fail(w, http.StatusBadRequest, "invalid body")
			return
		}
		var k struct {
			AdminKey string `json:"adminKey"`
		}
		json.Unmarshal(body, &k) // a malformed body is reported by doRegister
		got = k.AdminKey
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		web.Fail(w, http.StatusForbidden, "invalid admin key")
		return
	}
	h.RegisterAccount(w, r, models.RoleAdmin)
}

// adminLogin issues a token only for admin accounts. Ordinary users get the
// same "invalid credentials" response as a wrong password, so the endpoint
// doesn't reveal which emails belong to admins.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) { h.SignIn(w, r, true) }

// adminMe returns the signed-in admin, letting a client verify a stored token.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u := h.AdminAuth(w, r)
	if u == nil {
		return
	}
	u.Password = ""
	web.WriteJSON(w, http.StatusOK, u)
}
