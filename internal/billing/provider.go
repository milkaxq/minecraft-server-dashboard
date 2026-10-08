package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrSignature = errors.New("invalid payment signature")
var ErrUnavailable = errors.New("payment checkout is not configured")
var ErrConflict = errors.New("billing operation conflicts with current state")

type CheckoutRequest struct {
	OrderID     uuid.UUID
	AmountCents int64
	Currency    string
}
type Checkout struct {
	Reference string `json:"reference"`
	URL       string `json:"url"`
	Provider  string `json:"provider"`
}
type PaymentEvent struct {
	ID                string    `json:"event_id"`
	OrderID           uuid.UUID `json:"order_id"`
	CheckoutReference string    `json:"checkout_reference"`
	AmountCents       int64     `json:"amount_cents"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"`
	IssuedAt          int64     `json:"issued_at"`
}
type RefundRequest struct {
	Reference      string
	AmountCents    int64
	IdempotencyKey uuid.UUID
}

// Provider adapters own their checkout, signature and refund protocols. Business
// services still resolve amounts, currencies, ownership and entitlement in SQL.
type PaymentProvider interface {
	Name() string
	CreateCheckout(context.Context, CheckoutRequest) (Checkout, error)
	VerifyPayment(context.Context, []byte, string) (PaymentEvent, error)
	Refund(context.Context, RefundRequest) (string, error)
}

// MockProvider is development-only: no card input, currency movement or bank data.
type MockProvider struct{ secret []byte }

func NewMock(secret string) (*MockProvider, error) {
	if len(secret) < 32 || strings.HasPrefix(secret, "replace-") {
		return nil, ErrUnavailable
	}
	return &MockProvider{secret: []byte(secret)}, nil
}
func (p *MockProvider) Name() string { return "mock" }
func (p *MockProvider) sign(data []byte) string {
	h := hmac.New(sha256.New, p.secret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
func (p *MockProvider) CreateCheckout(_ context.Context, r CheckoutRequest) (Checkout, error) {
	if r.OrderID == uuid.Nil || r.AmountCents < 0 || len(r.Currency) != 3 {
		return Checkout{}, ErrConflict
	}
	data, _ := json.Marshal(r)
	reference := "mock_" + p.sign(data)
	return Checkout{Reference: reference, URL: "/checkout/" + r.OrderID.String(), Provider: p.Name()}, nil
}
func (p *MockProvider) VerifyPayment(_ context.Context, data []byte, signature string) (PaymentEvent, error) {
	var e PaymentEvent
	received, err := hex.DecodeString(signature)
	if err != nil {
		return e, ErrSignature
	}
	expected, _ := hex.DecodeString(p.sign(data))
	if !hmac.Equal(received, expected) {
		return e, ErrSignature
	}
	now := time.Now().Unix()
	if err = json.Unmarshal(data, &e); err != nil || e.ID == "" || len(e.ID) > 128 || e.OrderID == uuid.Nil || e.Status != "paid" || e.AmountCents < 0 || e.IssuedAt < now-300 || e.IssuedAt > now+30 {
		return e, ErrSignature
	}
	return e, nil
}
func (p *MockProvider) paidEvent(r CheckoutRequest, reference string) ([]byte, string) {
	e := PaymentEvent{ID: "mock_paid_" + r.OrderID.String(), OrderID: r.OrderID, CheckoutReference: reference, AmountCents: r.AmountCents, Currency: r.Currency, Status: "paid", IssuedAt: time.Now().Unix()}
	data, _ := json.Marshal(e)
	return data, p.sign(data)
}
func (p *MockProvider) Refund(_ context.Context, r RefundRequest) (string, error) {
	if !strings.HasPrefix(r.Reference, "mock_") || r.AmountCents <= 0 || r.IdempotencyKey == uuid.Nil {
		return "", ErrConflict
	}
	return fmt.Sprintf("mock_refund_%s", p.sign([]byte(fmt.Sprintf("%s:%d:%s", r.Reference, r.AmountCents, r.IdempotencyKey)))), nil
}
