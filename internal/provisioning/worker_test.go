package provisioning

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

type faultPanel struct {
	*pterodactyl.Mock
	userCreates                                                                                     atomic.Int32
	loseUserResponse                                                                                atomic.Bool
	creates                                                                                         atomic.Int32
	loseResponse, creationNotSent, rejectCreate, installing, installFailed, wrongLimits, loseDelete atomic.Bool
}

func (p *faultPanel) CreateUser(ctx context.Context, r pterodactyl.UserRequest) (pterodactyl.User, error) {
	p.userCreates.Add(1)
	u, err := p.Mock.CreateUser(ctx, r)
	if err == nil && p.loseUserResponse.Swap(false) {
		return pterodactyl.User{}, fmt.Errorf("simulated lost user response")
	}
	return u, err
}
func (p *faultPanel) CreateServer(ctx context.Context, r pterodactyl.ServerRequest) (pterodactyl.Server, error) {
	p.creates.Add(1)
	if p.creationNotSent.Load() {
		return pterodactyl.Server{}, fmt.Errorf("simulated network timeout")
	}
	if p.rejectCreate.Load() {
		return pterodactyl.Server{}, &pterodactyl.APIError{Status: 422}
	}
	s, err := p.Mock.CreateServer(ctx, r)
	if err != nil {
		return s, err
	}
	if p.loseResponse.Swap(false) {
		return pterodactyl.Server{}, fmt.Errorf("simulated lost response")
	}
	return p.adjust(s), nil
}
func (p *faultPanel) adjust(s pterodactyl.Server) pterodactyl.Server {
	if p.installing.Load() {
		s.Installed = false
	}
	if p.installFailed.Load() {
		s.Installed = false
		s.InstallFailed = true
	}
	if p.wrongLimits.Load() {
		s.Request.Limits.RAMMB++
	}
	return s
}
func (p *faultPanel) FindServerByExternalID(ctx context.Context, id uuid.UUID) (pterodactyl.Server, error) {
	s, err := p.Mock.FindServerByExternalID(ctx, id)
	return p.adjust(s), err
}
func (p *faultPanel) ReinstallServer(ctx context.Context, id int) error {
	p.installFailed.Store(false)
	return p.Mock.ReinstallServer(ctx, id)
}
func (p *faultPanel) DeleteServer(ctx context.Context, id int) error {
	err := p.Mock.DeleteServer(ctx, id)
	if err == nil && p.loseDelete.Swap(false) {
		return fmt.Errorf("simulated lost delete response")
	}
	return err
}

type fixture struct {
	db                 *pgxpool.Pool
	panel              *faultPanel
	worker             *Worker
	queue              jobs.Service
	owner, admin, node uuid.UUID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db := testutil.Database(t)
	ctx := context.Background()
	owner, admin, node := uuid.New(), uuid.New(), uuid.New()
	_, err := db.Exec(ctx, `INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,'player@example.com','unused','Player',now(),false),($2,'admin@example.com','unused','Admin',now(),true);
 `, owner, admin)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Dedicated',200,4096,20480)`, node)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 8; i++ {
		if _, err = db.Exec(ctx, "INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port) VALUES($1,$2,$3,'play.example.com',$4)", uuid.New(), node, i, 25564+i); err != nil {
			t.Fatal(err)
		}
	}
	if err = ConfigureSoftware(ctx, db, admin, SoftwareTemplate{ServerType: "PAPER", EggID: 1, Image: "java21", Startup: "minecraft", VersionVariable: "MINECRAFT_VERSION", Environment: map[string]string{"SERVER_JARFILE": "server.jar"}, Versions: []string{"1.21.1"}}); err != nil {
		t.Fatal(err)
	}
	panel := &faultPanel{Mock: pterodactyl.NewMock()}
	return &fixture{db: db, panel: panel, worker: &Worker{DB: db, Panel: panel, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, queue: jobs.Service{DB: db}, owner: owner, admin: admin, node: node}
}
func (f *fixture) create(t *testing.T) server.Server {
	t.Helper()
	s, err := (server.Service{DB: f.db}).Create(context.Background(), f.owner, server.CreateInput{PlanID: uuid.MustParse("10000000-0000-4000-8000-000000000001"), Name: "World", ServerType: "PAPER", MinecraftVersion: "1.21.1"}, "test-create")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func (f *fixture) enqueue(t *testing.T, s server.Server, action jobs.Action) jobs.Job {
	t.Helper()
	input := jobs.Input{Reason: "integration test"}
	if action == jobs.Terminate {
		input.Confirmation = s.ID.String()
	}
	j, err := f.queue.Enqueue(context.Background(), f.admin, s.ID, uuid.New(), action, input, "test-job")
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func (f *fixture) once(t *testing.T) {
	t.Helper()
	worked, err := f.worker.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("worker: %v worked=%v", err, worked)
	}
}
func (f *fixture) due(t *testing.T) {
	t.Helper()
	if _, err := f.db.Exec(context.Background(), "UPDATE jobs SET available_at=now() WHERE status IN ('PENDING','RUNNING')"); err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) state(t *testing.T, s server.Server, want server.State) {
	t.Helper()
	v, err := (server.Service{DB: f.db}).Get(context.Background(), f.owner, s.ID)
	if err != nil || v.Status != want {
		t.Fatalf("state=%s want=%s err=%v", v.Status, want, err)
	}
}
func (f *fixture) capacity(t *testing.T, ram, disk, cpu int) {
	t.Helper()
	var a, b, c int
	err := f.db.QueryRow(context.Background(), "SELECT allocated_ram_mb,allocated_disk_mb,allocated_cpu FROM nodes WHERE id=$1", f.node).Scan(&a, &b, &c)
	if err != nil || a != ram || b != disk || c != cpu {
		t.Fatalf("allocated=%d/%d/%d expected=%d/%d/%d err=%v", a, b, c, ram, disk, cpu, err)
	}
}
func TestProvisioningIdempotencyAndLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	s := f.create(t)
	key := uuid.New()
	input := jobs.Input{Reason: "complimentary server grant"}
	if _, err := f.queue.Enqueue(ctx, f.owner, s.ID, key, jobs.Provision, input, "forged"); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("customer authorized provisioning", err)
	}
	j, err := f.queue.Enqueue(ctx, f.admin, s.ID, key, jobs.Provision, input, "request-one")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := f.queue.Enqueue(ctx, f.admin, s.ID, key, jobs.Provision, input, "request-two")
	if err != nil || duplicate.ID != j.ID {
		t.Fatal("enqueue not idempotent", err)
	}
	if _, err = f.queue.Enqueue(ctx, f.admin, s.ID, key, jobs.Suspend, input, "request-three"); !errors.Is(err, jobs.ErrConflict) {
		t.Fatal("idempotency key reused with different action")
	}
	lease, err := f.queue.Claim(ctx)
	if err != nil || lease == nil {
		t.Fatal(err)
	}
	// Competing workers cannot enter while the job's session lock is held.
	if other, err := f.queue.Claim(ctx); err != nil || other != nil {
		if other != nil {
			other.Close()
		}
		t.Fatal("active job claimed twice", err)
	}
	lease.Close() // Simulated process exit before any side effect; RUNNING job is reclaimable.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.worker.RunOnce(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	f.state(t, s, server.Running)
	f.capacity(t, 2048, 10240, 100)
	if f.panel.creates.Load() != 1 {
		t.Fatal("duplicate Panel create")
	}
	remote, err := f.panel.FindServerByExternalID(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := f.panel.Resources(ctx, remote.Identifier)
	if err != nil || usage.State != "offline" {
		t.Fatal("provisioning started Minecraft")
	}
	f.enqueue(t, s, jobs.Suspend)
	f.once(t)
	f.state(t, s, server.Suspended)
	f.capacity(t, 2048, 10240, 100)
	f.enqueue(t, s, jobs.Unsuspend)
	f.once(t)
	f.state(t, s, server.Running)
	if _, err = f.queue.Enqueue(ctx, f.admin, s.ID, uuid.New(), jobs.Terminate, jobs.Input{Reason: "delete without confirmation"}, "request"); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("destructive confirmation not enforced")
	}
	f.panel.loseDelete.Store(true)
	f.enqueue(t, s, jobs.Terminate)
	f.once(t)
	f.state(t, s, server.Running)
	f.capacity(t, 2048, 10240, 100)
	f.due(t)
	f.once(t)
	f.state(t, s, server.Cancelled)
	f.capacity(t, 0, 0, 0)
	var reserved int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM node_allocations WHERE reserved_server_id=$1", s.ID).Scan(&reserved); err != nil || reserved != 0 {
		t.Fatal("allocation not released", err)
	}
	if _, err = f.queue.Get(ctx, f.owner, j.ID); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("customer can inspect privileged jobs")
	}
}
func TestLostCreateResponseReconciles(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	f.panel.loseResponse.Store(true)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Provisioning)
	f.capacity(t, 2048, 10240, 100)
	f.due(t)
	f.once(t)
	f.state(t, s, server.Running)
	f.capacity(t, 2048, 10240, 100)
	if f.panel.creates.Load() != 1 {
		t.Fatal("duplicate remote server after lost response")
	}
}
func TestAmbiguousCreationRetainsReservation(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	f.panel.creationNotSent.Store(true)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.due(t)
	f.once(t)
	f.state(t, s, server.Failed)
	f.capacity(t, 2048, 10240, 100)
	f.panel.creationNotSent.Store(false)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Failed)
	if f.panel.creates.Load() != 1 {
		t.Fatal("uncertain creation blindly retried")
	}
	_, err := f.queue.Enqueue(context.Background(), f.admin, s.ID, uuid.New(), jobs.Provision, jobs.Input{Reason: "operator verified no Panel request remains in flight", RecoveryConfirmation: "PANEL_OPERATIONS_FINISHED_AND_ABSENT"}, "recovery")
	if err != nil {
		t.Fatal(err)
	}
	f.once(t)
	f.state(t, s, server.Running)
	f.capacity(t, 2048, 10240, 100)
	if f.panel.creates.Load() != 2 {
		t.Fatal("explicit recovery did not issue one new request")
	}
}
func TestDefinitiveRejectionCanRetry(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	f.panel.rejectCreate.Store(true)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Failed)
	f.panel.rejectCreate.Store(false)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Running)
	f.capacity(t, 2048, 10240, 100)
}
func TestInstallationAndConfigurationSnapshot(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	f.panel.installing.Store(true)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Provisioning)
	if err := ConfigureSoftware(context.Background(), f.db, f.admin, SoftwareTemplate{ServerType: "PAPER", EggID: 99, Image: "different", Startup: "changed", VersionVariable: "VERSION", Versions: []string{"1.21.1"}}); err != nil {
		t.Fatal(err)
	}
	f.panel.installing.Store(false)
	f.due(t)
	f.once(t)
	f.state(t, s, server.Running)
	remote, err := f.panel.FindServerByExternalID(context.Background(), s.ID)
	if err != nil || remote.Request.EggID != 1 || remote.Request.Environment["MINECRAFT_VERSION"] != "1.21.1" {
		t.Fatal("in-flight recipe changed", err)
	}
}
func TestFailedInstallationRetry(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	f.panel.installFailed.Store(true)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Failed)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.due(t)
	f.once(t)
	f.state(t, s, server.Running)
	if f.panel.creates.Load() != 1 {
		t.Fatal("reinstall created a second server")
	}
}
func TestResourceMismatchIsRejected(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	f.panel.wrongLimits.Store(true)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Failed)
	f.capacity(t, 2048, 10240, 100)
}
func TestNoCapacityFailureIsConsistent(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	if _, err := f.db.Exec(context.Background(), "UPDATE nodes SET enabled=false"); err != nil {
		t.Fatal(err)
	}
	j := f.enqueue(t, s, jobs.Provision)
	if _, err := f.db.Exec(context.Background(), "UPDATE jobs SET max_attempts=1 WHERE id=$1", j.ID); err != nil {
		t.Fatal(err)
	}
	f.once(t)
	f.state(t, s, server.Failed)
	f.capacity(t, 0, 0, 0)
	if f.panel.creates.Load() != 0 {
		t.Fatal("provisioned without capacity")
	}
}

func TestConcurrentQueueReplay(t *testing.T) {
	f := setup(t)
	s := f.create(t)
	key := uuid.New()
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := f.queue.Enqueue(context.Background(), f.admin, s.ID, key, jobs.Provision, jobs.Input{Reason: "one audited grant"}, "request")
			if err != nil {
				t.Error(err)
				return
			}
			ids <- j.ID
		}()
	}
	wg.Wait()
	close(ids)
	var first uuid.UUID
	for id := range ids {
		if first == uuid.Nil {
			first = id
		}
		if id != first {
			t.Fatal("duplicate durable jobs")
		}
	}
	var count int
	if err := f.db.QueryRow(context.Background(), "SELECT count(*) FROM jobs WHERE server_id=$1", s.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("queue replay created extra jobs", err)
	}
}

func TestLostUserResponseAndConcurrentOwner(t *testing.T) {
	f := setup(t)
	a, b := f.create(t), f.create(t)
	f.panel.loseUserResponse.Store(true)
	f.enqueue(t, a, jobs.Provision)
	f.enqueue(t, b, jobs.Provision)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.worker.RunOnce(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for range 4 {
		f.due(t)
		if _, err := f.worker.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	f.state(t, a, server.Running)
	f.state(t, b, server.Running)
	f.capacity(t, 4096, 20480, 200)
	if f.panel.userCreates.Load() != 1 || f.panel.creates.Load() != 2 {
		t.Fatal("user or server creation duplicated")
	}
}
