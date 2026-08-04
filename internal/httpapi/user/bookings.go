package user

import (
	"errors"
	"log"
	"net/http"

	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/store/core"
)

func (h *Handlers) CreateBooking(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	var req struct {
		EventID  string `json:"eventId"`
		Quantity int    `json:"quantity"`
	}
	if web.ReadJSON(w, r, &req) != nil {
		web.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	// With payments configured the seats are only held: the booking starts
	// pending and the caller must pay before the hold lapses. With payments
	// off it confirms straight away, which is how the service behaved before
	// payments existed.
	b, err := h.UserStore.Book(u.ID, req.EventID, req.Quantity, h.Payments != nil)
	switch {
	case err == nil:
		if h.Payments == nil {
			// Best-effort: the seats are already reserved, so a mail failure
			// must not turn a completed purchase into an error.
			h.Mail.BookingConfirmed(u.Email, b)
			web.WriteJSON(w, http.StatusCreated, b)
			return
		}
		out := map[string]any{"booking": b}
		if intent, ierr := h.intentFor(b); ierr != nil {
			// The hold stands and the caller can retry payment; losing the
			// intent is not worth discarding their seats over.
			log.Printf("payments: could not create an intent for booking %s: %v", b.ID, ierr)
			out["paymentError"] = "could not start payment, please retry"
		} else {
			out["payment"] = intent
		}
		web.WriteJSON(w, http.StatusCreated, out)
	case errors.Is(err, core.ErrNotFound):
		web.Fail(w, http.StatusNotFound, "event not found")
	case errors.Is(err, core.ErrSoldOut):
		web.Fail(w, http.StatusConflict, core.ErrSoldOut.Error())
	default:
		web.ServerError(w, err) // unexpected DB error — don't leak internals
	}
}

func (h *Handlers) ListBookings(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	bookings, err := h.UserStore.UserBookings(u.ID)
	if err != nil {
		web.ServerError(w, err)
		return
	}
	web.WriteJSON(w, http.StatusOK, bookings)
}

func (h *Handlers) GetBooking(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	b, err := h.UserStore.Booking(r.PathValue("id"), u.ID)
	if err != nil {
		web.Fail(w, http.StatusNotFound, "booking not found")
		return
	}
	web.WriteJSON(w, http.StatusOK, b)
}

func (h *Handlers) CancelBooking(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	b, err := h.UserStore.CancelBooking(r.PathValue("id"), u.ID)
	if err != nil {
		web.Fail(w, http.StatusNotFound, "booking not found")
		return
	}
	web.WriteJSON(w, http.StatusOK, b)
}
