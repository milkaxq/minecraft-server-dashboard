package plan_test

import (
	"context"
	"testing"

	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/testutil"
	"minecraft-hosting/migrations"
)

func TestDefaultGroupPlansHaveDistinctDatabasePrices(t *testing.T) {
	db := testutil.Database(t)
	plans, err := (plan.Service{DB: db}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		players int
		name    string
		price   int64
		ram     int
	}{
		{4, "Starter", 600, 2048}, {8, "Game Night", 900, 3072},
		{16, "Survival", 1200, 4096}, {30, "Pro", 2400, 8192},
	} {
		var selected *plan.Plan
		for i := range plans {
			p := &plans[i]
			if p.Currency == "USD" && p.BillingPeriod == "MONTHLY" && p.PlayerRecommendation >= want.players && (selected == nil || p.PriceCents < selected.PriceCents) {
				selected = p
			}
		}
		if selected == nil || selected.Name != want.name || selected.PriceCents != want.price || selected.RAMMB != want.ram {
			t.Errorf("group %d: got %+v", want.players, selected)
		}
	}
}

func TestGroupPricingMigrationPreservesCustomizedPlans(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `UPDATE plans SET player_recommendation=7,price_cents=750 WHERE name='Starter'; UPDATE plans SET price_cents=950 WHERE name='Game Night'`)
	if err != nil {
		t.Fatal(err)
	}
	body, err := migrations.Files.ReadFile("020_group_plan_pricing.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(body)); err != nil {
		t.Fatal(err)
	}
	var recommendation int
	var price int64
	if err = db.QueryRow(ctx, `SELECT player_recommendation,price_cents FROM plans WHERE name='Starter'`).Scan(&recommendation, &price); err != nil {
		t.Fatal(err)
	}
	if recommendation != 7 || price != 750 {
		t.Fatal("customized Starter plan was overwritten")
	}
	if err = db.QueryRow(ctx, `SELECT price_cents FROM plans WHERE name='Game Night'`).Scan(&price); err != nil {
		t.Fatal(err)
	}
	if price != 950 {
		t.Fatal("customized Game Night price was overwritten")
	}
}
