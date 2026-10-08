package billing

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/server"
)

type Service struct {
	DB       *pgxpool.Pool
	Provider PaymentProvider
}
type Order struct {
	CheckoutURL   *string    `json:"-"`
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	ServerID      uuid.UUID  `json:"server_id"`
	PlanID        uuid.UUID  `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	PriceCents    int64      `json:"price_cents"`
	Currency      string     `json:"currency"`
	BillingPeriod string     `json:"billing_period"`
	Status        string     `json:"status"`
	Kind          string     `json:"kind"`
	Provider      *string    `json:"provider"`
	Reference     *string    `json:"-"`
	PaidAt        *time.Time `json:"paid_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

const OrderColumns = "id,user_id,server_id,plan_id,plan_name,price_cents,currency,billing_period,status,kind,provider,checkout_reference,checkout_url,paid_at,created_at"

func ScanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.UserID, &o.ServerID, &o.PlanID, &o.PlanName, &o.PriceCents, &o.Currency, &o.BillingPeriod, &o.Status, &o.Kind, &o.Provider, &o.Reference, &o.CheckoutURL, &o.PaidAt, &o.CreatedAt)
	return o, err
}

type Subscription struct {
	ID                uuid.UUID `json:"id"`
	ServerID          uuid.UUID `json:"server_id"`
	PlanID            uuid.UUID `json:"plan_id"`
	ServerName        string    `json:"server_name"`
	Status            string    `json:"status"`
	CurrentPeriodEnd  time.Time `json:"current_period_end"`
	PriceCents        *int64    `json:"price_cents"`
	Currency          *string   `json:"currency"`
	BillingPeriod     *string   `json:"billing_period"`
	CancelAtPeriodEnd bool      `json:"cancel_at_period_end"`
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func (s *Service) Get(ctx context.Context, user, id uuid.UUID) (Order, error) {
	o, err := ScanOrder(s.DB.QueryRow(ctx, "SELECT "+OrderColumns+" FROM orders WHERE id=$1 AND user_id=$2", id, user))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, server.ErrNotFound
	}
	return o, err
}
func (s *Service) Orders(ctx context.Context, user uuid.UUID) ([]Order, error) {
	rows, err := s.DB.Query(ctx, "SELECT "+OrderColumns+" FROM orders WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100", user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		o, e := ScanOrder(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Service) Subscriptions(ctx context.Context, user uuid.UUID) ([]Subscription, error) {
	rows, err := s.DB.Query(ctx, `SELECT su.id,su.server_id,su.plan_id,s.name,su.status,su.current_period_end,su.price_cents,su.currency,su.billing_period,su.cancel_at_period_end FROM subscriptions su JOIN servers s ON s.id=su.server_id WHERE su.user_id=$1 ORDER BY su.current_period_end,su.id LIMIT 100`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		var v Subscription
		if err = rows.Scan(&v.ID, &v.ServerID, &v.PlanID, &v.ServerName, &v.Status, &v.CurrentPeriodEnd, &v.PriceCents, &v.Currency, &v.BillingPeriod, &v.CancelAtPeriodEnd); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Order keys are serialized per user so concurrent retries create one server and
// one bill. Prices/resources are read from the locked plan, never browser values.
func (s *Service) Create(ctx context.Context, user, key uuid.UUID, in server.CreateInput, rid string) (Order, error) {
	if s.Provider == nil {
		return Order{}, ErrUnavailable
	}
	if key == uuid.Nil {
		return Order{}, server.ErrInvalid
	}
	if err := in.Validate(); err != nil {
		return Order{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "order-user:"+user.String()); err != nil {
		return Order{}, err
	}
	body, _ := json.Marshal(in)
	fingerprint := digest(body)
	old, found, err := orderReplay(ctx, tx, user, key, fingerprint)
	if err != nil {
		return old, err
	}
	if found {
		return old, nil
	}
	// Resolve price before creation and retain the plan lock across both snapshots.
	p, err := plan.Scan(tx.QueryRow(ctx, "SELECT "+plan.Columns+" FROM plans WHERE id=$1 AND enabled FOR SHARE", in.PlanID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, plan.ErrUnavailable
	}
	if err != nil {
		return Order{}, err
	}
	v, err := (server.Service{DB: s.DB}).CreateTx(ctx, tx, user, in, rid)
	if err != nil {
		return Order{}, err
	}
	o, err := insertOrder(ctx, tx, user, v.ID, p.ID, p.Name, p.PriceCents, p.Currency, p.BillingPeriod, "NEW", key, fingerprint)
	if err != nil {
		return o, err
	}
	if err = audit.Record(ctx, tx, user, v.ID, "order.created", rid, map[string]any{"order_id": o.ID, "price_cents": o.PriceCents, "kind": o.Kind}); err != nil {
		return o, err
	}
	return o, tx.Commit(ctx)
}
func orderReplay(ctx context.Context, tx pgx.Tx, user, key uuid.UUID, fingerprint string) (Order, bool, error) {
	var previous string
	err := tx.QueryRow(ctx, "SELECT request_fingerprint FROM orders WHERE user_id=$1 AND idempotency_key=$2", user, key).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	if previous != fingerprint {
		return Order{}, true, ErrConflict
	}
	o, err := ScanOrder(tx.QueryRow(ctx, "SELECT "+OrderColumns+" FROM orders WHERE user_id=$1 AND idempotency_key=$2", user, key))
	return o, true, err
}
func insertOrder(ctx context.Context, tx pgx.Tx, user, serverID, planID uuid.UUID, name string, amount int64, currency, period, kind string, key uuid.UUID, fingerprint string) (Order, error) {
	return ScanOrder(tx.QueryRow(ctx, `INSERT INTO orders(id,user_id,server_id,plan_id,plan_name,price_cents,currency,billing_period,status,kind,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PENDING',$9,$10,$11) RETURNING `+OrderColumns, uuid.New(), user, serverID, planID, name, amount, currency, period, kind, key, fingerprint))
}
func (s *Service) Renew(ctx context.Context, user, id, key uuid.UUID, rid string) (Order, error) {
	if s.Provider == nil {
		return Order{}, ErrUnavailable
	}
	if key == uuid.Nil {
		return Order{}, server.ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "order-user:"+user.String()); err != nil {
		return Order{}, err
	}
	var state server.State
	var planID uuid.UUID
	err = tx.QueryRow(ctx, "SELECT status,plan_id FROM servers WHERE id=$1 AND owner_id=$2 FOR UPDATE", id, user).Scan(&state, &planID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, server.ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if state == server.Cancelled || state == server.PendingPayment {
		return Order{}, ErrConflict
	}
	fingerprint := digest([]byte("renew:" + id.String()))
	old, found, err := orderReplay(ctx, tx, user, key, fingerprint)
	if err != nil {
		return old, err
	}
	if found {
		return old, nil
	}
	var amount *int64
	var currency, period *string
	err = tx.QueryRow(ctx, "SELECT price_cents,currency,billing_period FROM subscriptions WHERE server_id=$1 AND user_id=$2 FOR UPDATE", id, user).Scan(&amount, &currency, &period)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, server.ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if amount == nil || currency == nil || period == nil {
		return Order{}, ErrConflict
	}
	var terminating bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE server_id=$1 AND action='TERMINATE' AND status IN ('PENDING','RUNNING'))", id).Scan(&terminating); err != nil {
		return Order{}, err
	}
	if terminating {
		return Order{}, ErrConflict
	}
	existing, err := ScanOrder(tx.QueryRow(ctx, "SELECT "+OrderColumns+" FROM orders WHERE server_id=$1 AND status='PENDING'", id))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Order{}, err
	}
	var name string
	if err = tx.QueryRow(ctx, "SELECT name FROM plans WHERE id=$1", planID).Scan(&name); err != nil {
		return Order{}, err
	}
	o, err := insertOrder(ctx, tx, user, id, planID, name, *amount, *currency, *period, "RENEWAL", key, fingerprint)
	if err != nil {
		return o, err
	}
	if err = audit.Record(ctx, tx, user, id, "order.created", rid, map[string]any{"order_id": o.ID, "kind": "RENEWAL"}); err != nil {
		return o, err
	}
	return o, tx.Commit(ctx)
}

// Checkout creation is provider-idempotent on order UUID; a database rollback can
// safely retry CreateCheckout without opening a second payment session.
func (s *Service) Checkout(ctx context.Context, user, id uuid.UUID) (Checkout, error) {
	o, err := s.Get(ctx, user, id)
	if err != nil {
		return Checkout{}, err
	}
	if s.Provider == nil {
		return Checkout{}, ErrUnavailable
	}
	if o.Status != "PENDING" {
		return Checkout{}, ErrConflict
	}
	if o.Reference != nil && o.CheckoutURL != nil && o.Provider != nil && *o.Provider == s.Provider.Name() {
		return Checkout{Reference: *o.Reference, URL: *o.CheckoutURL, Provider: *o.Provider}, nil
	}
	checkout, err := s.Provider.CreateCheckout(ctx, CheckoutRequest{OrderID: o.ID, AmountCents: o.PriceCents, Currency: o.Currency})
	if err != nil {
		return Checkout{}, ErrUnavailable
	}
	if checkout.Provider != s.Provider.Name() || checkout.Reference == "" {
		return Checkout{}, ErrUnavailable
	}
	tag, err := s.DB.Exec(ctx, `UPDATE orders SET checkout_reference=$2,provider=$3,checkout_url=$4,updated_at=now() WHERE id=$1 AND status='PENDING' AND (checkout_reference IS NULL OR checkout_reference=$2)`, id, checkout.Reference, checkout.Provider, checkout.URL)
	if err != nil {
		return Checkout{}, err
	}
	if tag.RowsAffected() != 1 {
		return Checkout{}, ErrConflict
	}
	return checkout, nil
}
func (s *Service) CompleteMock(ctx context.Context, user, id uuid.UUID, rid string) error {
	o, err := s.Get(ctx, user, id)
	if err != nil {
		return err
	}
	provider, ok := s.Provider.(*MockProvider)
	if !ok {
		return ErrUnavailable
	}
	if o.Status == "PAID" {
		return nil
	}
	if _, err = s.Checkout(ctx, user, id); err != nil {
		return err
	}
	o, err = s.Get(ctx, user, id)
	if err != nil {
		return err
	}
	data, signature := provider.paidEvent(CheckoutRequest{OrderID: o.ID, AmountCents: o.PriceCents, Currency: o.Currency}, *o.Reference)
	return s.Webhook(ctx, data, signature, rid)
}
func (s *Service) SetCancellation(ctx context.Context, user, id uuid.UUID, cancel bool, rid string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var owner uuid.UUID
	err = tx.QueryRow(ctx, "SELECT owner_id FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && owner != user {
		return server.ErrNotFound
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "UPDATE subscriptions SET cancel_at_period_end=$2,updated_at=now() WHERE server_id=$1 AND user_id=$3 AND status='ACTIVE' AND current_period_end>now()", id, cancel, user)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if err = audit.Record(ctx, tx, user, id, "subscription.cancellation_changed", rid, map[string]any{"cancel_at_period_end": cancel}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) CancelOrder(ctx context.Context, user, id uuid.UUID, rid string) error {
	o, err := s.Get(ctx, user, id)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var state server.State
	if err = tx.QueryRow(ctx, "SELECT status FROM servers WHERE id=$1 FOR UPDATE", o.ServerID).Scan(&state); err != nil {
		return err
	}
	o, err = ScanOrder(tx.QueryRow(ctx, "SELECT "+OrderColumns+" FROM orders WHERE id=$1 AND user_id=$2 FOR UPDATE", id, user))
	if err != nil {
		return err
	}
	if o.Status == "CANCELLED" {
		return nil
	}
	if o.Status != "PENDING" {
		return ErrConflict
	}
	if o.Kind == "NEW" {
		if state != server.PendingPayment {
			return ErrConflict
		}
		if _, err = tx.Exec(ctx, "UPDATE servers SET status='CANCELLED',updated_at=now() WHERE id=$1", o.ServerID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE orders SET status='CANCELLED',updated_at=now() WHERE id=$1", id); err != nil {
		return err
	}
	if err = audit.Record(ctx, tx, user, o.ServerID, "order.cancelled", rid, map[string]any{"order_id": id}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
