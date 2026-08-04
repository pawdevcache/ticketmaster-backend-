package web

import (
	"errors"
	"net/http"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"
)

// doRegister creates an account with the given role. The role always comes from
// the caller, never from the request body, so "role":"admin" in a payload to
// POST /api/register is ignored.
func (d *Deps) RegisterAccount(w http.ResponseWriter, r *http.Request, role string) {
	var u models.User
	if ReadJSON(w, r, &u) != nil || u.Name == "" || u.Email == "" || u.Password == "" {
		Fail(w, http.StatusBadRequest, "name, email, password required")
		return
	}
	u.Role = role
	if err := d.Store.Register(&u); err != nil {
		if errors.Is(err, core.ErrDuplicate) {
			Fail(w, http.StatusConflict, "email already registered")
			return
		}
		ServerError(w, err)
		return
	}
	u.Password = "" // hide from response
	WriteJSON(w, http.StatusCreated, u)
}

func (d *Deps) SignIn(w http.ResponseWriter, r *http.Request, adminOnly bool) {
	var c struct{ Email, Password string }
	if ReadJSON(w, r, &c) != nil {
		Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	tok, u, err := d.Store.Login(c.Email, c.Password)
	if err != nil || (adminOnly && !u.IsAdmin()) {
		Fail(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"token": tok, "user": u})
}
