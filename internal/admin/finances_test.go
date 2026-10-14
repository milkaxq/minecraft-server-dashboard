package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

func TestFinancesAuthorizationCostsAndPurchasedSales(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	actor, customer, node, world := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{actor, customer} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,'test','Person',now(),$3)", id, id.String()+"@example.com", id == actor); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',600,12288,102400)", node); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) SELECT $1,$2,id,'World','PAYMENT_CONFIRMED','1.21.1','PAPER',ram_mb,disk_mb,cpu_limit,backup_limit,database_limit FROM plans ORDER BY price_cents LIMIT 1`, world, customer); err != nil {
		t.Fatal(err)
	}
	var plan uuid.UUID
	if err := db.QueryRow(ctx, "SELECT plan_id FROM servers WHERE id=$1", world).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	for _, sale := range []struct {
		provider, currency, status, paid string
		price                            int64
	}{
		{"mock", "USD", "PAID", "2026-10-10T10:00:00Z", 2000},
		{"stripe", "USD", "PAID", "2026-10-10T10:00:00Z", 600},
		{"stripe", "USD", "REFUNDED", "2026-10-10T10:00:00Z", 300},
		{"stripe", "EUR", "PAID", "2026-10-10T10:00:00Z", 900},
		{"stripe", "USD", "PAID", "2026-09-10T10:00:00Z", 5000},
	} {
		paid, _ := time.Parse(time.RFC3339, sale.paid)
		if _, err := db.Exec(ctx, "INSERT INTO orders(id,user_id,server_id,plan_id,price_cents,currency,billing_period,status,provider,paid_at) VALUES($1,$2,$3,$4,$5,$6,'MONTHLY',$7,$8,$9)", uuid.New(), customer, world, plan, sale.price, sale.currency, sale.status, sale.provider, paid); err != nil {
			t.Fatal(err)
		}
	}
	voiceAllocation := uuid.New()
	if _, err := db.Exec(ctx, "INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port,voice_enabled) VALUES($1,$2,1,'localhost',25565,true),($3,$2,2,'localhost',25566,false)", voiceAllocation, node, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO server_addon_ports(server_id,allocation_id,addon) VALUES($1,$2,'simple-voice-chat')", world, voiceAllocation); err != nil {
		t.Fatal(err)
	}
	s := Service{DB: db}
	nodes, err := s.Nodes(ctx, actor)
	if err != nil || len(nodes) != 1 || nodes[0].AvailableAllocations != 1 {
		t.Fatal("voice port counted as a free game address", nodes, err)
	}
	month := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.Finances(ctx, customer, month); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("customer read finances", err)
	}
	costs := HostingCosts{NodeID: node, Currency: "USD", NodeMonthly: 2000, OccupancyBPS: 5000, TargetMarginBPS: 3000}
	if _, err := s.SaveHostingCosts(ctx, customer, costs, "denied"); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("customer wrote costs", err)
	}
	if _, err := s.SaveHostingCosts(ctx, actor, costs, "saved"); err != nil {
		t.Fatal(err)
	}
	bad := costs
	bad.NodeMonthly = -1
	if _, err := s.SaveHostingCosts(ctx, actor, bad, "invalid"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("negative cost accepted", err)
	}
	bad = costs
	bad.Currency = "usd"
	if _, err := s.SaveHostingCosts(ctx, actor, bad, "invalid"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("invalid currency accepted", err)
	}
	bad = costs
	bad.NodeID = uuid.New()
	if _, err := s.SaveHostingCosts(ctx, actor, bad, "missing"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("unknown node accepted", err)
	}
	db.Exec(ctx, "UPDATE plans SET price_cents=99999 WHERE id=$1", plan)
	report, err := s.Finances(ctx, actor, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Costs) != 1 || report.Costs[0].NodeMonthly != 2000 || len(report.Sales) != 3 {
		t.Fatal("incorrect costs or currency/provider separation", report)
	}
	for _, sales := range report.Sales {
		if sales.Provider == "stripe" && sales.Currency == "USD" && (sales.PaidCents != 600 || sales.RefundedCents != 300 || sales.PaidCount != 1 || sales.RefundedCount != 1) {
			t.Fatal("sales changed with plan price or wrong month/refunds", sales)
		}
	}
	var audits int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action='hosting.costs.updated'").Scan(&audits); err != nil || audits != 1 {
		t.Fatal("cost audit missing", err, audits)
	}
	db.Exec(ctx, "UPDATE users SET is_admin=false WHERE id=$1", actor)
	if _, err = s.SaveHostingCosts(ctx, actor, costs, "revoked"); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("revoked admin wrote costs", err)
	}
}
