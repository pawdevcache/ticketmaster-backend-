package admin

import (
	"errors"
	"net/http"

	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/models"
	"ticketmaster/internal/store/core"
)

// scrub blanks password hashes before users go out in a response.
func scrub(users []*models.User) []*models.User {
	for _, u := range users {
		u.Password = ""
	}
	return users
}

func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	q := r.URL.Query()
	p := web.PageParams(r)
	users, total, err := h.AdminStore.Users(q.Get("keyword"), q.Get("role"), p)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WritePage(w, "users", scrub(users), total, p)
}

func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	u, err := h.AdminStore.User(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "user not found")
		return
	}
	u.Password = ""
	web.WriteJSON(w, http.StatusOK, u)
}

// adminUpdateUser edits any account, including its role. A password in the body
// is plaintext and gets hashed; leaving it out keeps the existing one.
func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	admin := h.AdminAuth(w, r)
	if admin == nil {
		return
	}
	u, err := h.AdminStore.User(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "user not found")
		return
	}
	id, hash := u.ID, u.Password
	if web.ReadJSON(w, r, u) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	u.ID = id
	if u.Email == "" {
		web.Fail(w, http.StatusBadRequest, "email required")
		return
	}
	// The stored value is a bcrypt hash, so anything different came from the
	// request body as plaintext and needs hashing.
	if u.Password != "" && u.Password != hash {
		if u.Password, err = core.HashPassword(u.Password); err != nil {
			web.ServerError(w, err)
			return
		}
	} else {
		u.Password = hash
	}
	if u.Role != models.RoleAdmin {
		u.Role = models.RoleUser
	}
	// Guard against an admin locking themselves out of the admin routes.
	if u.ID == admin.ID && !u.IsAdmin() {
		web.Fail(w, http.StatusBadRequest, "you cannot remove your own admin role")
		return
	}
	if err := h.AdminStore.UpdateUser(u); err != nil {
		if errors.Is(err, core.ErrDuplicate) {
			web.Fail(w, http.StatusConflict, "email already registered")
			return
		}
		web.ServerError(w, err)
		return
	}
	u.Password = ""
	web.WriteJSON(w, http.StatusOK, u)
}

func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	admin := h.AdminAuth(w, r)
	if admin == nil {
		return
	}
	if r.PathValue("id") == admin.ID {
		web.Fail(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	h.Deleted(w, h.AdminStore.DeleteUser(r.PathValue("id")), "user",
		"user holds confirmed bookings: cancel those first")
}
