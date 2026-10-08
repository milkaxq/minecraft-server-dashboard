package httpapi

import (
	"io"
	"net/http"

	"github.com/google/uuid"

	"minecraft-hosting/internal/billing"
	"minecraft-hosting/internal/server"
)

func (a *API) billingRoutes(mux *http.ServeMux) {
	routes := map[string]http.HandlerFunc{"GET /billing/config": a.billingConfig, "GET /orders": a.orders, "POST /orders": a.createOrder, "GET /orders/{order}": a.getOrder, "POST /orders/{order}/checkout": a.checkout, "POST /orders/{order}/cancel": a.cancelOrder, "POST /orders/{order}/mock-pay": a.mockPay, "GET /subscriptions": a.subscriptions, "PUT /servers/{id}/subscription": a.subscriptionCancellation, "POST /servers/{id}/renewals": a.renewal}
	for route, handler := range routes {
		mux.HandleFunc(route, a.protected(a.liveLimit("billing", 120, handler)))
	}
	mux.HandleFunc("POST /payments/webhooks/mock", a.limited(a.paymentWebhook))
}
func (a *API) configuredBilling(w http.ResponseWriter, r *http.Request) bool {
	if a.Billing == nil {
		a.fail(w, r, billing.ErrUnavailable)
		return false
	}
	return true
}
func (a *API) billingConfig(w http.ResponseWriter, r *http.Request) {
	name := "disabled"
	if a.Billing != nil && a.Billing.Provider != nil {
		name = a.Billing.Provider.Name()
	}
	write(w, 200, map[string]any{"provider": name, "checkout_enabled": name != "disabled", "collects_money": name != "disabled" && name != "mock"})
}
func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var input server.CreateInput
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	o, err := a.Billing.Create(r.Context(), currentUser(r).ID, key, input, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, o)
}
func (a *API) orders(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	o, err := a.Billing.Orders(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, o)
}
func orderID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue("order"))
	if err != nil {
		return id, server.ErrNotFound
	}
	return id, nil
}
func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	id, err := orderID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	o, err := a.Billing.Get(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, o)
}
func (a *API) checkout(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	id, err := orderID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var body struct{}
	if err = decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	c, err := a.Billing.Checkout(r.Context(), currentUser(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, c)
}
func (a *API) mockPay(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	id, err := orderID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var body struct{}
	if err = decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	if err = a.Billing.CompleteMock(r.Context(), currentUser(r).ID, id, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]string{"status": "confirmed"})
}
func (a *API) paymentWebhook(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	if a.Billing.Provider == nil || a.Billing.Provider.Name() != "mock" {
		a.fail(w, r, billing.ErrUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		a.fail(w, r, server.ErrInvalid)
		return
	}
	if err = a.Billing.Webhook(r.Context(), data, r.Header.Get("X-Payment-Signature"), requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]string{"status": "processed"})
}
func (a *API) subscriptions(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	v, err := a.Billing.Subscriptions(r.Context(), currentUser(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, v)
}
func (a *API) renewal(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	key, err := operationKey(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var body struct{}
	if err = decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	o, err := a.Billing.Renew(r.Context(), currentUser(r).ID, id, key, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, o)
}
func (a *API) subscriptionCancellation(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		a.fail(w, r, server.ErrNotFound)
		return
	}
	var input struct {
		Cancel *bool `json:"cancel_at_period_end"`
	}
	if err = decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	if input.Cancel == nil {
		a.fail(w, r, server.ErrInvalid)
		return
	}
	if err = a.Billing.SetCancellation(r.Context(), currentUser(r).ID, id, *input.Cancel, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}

func (a *API) cancelOrder(w http.ResponseWriter, r *http.Request) {
	if !a.configuredBilling(w, r) {
		return
	}
	id, err := orderID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct{}
	if err = decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err = a.Billing.CancelOrder(r.Context(), currentUser(r).ID, id, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
