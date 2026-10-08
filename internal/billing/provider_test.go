package billing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMockSignatureTimestampAndIdempotency(t *testing.T) {
	p, err := NewMock("test-only-payment-secret-at-least-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	r := CheckoutRequest{OrderID: uuid.New(), AmountCents: 975, Currency: "USD"}
	a, err := p.CreateCheckout(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.CreateCheckout(context.Background(), r)
	if a != b {
		t.Fatal("checkout not idempotent")
	}
	data, sig := p.paidEvent(r, a.Reference)
	if _, err = p.VerifyPayment(context.Background(), data, sig); err != nil {
		t.Fatal(err)
	}
	if _, err = p.VerifyPayment(context.Background(), append(data, ' '), sig); !errors.Is(err, ErrSignature) {
		t.Fatal("tampered payload accepted")
	}
	var e PaymentEvent
	json.Unmarshal(data, &e)
	e.IssuedAt = time.Now().Add(-10 * time.Minute).Unix()
	old, _ := json.Marshal(e)
	if _, err = p.VerifyPayment(context.Background(), old, p.sign(old)); !errors.Is(err, ErrSignature) {
		t.Fatal("expired signature accepted")
	}
	if _, err = NewMock("replace-with-secret-long-enough-but-not-real"); err == nil {
		t.Fatal("placeholder secret accepted")
	}
	request := RefundRequest{Reference: a.Reference, AmountCents: 975, IdempotencyKey: uuid.New()}
	x, err := p.Refund(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	y, _ := p.Refund(context.Background(), request)
	if x != y {
		t.Fatal("mock refund replay changed")
	}
}
func TestCalendarBillingClampsDaysAndLeapYears(t *testing.T) {
	for _, tc := range []struct{ start, period, end string }{{"2027-01-31T13:00:00Z", "MONTHLY", "2027-02-28T13:00:00Z"}, {"2028-01-31T13:00:00Z", "MONTHLY", "2028-02-29T13:00:00Z"}, {"2028-02-29T13:00:00Z", "YEARLY", "2029-02-28T13:00:00Z"}} {
		base, _ := time.Parse(time.RFC3339, tc.start)
		if got := periodEnd(base, tc.period).Format(time.RFC3339); got != tc.end {
			t.Errorf("%s -> %s", tc.start, got)
		}
	}
}
