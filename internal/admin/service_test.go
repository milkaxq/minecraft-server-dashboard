package admin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/testutil"
)

func TestAdminPlanChangesAndSensitiveDataExclusion(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	actor, customer := uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{actor, customer} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,'private-password-hash','Person',now(),$3)", u, u.String()+"@example.com", u == actor); err != nil {
			t.Fatal(err)
		}
	}
	s := Service{DB: db}
	p := plan.Plan{Name: "Builders", Description: "For builders", PriceCents: 975, Currency: "USD", BillingPeriod: "MONTHLY", RAMMB: 2048, DiskMB: 10240, CPULimit: 100, BackupLimit: 2, PlayerRecommendation: 10, Enabled: true}
	if _, err := s.SavePlan(ctx, customer, uuid.Nil, p, "test"); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("customer edited plan", err)
	}
	p, err := s.SavePlan(ctx, actor, uuid.Nil, p, "create")
	if err != nil {
		t.Fatal(err)
	}
	p.Enabled = false
	p.PriceCents = 1250
	if _, err = s.SavePlan(ctx, actor, p.ID, p, "disable"); err != nil {
		t.Fatal(err)
	}
	plans, err := s.Plans(ctx, actor)
	if err != nil || len(plans) != 5 {
		t.Fatal(plans, err)
	}
	enabled, err := (plan.Service{DB: db}).List(ctx)
	if err != nil || len(enabled) != 4 {
		t.Fatal("disabled plan offered", enabled, err)
	}
	users, err := s.Users(ctx, actor, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(users)
	if strings.Contains(string(data), "password") || strings.Contains(string(data), "private") {
		t.Fatal("sensitive user data exposed")
	}
	audits, err := s.Audit(ctx, actor, 0)
	if err != nil || len(audits) != 2 {
		t.Fatal("missing admin audit", audits, err)
	}
	for _, check := range []func() error{func() error { _, e := s.Users(ctx, customer, 0); return e }, func() error { _, e := s.Servers(ctx, customer, 0); return e }, func() error { _, e := s.Orders(ctx, customer, 0); return e }, func() error { _, e := s.Nodes(ctx, customer); return e }, func() error { _, e := s.Audit(ctx, customer, 0); return e }, func() error { _, e := s.Plans(ctx, customer); return e }} {
		if err = check(); !errors.Is(err, jobs.ErrForbidden) {
			t.Fatal("admin list exposed", err)
		}
	}
	db.Exec(ctx, "UPDATE users SET is_admin=false WHERE id=$1", actor)
	if _, err = s.SavePlan(ctx, actor, p.ID, p, "revoked"); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("revoked administrator kept access", err)
	}
}
