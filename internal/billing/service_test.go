package billing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/provisioning"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

type fixture struct {
	db                        *pgxpool.Pool
	s                         *Service
	provider                  *MockProvider
	panel                     *pterodactyl.Mock
	worker                    *provisioning.Worker
	owner, other, admin, node uuid.UUID
	input                     server.CreateInput
}

// Billing tests simulate an already verified installation; provisioning tests
// exercise the real receipt and starter bundle checks.
type verifiedBillingInstall struct{}

func (verifiedBillingInstall) VerifyInstallation(context.Context, string, string, string, map[string]string) error {
	return nil
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	db := testutil.Database(t)
	f := fixture{db: db, owner: uuid.New(), other: uuid.New(), admin: uuid.New(), node: uuid.New(), input: server.CreateInput{PlanID: uuid.MustParse("10000000-0000-4000-8000-000000000001"), Name: "Our world", MinecraftVersion: "1.21.1", ServerType: "PAPER"}}
	for _, u := range []uuid.UUID{f.owner, f.other, f.admin} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,'unused','Player',now(),$3)", u, u.String()+"@example.com", u == f.admin); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',1000,20480,102400)", f.node); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 8; i++ {
		if _, err := db.Exec(ctx, "INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port) VALUES($1,$2,$3,'play.example.com',$4)", uuid.New(), f.node, i, 25564+i); err != nil {
			t.Fatal(err)
		}
	}
	if err := provisioning.ConfigureSoftware(ctx, db, f.admin, provisioning.SoftwareTemplate{ServerType: "PAPER", EggID: 1, Image: "java21", Startup: "minecraft", VersionVariable: "MINECRAFT_VERSION", Environment: map[string]string{"SERVER_JARFILE": "server.jar"}, Versions: []string{"1.21.1"}}); err != nil {
		t.Fatal(err)
	}
	f.provider, _ = NewMock("test-only-payment-secret-at-least-32-characters")
	f.s = &Service{DB: db, Provider: f.provider}
	f.panel = pterodactyl.NewMock()
	f.worker = &provisioning.Worker{DB: db, Panel: f.panel, Verifier: verifiedBillingInstall{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return f
}
func (f fixture) purchase(t *testing.T) Order {
	t.Helper()
	ctx := context.Background()
	o, err := f.s.Create(ctx, f.owner, uuid.New(), f.input, "order")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.CompleteMock(ctx, f.owner, o.ID, "payment"); err != nil {
		t.Fatal(err)
	}
	return o
}
func (f fixture) run(t *testing.T) {
	t.Helper()
	ok, err := f.worker.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatal("worker", ok, err)
	}
}
func TestConcurrentOrderAndWebhookReplay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	key := uuid.New()
	var wg sync.WaitGroup
	orders := make(chan Order, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, e := f.s.Create(ctx, f.owner, key, f.input, "test")
			if e != nil {
				errs <- e
			} else {
				orders <- o
			}
		}()
	}
	wg.Wait()
	close(orders)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var o Order
	for v := range orders {
		if o.ID != uuid.Nil && o.ID != v.ID {
			t.Fatal("duplicate order")
		}
		o = v
	}
	var count int
	f.db.QueryRow(ctx, "SELECT count(*) FROM servers WHERE owner_id=$1", f.owner).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate server", count)
	}
	changed := f.input
	changed.Name = "Changed"
	if _, err := f.s.Create(ctx, f.owner, key, changed, "test"); !errors.Is(err, ErrConflict) {
		t.Fatal("changed request reused key", err)
	}
	c, err := f.s.Checkout(ctx, f.owner, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, sig := f.provider.paidEvent(CheckoutRequest{OrderID: o.ID, AmountCents: o.PriceCents, Currency: o.Currency}, c.Reference)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := f.s.Webhook(ctx, data, sig, "webhook"); e != nil {
				errs := e
				t.Error(errs)
			}
		}()
	}
	wg.Wait()
	for _, table := range []string{"orders", "subscriptions", "jobs", "payment_events"} {
		if err = f.db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatal("duplicate", table, count)
		}
	}
	f.run(t)
	remote, err := f.panel.FindServerByExternalID(ctx, o.ServerID)
	if err != nil || remote.Request.Limits.RAMMB != 2048 || remote.Request.Limits.CPU != 100 {
		t.Fatal("plan enforcement", remote, err)
	}
	if err = f.s.Webhook(ctx, data, sig, "replay"); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.worker.RunOnce(ctx); ok || err != nil {
		t.Fatal("duplicate provisioning job", ok, err)
	}
}
func TestOwnershipSignaturesAmountAndCancellation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o, err := f.s.Create(ctx, f.owner, uuid.New(), f.input, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() error{func() error { _, e := f.s.Get(ctx, f.other, o.ID); return e }, func() error { _, e := f.s.Checkout(ctx, f.other, o.ID); return e }, func() error { return f.s.CompleteMock(ctx, f.other, o.ID, "test") }, func() error { return f.s.CancelOrder(ctx, f.other, o.ID, "test") }} {
		if err = check(); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("ownership bypass", err)
		}
	}
	c, err := f.s.Checkout(ctx, f.owner, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, sig := f.provider.paidEvent(CheckoutRequest{OrderID: o.ID, AmountCents: o.PriceCents + 1, Currency: o.Currency}, c.Reference)
	if err = f.s.Webhook(ctx, data, sig, "wrong-amount"); !errors.Is(err, ErrConflict) {
		t.Fatal("amount injection", err)
	}
	if err = f.s.Webhook(ctx, data, "bad", "forged"); !errors.Is(err, ErrSignature) {
		t.Fatal("unsigned payment accepted", err)
	}
	if err = f.s.CancelOrder(ctx, f.owner, o.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.CancelOrder(ctx, f.owner, o.ID, "replay"); err != nil {
		t.Fatal(err)
	}
	data, sig = f.provider.paidEvent(CheckoutRequest{OrderID: o.ID, AmountCents: o.PriceCents, Currency: o.Currency}, c.Reference)
	if err = f.s.Webhook(ctx, data, sig, "late"); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled order provisioned", err)
	}
	var jobsCount int
	f.db.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&jobsCount)
	if jobsCount != 0 {
		t.Fatal("rejected payment queued job")
	}
}
func TestExpiryRenewalAndFrozenPrice(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := f.purchase(t)
	f.run(t)
	// Paid terms do not follow later plan edits.
	f.db.Exec(ctx, "UPDATE plans SET price_cents=99999,ram_mb=8192,cpu_limit=300 WHERE id=$1", o.PlanID)
	f.db.Exec(ctx, "UPDATE subscriptions SET current_period_end=now()-interval '1 second' WHERE server_id=$1", o.ServerID)
	controls := control.Service{DB: f.db, Panel: f.panel}
	if err := controls.Power(ctx, f.owner, o.ServerID, uuid.New(), pterodactyl.PowerStart, "", "test"); !errors.Is(err, control.ErrState) {
		t.Fatal("expired customer started server", err)
	}
	if _, err := f.s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	remote, err := f.panel.FindServerByExternalID(ctx, o.ServerID)
	if err != nil || !remote.Suspended {
		t.Fatal("expiry did not suspend", err)
	}
	renewal, err := f.s.Renew(ctx, f.owner, o.ServerID, uuid.New(), "renew")
	if err != nil {
		t.Fatal(err)
	}
	if renewal.PriceCents != o.PriceCents {
		t.Fatal("renewal repriced", renewal.PriceCents, o.PriceCents)
	}
	if err = f.s.CompleteMock(ctx, f.owner, renewal.ID, "renew-payment"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	remote, err = f.panel.FindServerByExternalID(ctx, o.ServerID)
	if err != nil || remote.Suspended {
		t.Fatal("renewal did not unsuspend", err)
	}
	if remote.Request.Limits.RAMMB != 2048 {
		t.Fatal("plan edit changed resources")
	}
	list, err := f.s.Subscriptions(ctx, f.owner)
	if err != nil || len(list) != 1 || list[0].Status != "ACTIVE" || !list[0].CurrentPeriodEnd.After(time.Now()) {
		t.Fatal(list, err)
	}
	end := list[0].CurrentPeriodEnd
	if err = f.s.CompleteMock(ctx, f.owner, renewal.ID, "replay"); err != nil {
		t.Fatal(err)
	}
	list, _ = f.s.Subscriptions(ctx, f.owner)
	if !end.Equal(list[0].CurrentPeriodEnd) {
		t.Fatal("replay extended period twice")
	}
}
func TestAdminSuspensionSurvivesRenewalAndTerminationEndsSubscription(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := f.purchase(t)
	f.run(t)
	queue := jobs.Service{DB: f.db}
	if _, err := queue.Enqueue(ctx, f.admin, o.ServerID, uuid.New(), jobs.Suspend, jobs.Input{Reason: "manual abuse hold"}, "admin"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	renewal, err := f.s.Renew(ctx, f.owner, o.ServerID, uuid.New(), "renew")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.CompleteMock(ctx, f.owner, renewal.ID, "renew"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, e := f.worker.RunOnce(ctx); ok || e != nil {
		t.Fatal("renewal removed admin hold", ok, e)
	}
	remote, _ := f.panel.FindServerByExternalID(ctx, o.ServerID)
	if !remote.Suspended {
		t.Fatal("admin hold removed")
	}
	if _, err = queue.Enqueue(ctx, f.admin, o.ServerID, uuid.New(), jobs.Terminate, jobs.Input{Reason: "explicit termination", Confirmation: o.ServerID.String()}, "admin"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	subscriptions, e := f.s.Subscriptions(ctx, f.owner)
	if e != nil || subscriptions[0].Status != "CANCELLED" {
		t.Fatal(subscriptions, e)
	}
	if _, err = f.s.Renew(ctx, f.owner, o.ServerID, uuid.New(), "renew"); !errors.Is(err, ErrConflict) {
		t.Fatal("terminated server renewed", err)
	}
}
func TestRenewalWhileExpiryJobPendingAndCancellationAtPeriodEnd(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := f.purchase(t)
	f.run(t)
	if err := f.s.SetCancellation(ctx, f.owner, o.ServerID, true, "cancel"); err != nil {
		t.Fatal(err)
	}
	f.db.Exec(ctx, "UPDATE subscriptions SET current_period_end=now()-interval '1 second' WHERE server_id=$1", o.ServerID)
	if _, err := f.s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	subs, _ := f.s.Subscriptions(ctx, f.owner)
	if subs[0].Status != "CANCELLED" {
		t.Fatal("cancelled period still active")
	}
	renewal, err := f.s.Renew(ctx, f.owner, o.ServerID, uuid.New(), "renew")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.CompleteMock(ctx, f.owner, renewal.ID, "paid"); err != nil {
		t.Fatal(err)
	}
	f.run(t) // The already-queued suspension completes before reconciliation restores access.
	if _, err = f.s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	remote, _ := f.panel.FindServerByExternalID(ctx, o.ServerID)
	if remote.Suspended {
		t.Fatal("paid renewal remained suspended")
	}
}
func TestEventIDCannotBeReusedForAnotherOrder(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	first := f.purchase(t)
	other, err := f.s.Create(ctx, f.owner, uuid.New(), f.input, "second")
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := f.s.Checkout(ctx, f.owner, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := f.provider.paidEvent(CheckoutRequest{OrderID: other.ID, AmountCents: other.PriceCents, Currency: other.Currency}, checkout.Reference)
	var event PaymentEvent
	json.Unmarshal(data, &event)
	event.ID = "mock_paid_" + first.ID.String()
	data, _ = json.Marshal(event)
	if err = f.s.Webhook(ctx, data, f.provider.sign(data), "reused-event"); !errors.Is(err, ErrConflict) {
		t.Fatal("event reused for another order", err)
	}
	got, _ := f.s.Get(ctx, f.owner, other.ID)
	if got.Status != "PENDING" {
		t.Fatal("foreign event marked paid")
	}
}

func TestExpiryBatchDoesNotStarveBehindAlreadySuspendedSubscriptions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := f.purchase(t)
	f.run(t)
	// A large history of expired, already-suspended services must not consume every
	// slot in the next reconciliation batch and hide newly expired active services.
	_, err := f.db.Exec(ctx, `INSERT INTO servers(id,owner_id,node_id,plan_id,pterodactyl_server_id,pterodactyl_identifier,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) SELECT gen_random_uuid(),$1,$2,'10000000-0000-4000-8000-000000000001',1000+n,gen_random_uuid()::text,'Old world','SUSPENDED','1.21.1','PAPER',2048,10240,100,2,0 FROM generate_series(1,110) n`, f.owner, f.node)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.db.Exec(ctx, `INSERT INTO subscriptions(id,user_id,server_id,plan_id,status,current_period_end) SELECT gen_random_uuid(),owner_id,id,plan_id,'PAST_DUE',now()-interval '2 months' FROM servers WHERE name='Old world'`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, "UPDATE subscriptions SET current_period_end=now()-interval '1 second' WHERE server_id=$1", o.ServerID); err != nil {
		t.Fatal(err)
	}
	count, err := f.s.Reconcile(ctx)
	if err != nil || count != 1 {
		t.Fatal("expiry batch starved", count, err)
	}
	var action string
	if err = f.db.QueryRow(ctx, "SELECT action FROM jobs WHERE server_id=$1 AND status='PENDING'", o.ServerID).Scan(&action); err != nil || action != "SUSPEND" {
		t.Fatal("new expiry not scheduled", action, err)
	}
}
