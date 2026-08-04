package admin

import (
	"errors"
	"net/http"

	"ticketmaster/internal/httpapi/web"
	adminstore "ticketmaster/internal/store/admin"
	"ticketmaster/internal/store/core"
)

func (h *Handlers) ListBookings(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	q := r.URL.Query()
	p := web.PageParams(r)
	bookings, total, err := h.AdminStore.AllBookings(adminstore.BookingFilter{
		UserID: q.Get("userId"), EventID: q.Get("eventId"), Status: q.Get("status"),
	}, p)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WritePage(w, "bookings", bookings, total, p)
}

func (h *Handlers) GetBooking(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	b, err := h.Store.BookingByID(r.PathValue("id"))
	if err != nil {
		web.Fail(w, http.StatusNotFound, "booking not found")
		return
	}
	web.WriteJSON(w, http.StatusOK, b)
}

// adminCancelBooking cancels anyone's booking and returns the tickets to the
// event. Deleting instead would lose the audit trail, so cancel is a separate
// action from DELETE.
func (h *Handlers) CancelBooking(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	b, err := h.AdminStore.AdminCancelBooking(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			web.Fail(w, http.StatusNotFound, "booking not found")
			return
		}
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, b)
}

func (h *Handlers) DeleteBooking(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	h.Deleted(w, h.AdminStore.DeleteBooking(r.PathValue("id")), "booking", "")
}
