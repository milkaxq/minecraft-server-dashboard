package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

type HostingCosts struct {
	NodeID           uuid.UUID `json:"node_id"`
	Currency         string    `json:"currency"`
	NodeMonthly      int64     `json:"node_monthly_cents"`
	OverheadMonthly  int64     `json:"overhead_monthly_cents"`
	PerServerMonthly int64     `json:"per_server_monthly_cents"`
	FeeBPS           int       `json:"fee_bps"`
	FeeFixed         int64     `json:"fee_fixed_cents"`
	TaxBPS           int       `json:"tax_bps"`
	TargetMarginBPS  int       `json:"target_margin_bps"`
	OccupancyBPS     int       `json:"occupancy_bps"`
}
type SalesTotal struct {
	Provider      string `json:"provider"`
	Currency      string `json:"currency"`
	PaidCount     int    `json:"paid_count"`
	PaidCents     int64  `json:"paid_cents"`
	RefundedCount int    `json:"refunded_count"`
	RefundedCents int64  `json:"refunded_cents"`
}
type Finances struct {
	Month string         `json:"month"`
	Costs []HostingCosts `json:"costs"`
	Sales []SalesTotal   `json:"sales"`
}

const costColumns = "node_id,currency,node_monthly_cents,overhead_monthly_cents,per_server_monthly_cents,fee_bps,fee_fixed_cents,tax_bps,target_margin_bps,occupancy_bps"

func (s Service) Finances(ctx context.Context, actor uuid.UUID, month time.Time) (Finances, error) {
	month = month.UTC()
	out := Finances{Month: month.UTC().Format("2006-01"), Costs: []HostingCosts{}, Sales: []SalesTotal{}}
	if err := s.Require(ctx, actor); err != nil {
		return out, err
	}
	month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows, err := s.DB.Query(ctx, "SELECT "+costColumns+" FROM hosting_cost_profiles ORDER BY node_id")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c HostingCosts
		if err = rows.Scan(&c.NodeID, &c.Currency, &c.NodeMonthly, &c.OverheadMonthly, &c.PerServerMonthly, &c.FeeBPS, &c.FeeFixed, &c.TaxBPS, &c.TargetMarginBPS, &c.OccupancyBPS); err != nil {
			rows.Close()
			return out, err
		}
		out.Costs = append(out.Costs, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = s.DB.Query(ctx, `SELECT coalesce(provider,''),currency,count(*) FILTER(WHERE status='PAID'),coalesce(sum(price_cents) FILTER(WHERE status='PAID'),0),count(*) FILTER(WHERE status='REFUNDED'),coalesce(sum(price_cents) FILTER(WHERE status='REFUNDED'),0) FROM orders WHERE paid_at >= $1 AND paid_at < $2 AND status IN ('PAID','REFUNDED') GROUP BY provider,currency ORDER BY provider,currency`, month, month.AddDate(0, 1, 0))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v SalesTotal
		if err = rows.Scan(&v.Provider, &v.Currency, &v.PaidCount, &v.PaidCents, &v.RefundedCount, &v.RefundedCents); err != nil {
			return out, err
		}
		out.Sales = append(out.Sales, v)
	}
	return out, rows.Err()
}
func (s Service) SaveHostingCosts(ctx context.Context, actor uuid.UUID, c HostingCosts, rid string) (HostingCosts, error) {
	if err := s.Require(ctx, actor); err != nil {
		return c, err
	}
	if c.NodeID == uuid.Nil || len(c.Currency) != 3 || c.NodeMonthly < 0 || c.NodeMonthly > 100000000 || c.OverheadMonthly < 0 || c.OverheadMonthly > 100000000 || c.PerServerMonthly < 0 || c.PerServerMonthly > 100000000 || c.FeeFixed < 0 || c.FeeFixed > 1000000 || c.FeeBPS < 0 || c.FeeBPS > 5000 || c.TaxBPS < 0 || c.TaxBPS > 5000 || c.TargetMarginBPS < 0 || c.TargetMarginBPS > 9500 || c.OccupancyBPS < 1 || c.OccupancyBPS > 10000 {
		return c, server.ErrInvalid
	}
	for _, r := range c.Currency {
		if r < 'A' || r > 'Z' {
			return c, server.ErrInvalid
		}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(context.Background())
	var allowed bool
	if err = tx.QueryRow(ctx, "SELECT is_admin AND verified_at IS NOT NULL FROM users WHERE id=$1 FOR SHARE", actor).Scan(&allowed); err != nil || !allowed {
		return c, jobs.ErrForbidden
	}
	var node uuid.UUID
	if err = tx.QueryRow(ctx, "SELECT id FROM nodes WHERE id=$1 FOR SHARE", c.NodeID).Scan(&node); err != nil {
		if err == pgx.ErrNoRows {
			return c, server.ErrNotFound
		}
		return c, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO hosting_cost_profiles (`+costColumns+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(node_id) DO UPDATE SET currency=excluded.currency,node_monthly_cents=excluded.node_monthly_cents,overhead_monthly_cents=excluded.overhead_monthly_cents,per_server_monthly_cents=excluded.per_server_monthly_cents,fee_bps=excluded.fee_bps,fee_fixed_cents=excluded.fee_fixed_cents,tax_bps=excluded.tax_bps,target_margin_bps=excluded.target_margin_bps,occupancy_bps=excluded.occupancy_bps,updated_at=now()`, c.NodeID, c.Currency, c.NodeMonthly, c.OverheadMonthly, c.PerServerMonthly, c.FeeBPS, c.FeeFixed, c.TaxBPS, c.TargetMarginBPS, c.OccupancyBPS)
	if err != nil {
		return c, err
	}
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "hosting.costs.updated", rid, map[string]any{"node_id": c.NodeID, "currency": c.Currency}); err != nil {
		return c, err
	}
	return c, tx.Commit(ctx)
}
