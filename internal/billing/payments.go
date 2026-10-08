package billing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

func (s *Service) Webhook(ctx context.Context, data []byte, signature, rid string) error {
	if s.Provider == nil {
		return ErrUnavailable
	}
	event, err := s.Provider.VerifyPayment(ctx, data, signature)
	if err != nil {
		return ErrSignature
	}
	// Resolve server identity from the order before applying the shared lock order:
	// server row -> order -> subscription. Expiry/admin lifecycle use server first.
	var id uuid.UUID
	if err = s.DB.QueryRow(ctx, "SELECT server_id FROM orders WHERE id=$1", event.OrderID).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return server.ErrNotFound
	} else if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var state server.State
	if err = tx.QueryRow(ctx, "SELECT status FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&state); err != nil {
		return err
	}
	o, err := ScanOrder(tx.QueryRow(ctx, "SELECT "+OrderColumns+" FROM orders WHERE id=$1 FOR UPDATE", event.OrderID))
	if err != nil {
		return err
	}
	if event.AmountCents != o.PriceCents || event.Currency != o.Currency || o.Reference == nil || event.CheckoutReference != *o.Reference || o.Provider == nil || *o.Provider != s.Provider.Name() {
		return ErrConflict
	}
	normalized := event
	normalized.IssuedAt = 0
	canonical, _ := json.Marshal(normalized)
	fingerprint := digest(canonical)
	var previous string
	var previousOrder uuid.UUID
	err = tx.QueryRow(ctx, "SELECT fingerprint,order_id FROM payment_events WHERE provider=$1 AND event_id=$2", s.Provider.Name(), event.ID).Scan(&previous, &previousOrder)
	if err == nil {
		if previous != fingerprint || previousOrder != o.ID {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	tag, err := tx.Exec(ctx, "INSERT INTO payment_events(provider,event_id,order_id,fingerprint) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", s.Provider.Name(), event.ID, o.ID, fingerprint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		if err = tx.QueryRow(ctx, "SELECT fingerprint,order_id FROM payment_events WHERE provider=$1 AND event_id=$2", s.Provider.Name(), event.ID).Scan(&previous, &previousOrder); err != nil {
			return err
		}
		if previous != fingerprint || previousOrder != o.ID {
			return ErrConflict
		}
		return nil
	}
	if o.Status == "PAID" {
		return tx.Commit(ctx)
	}
	if o.Status != "PENDING" || state == server.Cancelled {
		return ErrConflict
	}
	var terminating bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE server_id=$1 AND action='TERMINATE' AND status IN ('PENDING','RUNNING'))", id).Scan(&terminating); err != nil {
		return err
	}
	if terminating {
		return ErrConflict
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		return err
	}
	var end time.Time
	if o.Kind == "NEW" {
		if state != server.PendingPayment {
			return ErrConflict
		}
		end = periodEnd(now, o.BillingPeriod)
		if _, err = tx.Exec(ctx, `INSERT INTO subscriptions(id,user_id,server_id,plan_id,status,current_period_end,price_cents,currency,billing_period) VALUES($1,$2,$3,$4,'ACTIVE',$5,$6,$7,$8)`, uuid.New(), o.UserID, id, o.PlanID, end, o.PriceCents, o.Currency, o.BillingPeriod); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE servers SET status='PAYMENT_CONFIRMED',updated_at=now() WHERE id=$1", id); err != nil {
			return err
		}
		if err = queue(ctx, tx, o.UserID, id, jobs.Provision, "payment:"+o.ID.String(), rid); err != nil {
			return err
		}
	} else {
		var existingEnd time.Time
		var amount int64
		var currency, period string
		if err = tx.QueryRow(ctx, "SELECT current_period_end,price_cents,currency,billing_period FROM subscriptions WHERE server_id=$1 AND user_id=$2 FOR UPDATE", id, o.UserID).Scan(&existingEnd, &amount, &currency, &period); err != nil {
			return err
		}
		if amount != o.PriceCents || currency != o.Currency || period != o.BillingPeriod {
			return ErrConflict
		}
		base := now
		if existingEnd.After(base) {
			base = existingEnd
		}
		end = periodEnd(base, o.BillingPeriod)
		if _, err = tx.Exec(ctx, "UPDATE subscriptions SET status='ACTIVE',current_period_end=$2,cancel_at_period_end=false,updated_at=now() WHERE server_id=$1", id, end); err != nil {
			return err
		}
		// An expiry-suspension already running is reconciled after it finishes. Payment
		// never removes an ADMIN suspension or races an active lifecycle command.
		if err = reconcileServer(ctx, tx, o.UserID, id, now, rid); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE orders SET status='PAID',payment_reference=$2,paid_at=now(),updated_at=now() WHERE id=$1", o.ID, event.CheckoutReference); err != nil {
		return err
	}
	if err = audit.Record(ctx, tx, o.UserID, id, "payment.confirmed", rid, map[string]any{"order_id": o.ID, "provider": s.Provider.Name(), "period_end": end}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Calendar billing clamps the day rather than rolling Jan 31 into March. UTC
// avoids host timezone/DST changes in financial period boundaries.
func periodEnd(base time.Time, period string) time.Time {
	base = base.UTC()
	year, month, day := base.Date()
	if period == "YEARLY" {
		year++
	} else {
		month++
		if month > 12 {
			month = 1
			year++
		}
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, base.Hour(), base.Minute(), base.Second(), base.Nanosecond(), time.UTC)
}
func queue(ctx context.Context, tx pgx.Tx, actor, id uuid.UUID, action jobs.Action, source, rid string) error {
	var active bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE server_id=$1 AND status IN ('PENDING','RUNNING'))", id).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrConflict
	}
	// Deterministic keys protect system retries as well as transaction atomicity.
	key := uuid.NewSHA1(uuid.NameSpaceOID, []byte(source+":"+string(action)))
	fingerprint := digest([]byte(source + ":" + string(action)))
	var jobID uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO jobs(id,server_id,actor_id,action,idempotency_key,request_fingerprint,request_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(server_id,idempotency_key) DO NOTHING RETURNING id`, uuid.New(), id, actor, action, key, fingerprint, rid).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, tx, actor, id, "billing.job.enqueued", rid, map[string]any{"job_id": jobID, "action": action})
}
