package user

import (
	"errors"
	"log"
	"net/http"

	"ticketmaster/internal/httpapi/web"
	"ticketmaster/internal/models"
	"ticketmaster/internal/payment"
	"ticketmaster/internal/store/core"
)

// intentFor returns the charge a pending booking is waiting on, creating it on
// first use and reusing it afterwards. Reusing matters: a buyer who reloads
// the checkout page should land on the same charge rather than accumulate
// abandoned intents against one booking.
func (h *Handlers) intentFor(b *models.Booking) (*payment.Intent, error) {
	if b.PaymentIntentID != "" {
		return &payment.Intent{
			ID: b.PaymentIntentID, Amount: payment.MinorUnits(b.Total),
			Provider: h.Payments.Name(),
		}, nil
	}
	intent, err := h.Payments.CreateIntent(payment.MinorUnits(b.Total), "", b.ID)
	if err != nil {
		return nil, err
	}
	if err := h.UserStore.SetPaymentIntent(b.ID, intent.ID); err != nil {
		return nil, err
	}
	b.PaymentIntentID = intent.ID
	return intent, nil
}

// PayBooking confirms a held booking once its charge has actually been paid.
//
// The provider is asked directly whether the intent succeeded. The caller says
// only which booking they are paying for — believing a client that claims
// "paid" would hand out tickets for free.
func (h *Handlers) PayBooking(w http.ResponseWriter, r *http.Request) {
	u := h.Auth(w, r)
	if u == nil {
		return
	}
	if h.Payments == nil {
		web.Fail(w, http.StatusNotFound, "payments are not enabled")
		return
	}

	b, err := h.UserStore.PendingBooking(r.PathValue("id"), u.ID)
	if err != nil {
		// Either it is not theirs, or it is no longer awaiting payment. Look
		// again without the status filter so an already-paid booking reads as
		// success rather than a confusing 404 after a double-clicked button.
		if done, derr := h.UserStore.Booking(r.PathValue("id"), u.ID); derr == nil {
			if done.Status == models.BookingConfirmed {
				web.WriteJSON(w, http.StatusOK, map[string]any{"booking": done, "status": "already paid"})
				return
			}
			web.Fail(w, http.StatusConflict, "this booking is "+done.Status+" and cannot be paid")
			return
		}
		web.Fail(w, http.StatusNotFound, "booking not found")
		return
	}
	if b.PaymentIntentID == "" {
		web.Fail(w, http.StatusConflict, "no payment was started for this booking")
		return
	}

	if err := h.Payments.Verify(b.PaymentIntentID); err != nil {
		if errors.Is(err, payment.ErrNotSucceeded) {
			web.Fail(w, http.StatusPaymentRequired, "payment has not completed")
			return
		}
		// The provider is unreachable or erroring: the hold stands, so the
		// buyer can try again rather than lose their seats.
		log.Printf("payments: could not verify intent %s: %v", b.PaymentIntentID, err)
		web.Fail(w, http.StatusBadGateway, "could not confirm payment with the provider, please retry")
		return
	}

	paid, err := h.UserStore.MarkPaid(b.ID, u.ID, b.PaymentIntentID)
	switch {
	case err == nil:
		h.Mail.BookingConfirmed(u.Email, paid)
		web.WriteJSON(w, http.StatusOK, map[string]any{"booking": paid, "status": "paid"})
	case errors.Is(err, core.ErrSoldOut):
		// It stopped being pending between our read and the update — most
		// likely the hold lapsed a moment ago.
		web.Fail(w, http.StatusConflict, "the hold on this booking expired before payment completed")
	case errors.Is(err, core.ErrNotFound):
		web.Fail(w, http.StatusNotFound, "booking not found")
	default:
		web.ServerError(w, err)
	}
}
