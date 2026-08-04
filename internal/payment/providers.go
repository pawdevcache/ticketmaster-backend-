package payment

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// --- test provider ---

// testProvider issues intents that are always already paid. It exists so the
// hold-and-pay flow can be exercised without a payment account; it must never
// be selected in production, which is why it is not the default.
type testProvider struct{ currency string }

func (p *testProvider) Name() string { return "test" }

func (p *testProvider) CreateIntent(amountMinor int64, currency, reference string) (*Intent, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	id := "pi_test_" + hex.EncodeToString(b)
	if currency == "" {
		currency = p.currency
	}
	return &Intent{
		ID: id, ClientSecret: id + "_secret", Amount: amountMinor,
		Currency: currency, Provider: p.Name(),
	}, nil
}

// Verify accepts any intent this provider could have issued. The prefix check
// is not security — it is a guard against a stray production intent id being
// waved through by a misconfigured deployment.
func (p *testProvider) Verify(intentID string) error {
	if !strings.HasPrefix(intentID, "pi_test_") {
		return ErrNotSucceeded
	}
	return nil
}

// --- stripe ---

// stripeProvider talks to Stripe's REST API directly rather than through the
// SDK, which keeps the dependency list at two libraries.
//
// UNVERIFIED: written against Stripe's documented API but never exercised
// against a real account, since that needs live keys. Test it in Stripe's test
// mode before taking real money.
type stripeProvider struct {
	key      string
	currency string
}

func (p *stripeProvider) Name() string { return "stripe" }

func (p *stripeProvider) CreateIntent(amountMinor int64, currency, reference string) (*Intent, error) {
	if currency == "" {
		currency = p.currency
	}
	form := url.Values{
		"amount":                             {strconv.FormatInt(amountMinor, 10)},
		"currency":                           {currency},
		"automatic_payment_methods[enabled]": {"true"},
		"metadata[bookingId]":                {reference},
	}
	var out struct {
		ID           string `json:"id"`
		ClientSecret string `json:"client_secret"`
		Amount       int64  `json:"amount"`
		Currency     string `json:"currency"`
	}
	if err := p.call(http.MethodPost, "https://api.stripe.com/v1/payment_intents", form, &out); err != nil {
		return nil, err
	}
	return &Intent{
		ID: out.ID, ClientSecret: out.ClientSecret, Amount: out.Amount,
		Currency: out.Currency, Provider: p.Name(),
	}, nil
}

// Verify re-reads the intent from Stripe. Only "succeeded" counts: an intent
// can be created, attempted and abandoned, and every other state means no
// money moved.
func (p *stripeProvider) Verify(intentID string) error {
	var out struct {
		Status string `json:"status"`
	}
	if err := p.call(http.MethodGet, "https://api.stripe.com/v1/payment_intents/"+url.PathEscape(intentID), nil, &out); err != nil {
		return err
	}
	if out.Status != "succeeded" {
		return fmt.Errorf("%w (status %q)", ErrNotSucceeded, out.Status)
	}
	return nil
}

func (p *stripeProvider) call(method, endpoint string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.key, "")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(raw, &e)
		if e.Error.Message == "" {
			e.Error.Message = strings.TrimSpace(string(raw))
		}
		return fmt.Errorf("stripe %s: %s", res.Status, e.Error.Message)
	}
	return json.Unmarshal(raw, out)
}
