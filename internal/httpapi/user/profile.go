package user

import (
	"errors"
	"net/http"
	"strconv"

	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/store/core"
)

// Me returns the signed-in account.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	u.Password = ""
	web.WriteJSON(w, http.StatusOK, u)
}

// UpdateMe edits the caller's own profile: name, email and password.
//
// Changing the email or the password requires the current password. A bearer
// token alone is not enough: tokens live for a day and can be stolen, and
// either field is a route to permanent control of the account — the email
// because password recovery goes there, the password for obvious reasons.
//
// Role is not editable here at all, so no request body can promote its sender.
func (h *Handlers) UpdateMe(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	var req struct {
		Name            string `json:"name"`
		Email           string `json:"email"`
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if web.ReadJSON(w, r, &req) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}

	changingEmail := req.Email != "" && req.Email != u.Email
	if changingEmail || req.NewPassword != "" {
		if !core.CheckPassword(u.Password, req.CurrentPassword) {
			web.Fail(w, http.StatusForbidden, "currentPassword is required and must be correct")
			return
		}
	}
	if req.NewPassword != "" && len(req.NewPassword) < core.MinPasswordLen {
		web.Fail(w, http.StatusBadRequest,
			"newPassword must be at least "+strconv.Itoa(core.MinPasswordLen)+" characters")
		return
	}

	if req.Name != "" {
		u.Name = req.Name
	}
	if changingEmail {
		u.Email = req.Email
	}
	if req.NewPassword != "" {
		hash, err := core.HashPassword(req.NewPassword)
		if err != nil {
			web.ServerError(w, err)
			return
		}
		u.Password = hash
	}
	if err := h.UserStore.UpdateUser(u); err != nil {
		if errors.Is(err, core.ErrDuplicate) {
			web.Fail(w, http.StatusConflict, "email already registered")
			return
		}
		web.ServerError(w, err)
		return
	}
	// A new password retires every other session, in case one of them is why
	// the password is being changed.
	if req.NewPassword != "" {
		if err := h.UserStore.RevokeOtherSessions(u.ID, web.BearerToken(r)); err != nil {
			web.ServerError(w, err)
			return
		}
	}
	u.Password = ""
	web.WriteJSON(w, http.StatusOK, u)
}
