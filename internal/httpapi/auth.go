package httpapi

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"ticketmaster/internal/config"
	"ticketmaster/internal/models"
	"ticketmaster/internal/store"
)

// --- users & auth ---

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	s.doRegister(w, r, models.RoleUser)
}

// adminRegister creates an admin account. Because an admin can create events,
// venues and attractions, sign-up is gated behind ADMIN_REGISTRATION_KEY: the caller
// must present it in the X-Admin-Key header (or "adminKey" in the body).
// Registration is disabled outright while that variable is unset.
func (s *Server) adminRegister(w http.ResponseWriter, r *http.Request) {
	want := config.Get("ADMIN_REGISTRATION_KEY", "")
	if want == "" {
		fail(w, http.StatusServiceUnavailable, "admin registration is disabled: set ADMIN_REGISTRATION_KEY")
		return
	}
	got := r.Header.Get("X-Admin-Key")
	if got == "" {
		// Fall back to the body so clients that can't set headers still work.
		// Read it from a copy — doRegister decodes the body again below.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			fail(w, http.StatusBadRequest, "invalid body")
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
		fail(w, http.StatusForbidden, "invalid admin key")
		return
	}
	s.doRegister(w, r, models.RoleAdmin)
}

// doRegister creates an account with the given role. The role always comes from
// the caller, never from the request body, so "role":"admin" in a payload to
// POST /api/register is ignored.
func (s *Server) doRegister(w http.ResponseWriter, r *http.Request, role string) {
	var u models.User
	if readJSON(w, r, &u) != nil || u.Name == "" || u.Email == "" || u.Password == "" {
		fail(w, http.StatusBadRequest, "name, email, password required")
		return
	}
	u.Role = role
	if err := s.store.Register(&u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail(w, http.StatusConflict, "email already registered")
			return
		}
		serverError(w, err)
		return
	}
	u.Password = "" // hide from response
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) { s.doLogin(w, r, false) }

// forgotPassword starts the reset flow for the "forgot password?" screen.
//
// It always answers 200 with the same message, whether or not the address has
// an account, so the endpoint can't be used to discover who is registered.
// There is no mail service wired up, so the token is written to the server log;
// outside production it also comes back in the response to keep local testing
// workable. See resetPassword for step two.
func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Email string `json:"email"`
	}
	if readJSON(w, r, &c) != nil || c.Email == "" {
		fail(w, http.StatusBadRequest, "email required")
		return
	}
	out := map[string]string{"status": "if that email has an account, a reset token has been issued"}
	tok, err := s.store.CreateReset(c.Email)
	switch {
	case err == nil:
		log.Printf("password reset token issued for %s (valid %s): %s", c.Email, store.ResetTTL, tok)
		if config.DevMode() {
			out["resetToken"] = tok
			out["note"] = "resetToken is returned outside production only; set ENV=production to suppress it"
		}
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrUnauthorized):
		// Unknown address, or an admin account: reveal neither.
	default:
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// resetPassword completes the flow: it trades a token from forgotPassword for
// a new password. The token is single-use and expires; a successful reset also
// signs out every existing session for that account.
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if readJSON(w, r, &c) != nil || c.Token == "" || len(c.NewPassword) < store.MinPasswordLen {
		fail(w, http.StatusBadRequest,
			"token and a new password of at least "+strconv.Itoa(store.MinPasswordLen)+" characters are required")
		return
	}
	switch err := s.store.ResetPassword(c.Token, c.NewPassword); {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"status": "password updated, please sign in again"})
	case errors.Is(err, store.ErrUnauthorized):
		fail(w, http.StatusBadRequest, "reset token is invalid, already used, or expired")
	default:
		serverError(w, err)
	}
}

// logout revokes the token the request was made with.
//
// Until this existed a client could only forget its token locally while the
// session stayed valid server-side for a full day, so a leaked token could not
// be withdrawn. Only the calling session is revoked; the same account signed
// in elsewhere stays signed in.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if s.auth(w, r) == nil {
		return
	}
	if err := s.store.Logout(bearerToken(r)); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminLogin issues a token only for admin accounts. Ordinary users get the
// same "invalid credentials" response as a wrong password, so the endpoint
// doesn't reveal which emails belong to admins.
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) { s.doLogin(w, r, true) }

func (s *Server) doLogin(w http.ResponseWriter, r *http.Request, adminOnly bool) {
	var c struct{ Email, Password string }
	if readJSON(w, r, &c) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	tok, u, err := s.store.Login(c.Email, c.Password)
	if err != nil || (adminOnly && !u.IsAdmin()) {
		fail(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "user": u})
}
