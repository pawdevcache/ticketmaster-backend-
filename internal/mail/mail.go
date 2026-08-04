// Package mail delivers transactional email: password resets and booking
// confirmations.
//
// Delivery is best-effort by design. A password reset must answer the caller
// identically whether or not the address exists, so it cannot report a send
// failure without leaking that the account is real; and a booking is already
// paid for by the time the confirmation goes out, so failing the request
// because a mail server was slow would lose the sale, not save it. Sends are
// therefore logged on failure and never returned to the handler.
//
// With no SMTP host configured the package falls back to writing messages to
// the log, which is what local development and the test suite rely on.
package mail

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// sendTimeout bounds a single delivery. A user is waiting on the response, so
// an unreachable mail server must fail quickly rather than hold the request
// open until the client gives up.
const sendTimeout = 10 * time.Second

// Mailer sends transactional messages. Build one with New.
type Mailer struct {
	// send is the transport. Swapped in tests, and replaced by a logging
	// implementation when SMTP is unconfigured.
	send   func(to, subject, body string) error
	from   string
	appURL string
	live   bool
}

// Config describes the mail transport. Host empty means "not configured": New
// then returns a Mailer that logs instead of sending.
type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	// Implicit selects TLS-on-connect (port 465) instead of STARTTLS
	// (port 587). Providers differ; both are in common use.
	Implicit bool
	// AppURL is the front end's base address, used to build the link in a
	// password-reset email.
	AppURL string
}

// New returns a Mailer for the given config, falling back to a logging
// transport when no SMTP host is set.
func New(c Config) *Mailer {
	if c.Host == "" {
		return &Mailer{
			from:   c.From,
			appURL: c.AppURL,
			send: func(to, subject, body string) error {
				log.Printf("[mail: not configured, logging instead] to=%s subject=%q\n%s", to, subject, body)
				return nil
			},
		}
	}
	m := &Mailer{from: c.From, appURL: c.AppURL, live: true}
	m.send = func(to, subject, body string) error { return sendSMTP(c, to, subject, body) }
	return m
}

// Live reports whether messages actually leave the process. False means the
// logging fallback is in use.
func (m *Mailer) Live() bool { return m.live }

// deliver sends and swallows the error, logging it. See the package comment
// for why a failure never reaches the caller.
func (m *Mailer) deliver(to, subject, body string) {
	if to == "" {
		return
	}
	if err := m.send(to, subject, body); err != nil {
		log.Printf("mail: could not send %q to %s: %v", subject, to, err)
	}
}

func sendSMTP(c Config, to, subject, body string) error {
	addr := net.JoinHostPort(c.Host, c.Port)
	msg := buildMessage(c.From, to, subject, body)

	var auth smtp.Auth
	if c.Username != "" {
		auth = smtp.PlainAuth("", c.Username, c.Password, c.Host)
	}

	if !c.Implicit {
		// smtp.SendMail negotiates STARTTLS when the server offers it, and
		// refuses to send a plaintext password otherwise.
		return smtp.SendMail(addr, auth, c.From, []string{to}, msg)
	}

	// Implicit TLS: the connection is encrypted before any SMTP dialogue.
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: sendTimeout}, "tcp", addr, &tls.Config{ServerName: c.Host})
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		return err
	}
	defer cl.Quit()
	if auth != nil {
		if err := cl.Auth(auth); err != nil {
			return err
		}
	}
	if err := cl.Mail(c.From); err != nil {
		return err
	}
	if err := cl.Rcpt(to); err != nil {
		return err
	}
	wc, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	return wc.Close()
}

// buildMessage assembles an RFC 5322 message. Lines are CRLF-terminated
// because SMTP requires it, and a bare LF can end the message early.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + header(from) + "\r\n")
	b.WriteString("To: " + header(to) + "\r\n")
	b.WriteString("Subject: " + header(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}

// header strips CR and LF from a header value.
//
// Recipient addresses come from user records and registration does not
// validate their format, so an address containing a newline would otherwise
// let the registrant inject arbitrary headers — a Bcc to a mailing list, or a
// second message body.
func header(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}
