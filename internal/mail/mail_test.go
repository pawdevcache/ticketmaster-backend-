package mail

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ticketmaster/internal/models"
)

// capture swaps the transport so a test can read what would have been sent.
func capture(m *Mailer) *[]struct{ To, Subject, Body string } {
	var sent []struct{ To, Subject, Body string }
	m.send = func(to, subject, body string) error {
		sent = append(sent, struct{ To, Subject, Body string }{to, subject, body})
		return nil
	}
	return &sent
}

func TestNewFallsBackToLoggingWithoutAHost(t *testing.T) {
	if m := New(Config{}); m.Live() {
		t.Error("Live() = true with no SMTP host, want false so local runs still work")
	}
	if m := New(Config{Host: "smtp.example.com"}); !m.Live() {
		t.Error("Live() = false with an SMTP host configured, want true")
	}
}

// Addresses come from user records and registration does not validate their
// format. A newline in one would otherwise let the registrant append headers
// of their own — a Bcc, or a second body.
func TestBuildMessageStripsHeaderInjection(t *testing.T) {
	msg := string(buildMessage(
		"from@example.com",
		"victim@example.com\r\nBcc: everyone@example.com",
		"Subject\r\nX-Injected: yes",
		"body",
	))
	// The defence is that the injected text stays *inside* a header value
	// instead of starting a new header, so assert on line starts rather than
	// on the text appearing anywhere.
	headers, _, _ := strings.Cut(msg, "\r\n\r\n")
	for _, line := range strings.Split(headers, "\r\n") {
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			t.Errorf("malformed header line %q", line)
			continue
		}
		switch name {
		case "From", "To", "Subject", "MIME-Version", "Content-Type":
		default:
			t.Errorf("injected header %q became a real header:\n%s", name, msg)
		}
	}
	if !strings.Contains(msg, "To: victim@example.comBcc: everyone@example.com\r\n") {
		t.Errorf("recipient was not flattened onto one header line:\n%s", msg)
	}
	if strings.Count(msg, "\r\n\r\n") != 1 {
		t.Errorf("injection created a second header/body boundary:\n%q", msg)
	}
}

// SMTP terminates lines with CRLF; a bare LF can end the message early.
func TestBuildMessageUsesCRLF(t *testing.T) {
	msg := string(buildMessage("a@b.c", "d@e.f", "hi", "line one\nline two"))
	if strings.Contains(strings.ReplaceAll(msg, "\r\n", ""), "\n") {
		t.Errorf("message contains a bare LF:\n%q", msg)
	}
	if !strings.Contains(msg, "\r\n\r\nline one\r\nline two") {
		t.Errorf("body was not CRLF-normalised:\n%q", msg)
	}
}

func TestPasswordResetIncludesLinkAndToken(t *testing.T) {
	m := New(Config{AppURL: "https://tickets.example.com/"})
	sent := capture(m)
	m.PasswordReset("user@example.com", "abc123", time.Hour)

	if len(*sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(*sent))
	}
	got := (*sent)[0]
	if got.To != "user@example.com" {
		t.Errorf("To = %q", got.To)
	}
	// The link is what a reader clicks; the bare token is the fallback when a
	// mail client mangles it.
	if !strings.Contains(got.Body, "https://tickets.example.com/reset-password?token=abc123") {
		t.Errorf("reset link missing or malformed:\n%s", got.Body)
	}
	if !strings.Contains(got.Body, "abc123") {
		t.Errorf("raw token missing:\n%s", got.Body)
	}
	if !strings.Contains(got.Body, "1 hour") {
		t.Errorf("expiry not stated in readable form:\n%s", got.Body)
	}
}

func TestPasswordResetEscapesTheTokenInTheURL(t *testing.T) {
	m := New(Config{AppURL: "https://x.test"})
	sent := capture(m)
	m.PasswordReset("u@e.test", "a b&c", time.Hour)
	if !strings.Contains((*sent)[0].Body, "token=a+b%26c") {
		t.Errorf("token was not URL-escaped:\n%s", (*sent)[0].Body)
	}
}

func TestBookingConfirmedCarriesTheTicketCode(t *testing.T) {
	m := New(Config{})
	sent := capture(m)
	m.BookingConfirmed("buyer@example.com", &models.Booking{
		ID: "bk1", Quantity: 2, Total: 110, TicketCode: "ticketcode123",
		Event: &models.EventSummary{
			Name: "Rock Night", VenueName: "National Arena", VenueCity: "Colombo",
		},
	})
	body := (*sent)[0].Body
	for _, want := range []string{"Rock Night", "National Arena", "Colombo", "ticketcode123", "110.00", "bk1", "Purchased"} {
		if !strings.Contains(body, want) {
			t.Errorf("body is missing %q:\n%s", want, body)
		}
	}
	if subj := (*sent)[0].Subject; !strings.Contains(subj, "Rock Night") {
		t.Errorf("Subject = %q, want it to name the event", subj)
	}
}

// A booking whose event has been deleted still deserves a receipt.
func TestBookingConfirmedSurvivesAMissingEvent(t *testing.T) {
	m := New(Config{})
	sent := capture(m)
	m.BookingConfirmed("buyer@example.com", &models.Booking{ID: "bk2", Quantity: 1, Total: 10})
	if len(*sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(*sent))
	}
	if !strings.Contains((*sent)[0].Body, "bk2") {
		t.Error("receipt does not identify the booking")
	}
}

// A send failure is logged, never returned: see the package comment.
func TestDeliverSwallowsErrors(t *testing.T) {
	m := New(Config{})
	m.send = func(string, string, string) error { return errors.New("smtp is down") }
	m.PasswordReset("u@e.test", "tok", time.Hour) // must not panic
	m.BookingConfirmed("u@e.test", &models.Booking{ID: "x"})
}

// An empty recipient is not worth a connection attempt.
func TestDeliverSkipsAnEmptyRecipient(t *testing.T) {
	m := New(Config{})
	calls := 0
	m.send = func(string, string, string) error { calls++; return nil }
	m.PasswordReset("", "tok", time.Hour)
	if calls != 0 {
		t.Errorf("attempted %d sends to an empty address, want 0", calls)
	}
}

func TestHumanDuration(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{time.Hour, "1 hour"},
		{2 * time.Hour, "2 hours"},
		{30 * time.Minute, "30 minutes"},
		{90 * time.Second, "1 minutes"},
	} {
		if got := humanDuration(tc.in); got != tc.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A receipt without a purchase date is not a receipt. When payments are on the
// moment money moved is the one that matters; otherwise it is the booking time.
func TestBookingConfirmedShowsThePurchaseTime(t *testing.T) {
	created := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	paid := time.Date(2026, 3, 1, 9, 45, 0, 0, time.UTC)

	m := New(Config{})
	sent := capture(m)
	m.BookingConfirmed("b@e.test", &models.Booking{ID: "bk", CreatedAt: created})
	if !strings.Contains((*sent)[0].Body, "Sun 1 Mar 2026, 09:30") {
		t.Errorf("unpaid booking did not show its creation time:\n%s", (*sent)[0].Body)
	}

	m2 := New(Config{})
	sent2 := capture(m2)
	m2.BookingConfirmed("b@e.test", &models.Booking{ID: "bk", CreatedAt: created, PaidAt: &paid})
	if !strings.Contains((*sent2)[0].Body, "Sun 1 Mar 2026, 09:45") {
		t.Errorf("paid booking did not show the payment time:\n%s", (*sent2)[0].Body)
	}
}
