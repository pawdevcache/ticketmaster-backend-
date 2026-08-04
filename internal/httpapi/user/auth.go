package user

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"ticketmaster/internal/config"
	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/models"
	"ticketmaster/internal/store"
)

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	h.RegisterAccount(w, r, models.RoleUser)
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) { h.SignIn(w, r, false) }

// forgotPassword starts the reset flow for the "forgot password?" screen.
//
// It always answers 200 with the same message, whether or not the address has
// an account, so the endpoint can't be used to discover who is registered.
// There is no mail service wired up, so the token is written to the server log;
// outside production it also comes back in the response to keep local testing
// workable. See resetPassword for step two.
func (h *Handlers) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Email string `json:"email"`
	}
	if web.ReadJSON(w, r, &c) != nil || c.Email == "" {
		web.Fail(w, http.StatusBadRequest, "email required")
		return
	}
	out := map[string]string{"status": "if that email has an account, a reset token has been issued"}
	tok, err := h.Store.CreateReset(c.Email)
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
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, out)
}

// resetPassword completes the flow: it trades a token from forgotPassword for
// a new password. The token is single-use and expires; a successful reset also
// signs out every existing session for that account.
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if web.ReadJSON(w, r, &c) != nil || c.Token == "" || len(c.NewPassword) < store.MinPasswordLen {
		web.Fail(w, http.StatusBadRequest,
			"token and a new password of at least "+strconv.Itoa(store.MinPasswordLen)+" characters are required")
		return
	}
	switch err := h.Store.ResetPassword(c.Token, c.NewPassword); {
	case err == nil:
		web.WriteJSON(w, http.StatusOK, map[string]string{"status": "password updated, please sign in again"})
	case errors.Is(err, store.ErrUnauthorized):
		web.Fail(w, http.StatusBadRequest, "reset token is invalid, already used, or expired")
	default:
		web.ServerError(w, err)
	}
}

// logout revokes the token the request was made with.
//
// Until this existed a client could only forget its token locally while the
// session stayed valid server-side for a full day, so a leaked token could not
// be withdrawn. Only the calling session is revoked; the same account signed
// in elsewhere stays signed in.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if h.Auth(w, r) == nil {
		return
	}
	if err := h.Store.Logout(web.BearerToken(r)); err != nil {
		web.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
