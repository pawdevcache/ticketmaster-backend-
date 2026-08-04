package mail

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"ticketmaster/internal/models"
)

// PasswordReset sends the recovery link. The raw token is included beneath the
// link because a mail client that mangles or strips links would otherwise
// leave the reader with no way to continue.
func (m *Mailer) PasswordReset(to, token string, ttl time.Duration) {
	link := m.appURL
	if link != "" {
		link = strings.TrimRight(link, "/") + "/reset-password?token=" + url.QueryEscape(token)
	}
	body := fmt.Sprintf(`Someone asked to reset the password for this address.

Open this link to choose a new one:

  %s

Or paste this code into the reset form:

  %s

The link works once and expires in %s. If you did not ask for it, nothing
has changed and you can ignore this message.
`, link, token, humanDuration(ttl))
	m.deliver(to, "Reset your Ticketmaster password", body)
}

// BookingConfirmed sends the receipt and the ticket code that the QR encodes,
// so a lost app still leaves the holder able to get in.
func (m *Mailer) BookingConfirmed(to string, b *models.Booking) {
	when, where := "", ""
	name := "your event"
	if b.Event != nil {
		name = b.Event.Name
		if !b.Event.Date.IsZero() {
			when = b.Event.Date.Format("Mon 2 Jan 2006, 15:04")
		}
		where = strings.Join(nonEmpty(b.Event.VenueName, b.Event.VenueAddress, b.Event.VenueCity), ", ")
	}
	body := fmt.Sprintf(`Your booking is confirmed.

  Event     %s
  When      %s
  Where     %s
  Tickets   %d
  Total     %.2f

Show this code at the door:

  %s

Reference %s
`, name, orDash(when), orDash(where), b.Quantity, b.Total, orDash(b.TicketCode), b.ID)
	m.deliver(to, "Your tickets for "+name, body)
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func orDash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

// humanDuration renders a TTL the way a person would say it, so the email
// reads "1 hour" rather than "1h0m0s".
func humanDuration(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		if h := int(d.Hours()); h == 1 {
			return "1 hour"
		} else {
			return fmt.Sprintf("%d hours", h)
		}
	case d >= time.Minute:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return d.String()
	}
}
