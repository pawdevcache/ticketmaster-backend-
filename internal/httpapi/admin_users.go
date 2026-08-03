package httpapi

import (
	"errors"
	"net/http"

	"ticketmaster/internal/models"
	"ticketmaster/internal/store"
)

// --- admin: user accounts ---

// scrub blanks password hashes before users go out in a response.
func scrub(users []*models.User) []*models.User {
	for _, u := range users {
		u.Password = ""
	}
	return users
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	q := r.URL.Query()
	p := pageParams(r)
	users, total, err := s.store.Users(q.Get("keyword"), q.Get("role"), p)
	if err != nil {
		serverError(w, err)
		return
	}
	writePage(w, "users", scrub(users), total, p)
}

func (s *Server) adminGetUser(w http.ResponseWriter, r *http.Request) {
	if s.adminAuth(w, r) == nil {
		return
	}
	u, err := s.store.User(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	u.Password = ""
	writeJSON(w, http.StatusOK, u)
}

// adminUpdateUser edits any account, including its role. A password in the body
// is plaintext and gets hashed; leaving it out keeps the existing one.
func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	admin := s.adminAuth(w, r)
	if admin == nil {
		return
	}
	u, err := s.store.User(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	id, hash := u.ID, u.Password
	if readJSON(w, r, u) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	u.ID = id
	if u.Email == "" {
		fail(w, http.StatusBadRequest, "email required")
		return
	}
	// The stored value is a bcrypt hash, so anything different came from the
	// request body as plaintext and needs hashing.
	if u.Password != "" && u.Password != hash {
		if u.Password, err = store.HashPassword(u.Password); err != nil {
			serverError(w, err)
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
		fail(w, http.StatusBadRequest, "you cannot remove your own admin role")
		return
	}
	if err := s.store.UpdateUser(u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			fail(w, http.StatusConflict, "email already registered")
			return
		}
		serverError(w, err)
		return
	}
	u.Password = ""
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	admin := s.adminAuth(w, r)
	if admin == nil {
		return
	}
	if r.PathValue("id") == admin.ID {
		fail(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	s.deleted(w, s.store.DeleteUser(r.PathValue("id")), "user",
		"user holds confirmed bookings: cancel those first")
}
