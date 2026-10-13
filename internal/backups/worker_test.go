package backups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/pterodactyl"
)

// Only the external Panel boundary is replaced; authorization, claims, intents,
// retention and audit writes execute against PostgreSQL.
type backupPanel struct {
	pterodactyl.ManagementClient
	mu                  sync.Mutex
	items               []pterodactyl.Backup
	creates, deletes    int
	uncertain           bool
	unavailable         bool
	onList              func()
	listCalls, onListAt int
	rejectAutoDelete    bool
}

func (p *backupPanel) Ready(context.Context, string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unavailable {
		return errors.New("unavailable")
	}
	return nil
}
func (p *backupPanel) ListBackups(context.Context, string) ([]pterodactyl.Backup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listCalls++
	if p.onList != nil && (p.onListAt <= 1 || p.listCalls == p.onListAt) {
		fn := p.onList
		p.onList = nil
		fn()
	}
	return append([]pterodactyl.Backup(nil), p.items...), nil
}
func (p *backupPanel) CreateBackup(_ context.Context, _ string, name string) (pterodactyl.Backup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++
	b := pterodactyl.Backup{UUID: uuid.NewString(), Name: name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	p.items = append(p.items, b)
	if p.uncertain {
		return pterodactyl.Backup{}, errors.New("response lost")
	}
	return b, nil
}
func (p *backupPanel) DeleteBackup(_ context.Context, _ string, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deletes++
	for i, b := range p.items {
		if b.UUID == id {
			p.items = append(p.items[:i], p.items[i+1:]...)
			return nil
		}
	}
	return pterodactyl.ErrNotFound
}
func due(t *testing.T, f fixture) time.Time {
	t.Helper()
	ctx := context.Background()
	_, err := f.s.Put(ctx, f.owner, f.id, Update{Enabled: true, Hour: 3, Timezone: "UTC"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err = f.db.Exec(ctx, "UPDATE backup_schedules SET next_run=$2 WHERE server_id=$1", f.id, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	return now
}
func worker(f fixture, p *backupPanel) *Worker {
	return &Worker{DB: f.db, Control: f.s.Control, Panel: p, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}
func TestWorkerClaimsAndCompletion(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Tick(ctx, now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	p.mu.Lock()
	count := p.creates
	p.mu.Unlock()
	if count != 1 {
		t.Fatalf("duplicate create %d", count)
	}
	v, err := f.s.Get(ctx, f.owner, f.id)
	if err != nil || len(v.Runs) != 1 || v.Runs[0].Status != "CREATING" || v.LastSuccess != nil || v.NextRun == nil || !v.NextRun.After(now) {
		t.Fatalf("accepted not successful: %+v %v", v, err)
	}
	completed := now.Add(time.Minute).Format(time.RFC3339)
	p.mu.Lock()
	p.items[0].CompletedAt = &completed
	p.items[0].Successful = true
	p.mu.Unlock()
	if err = w.Tick(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	v, err = f.s.Get(ctx, f.owner, f.id)
	if err != nil || v.LastSuccess == nil || v.Runs[0].Status != "COMPLETED" {
		t.Fatalf("completion: %+v %v", v, err)
	}
	var auditCount int
	if err = f.db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE server_id=$1 AND metadata->>'automation'='true'", f.id).Scan(&auditCount); err != nil || auditCount < 2 {
		t.Fatalf("audits %d %v", auditCount, err)
	}
}
func TestWorkerAmbiguousCreateNeverRetries(t *testing.T) {
	f := setup(t)
	p := &backupPanel{uncertain: true}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := w.Tick(ctx, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if p.creates != 1 {
		t.Fatalf("retried unknown create: %d", p.creates)
	}
	v, err := f.s.Get(ctx, f.owner, f.id)
	if err != nil || v.Warning != "needs_inspection" {
		t.Fatalf("unknown invisible: %+v %v", v, err)
	}
}
func TestWorkerFullManualAndLockedNeverRotated(t *testing.T) {
	f := setup(t)
	done := "2026-10-01T00:00:00Z"
	p := &backupPanel{items: []pterodactyl.Backup{{UUID: uuid.NewString(), Name: "Automatic fake", Successful: true, CompletedAt: &done}, {UUID: uuid.NewString(), Successful: true, CompletedAt: &done, Locked: true}}}
	now := due(t, f)
	if err := worker(f, p).Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if p.creates != 0 || p.deletes != 0 {
		t.Fatal("manual or locked backup changed")
	}
	v, _ := f.s.Get(context.Background(), f.owner, f.id)
	if len(v.Runs) != 1 || v.Runs[0].Status != "SKIPPED" || v.Warning != "capacity_full" {
		t.Fatalf("missing capacity warning %+v", v)
	}
}
func TestWorkerDisabledOrSuspendedDoesNotCreate(t *testing.T) {
	for _, mode := range []string{"disabled", "suspended"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			now := due(t, f)
			p := &backupPanel{}
			ctx := context.Background()
			if mode == "disabled" {
				if _, err := f.s.Put(ctx, f.owner, f.id, Update{Hour: 3, Timezone: "UTC", Revision: 1}, "test"); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.db.Exec(ctx, "UPDATE servers SET status='SUSPENDED' WHERE id=$1", f.id); err != nil {
					t.Fatal(err)
				}
			}
			if err := worker(f, p).Tick(ctx, now); err != nil {
				t.Fatal(err)
			}
			if p.creates != 0 || p.deletes != 0 {
				t.Fatal("ineligible mutation")
			}
		})
	}
}

func TestWorkerResumeDurableIntentDoesNotRepeatCreate(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.enqueue(ctx, now); err != nil {
		t.Fatal(err)
	}
	var run uuid.UUID
	if err := f.db.QueryRow(ctx, "SELECT id FROM backup_runs WHERE server_id=$1", f.id).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx, "INSERT INTO backup_intents(id,run_id,action,status) VALUES($1,$2,'CREATE','INTENDED')", uuid.New(), run); err != nil {
		t.Fatal(err)
	}
	if err := w.Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if p.creates != 0 {
		t.Fatal("retried post-crash create")
	}
	v, _ := f.s.Get(ctx, f.owner, f.id)
	if v.Runs[0].Status != "NEEDS_INSPECTION" {
		t.Fatalf("%+v", v)
	}
}
func TestWorkerLeaseFencing(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.enqueue(ctx, now); err != nil {
		t.Fatal(err)
	}
	old, err := w.claim(ctx, now, []uuid.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := w.claim(ctx, now.Add(91*time.Second), []uuid.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := f.db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if err = w.transition(ctx, conn, old, "FAILED", "stale", nil); !errors.Is(err, errLease) {
		t.Fatalf("old lease wrote: %v", err)
	}
	if err = w.transition(ctx, conn, newer, "SKIPPED", "test", nil); err != nil {
		t.Fatal(err)
	}
	w.release(old)
	var token uuid.UUID
	if err = f.db.QueryRow(ctx, "SELECT lease_token FROM backup_runs WHERE id=$1", old.ID).Scan(&token); err != nil || token != newer.Token {
		t.Fatalf("stale release removed new lease: %v %v", token, err)
	}
}
func TestWorkerDisableStillMonitorsFailedCompletion(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	last := now.Add(-time.Hour)
	if _, err := f.db.Exec(ctx, "UPDATE backup_schedules SET last_success=$2 WHERE server_id=$1", f.id, last); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Put(ctx, f.owner, f.id, Update{Hour: 3, Timezone: "UTC", Revision: 1}, "test"); err != nil {
		t.Fatal(err)
	}
	done := now.Add(time.Minute).Format(time.RFC3339)
	p.items[0].CompletedAt = &done
	p.items[0].Successful = false
	if err := w.Tick(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	v, _ := f.s.Get(ctx, f.owner, f.id)
	if v.Runs[0].Status != "FAILED" || v.LastSuccess == nil || !v.LastSuccess.Equal(last) {
		t.Fatalf("lost earlier success %+v", v)
	}
}

func TestWorkerRejectionStopsMutationWorkflow(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.enqueue(ctx, now); err != nil {
		t.Fatal(err)
	}
	r, err := w.claim(ctx, now, []uuid.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := f.db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	op, err := w.intent(ctx, conn, r, "CREATE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.rejectIntent(ctx, conn, r, op, "schedule_changed"); err == nil {
		t.Fatal("a rejected step allowed the mutation workflow to continue")
	}
}

func TestWorkerAndManualCreationShareAllowance(t *testing.T) {
	f := setup(t)
	done := "2026-10-01T00:00:00Z"
	p := &backupPanel{items: []pterodactyl.Backup{{UUID: uuid.NewString(), Successful: true, CompletedAt: &done}}}
	now := due(t, f)
	ctx := context.Background()
	manager := &management.Service{DB: f.db, Control: f.s.Control, Panel: p}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := worker(f, p).Tick(ctx, now); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := manager.NewBackup(ctx, f.owner, f.id, uuid.New(), "Manual", "test")
		if err != nil && !errors.Is(err, management.ErrLimit) && !errors.Is(err, control.ErrBusy) {
			t.Error(err)
		}
	}()
	wg.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.items) > 2 || p.creates != 1 {
		t.Fatalf("allowance race: %d backups, %d creates", len(p.items), p.creates)
	}
}
func TestWorkerRotatesOnlyOlderRecordedBackup(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	now := due(t, f)
	last := now.Add(-time.Hour).Format(time.RFC3339)
	older := now.Add(-48 * time.Hour).Format(time.RFC3339)
	oldID, lastID := uuid.New(), uuid.New()
	p := &backupPanel{items: []pterodactyl.Backup{{UUID: oldID.String(), Successful: true, CompletedAt: &older}, {UUID: lastID.String(), Successful: true, CompletedAt: &last}}}
	for i, b := range []uuid.UUID{oldID, lastID} {
		if _, err := f.db.Exec(ctx, `INSERT INTO backup_runs(id,server_id,schedule_revision,scheduled_at,owner_id,panel_identifier,status,backup_uuid) VALUES($1,$2,1,$3,$4,'abcdef12','COMPLETED',$5)`, uuid.New(), f.id, now.Add(time.Duration(-72+i)*time.Hour), f.owner, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := worker(f, p).Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if p.deletes != 1 || p.creates != 1 || len(p.items) != 2 {
		t.Fatalf("rotation: %+v", p)
	}
	for _, b := range p.items {
		if b.UUID == oldID.String() {
			t.Fatal("old not rotated")
		}
	}
	found := false
	for _, b := range p.items {
		found = found || b.UUID == lastID.String()
	}
	if !found {
		t.Fatal("lost last successful copy")
	}
}
func TestWorkerUnavailableRetriesAreBounded(t *testing.T) {
	f := setup(t)
	p := &backupPanel{unavailable: true}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if err := w.Tick(ctx, now.Add(time.Duration(i)*10*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	v, _ := f.s.Get(ctx, f.owner, f.id)
	if v.Runs[0].Status != "FAILED" || p.creates != 0 {
		t.Fatalf("unbounded retry: %+v", v)
	}
}

func TestWorkerUnknownDeleteBlocksFurtherMutation(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.enqueue(ctx, now); err != nil {
		t.Fatal(err)
	}
	r, err := w.claim(ctx, now, []uuid.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := f.db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	target := uuid.New()
	op, err := w.intent(ctx, conn, r, "DELETE", &target)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.unknown(ctx, conn, r, op, "delete_unconfirmed"); !errors.Is(err, control.ErrUnknown) {
		t.Fatalf("workflow continued after uncertain delete: %v", err)
	}
	if err = w.Tick(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if p.creates != 0 || p.deletes != 0 {
		t.Fatal("retried ambiguous mutation")
	}
}

func TestWorkerRechecksAllowanceImmediatelyBeforeCreate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.db.Exec(ctx, "UPDATE servers SET backup_limit=3 WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	p := &backupPanel{items: []pterodactyl.Backup{{UUID: uuid.NewString()}, {UUID: uuid.NewString()}}, onList: func() {
		if _, err := f.db.Exec(ctx, "UPDATE servers SET backup_limit=2 WHERE id=$1", f.id); err != nil {
			t.Error(err)
		}
	}}
	now := due(t, f)
	if err := worker(f, p).Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if p.creates != 0 {
		t.Fatal("creation exceeded allowance reduced after capacity snapshot")
	}
}

func TestEnqueueAuditsOnlyOneQueuedRun(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := w.enqueue(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := f.db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE server_id=$1 AND action='backup.automatic.QUEUED' AND metadata->>'automation'='true'", f.id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("queued transition audits %d want 1", count)
	}
}

func TestClaimDoesNotStarveNewRunsBehindUnfinishedBackups(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.enqueue(ctx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx, "UPDATE backup_runs SET status='CREATING',backup_uuid=$2 WHERE server_id=$1", f.id, uuid.New()); err != nil {
		t.Fatal(err)
	}
	queued := uuid.New()
	if _, err := f.db.Exec(ctx, `INSERT INTO backup_runs(id,server_id,schedule_revision,scheduled_at,owner_id,panel_identifier,status) VALUES($1,$2,1,$3,$4,'abcdef12','QUEUED')`, queued, f.id, now.Add(-time.Hour), f.owner); err != nil {
		t.Fatal(err)
	}
	first, err := w.claim(ctx, now, []uuid.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == queued {
		t.Fatal("fixture did not claim older unfinished backup")
	}
	w.release(first)
	next, err := w.claim(ctx, time.Now(), []uuid.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if next.ID != queued {
		t.Fatal("unfinished backup monopolized claims ahead of queued work")
	}
}

func (p *backupPanel) CreateAutomaticBackup(ctx context.Context, id, name string, preflight func(context.Context) error) (pterodactyl.Backup, error) {
	if err := preflight(ctx); err != nil {
		return pterodactyl.Backup{}, pterodactyl.ErrBackupPreflight
	}
	return p.CreateBackup(ctx, id, name)
}
func (p *backupPanel) DeleteAutomaticBackup(ctx context.Context, id, target string, preflight func(context.Context) error) error {
	if err := preflight(ctx); err != nil {
		return pterodactyl.ErrBackupPreflight
	}
	p.mu.Lock()
	reject := p.rejectAutoDelete
	p.mu.Unlock()
	if reject {
		return pterodactyl.ErrConflict
	}
	return p.DeleteBackup(ctx, id, target)
}

func TestWorkerAutomaticDeleteConflictIsKnownRejection(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	now := due(t, f)
	old, last := uuid.New(), uuid.New()
	older := now.Add(-48 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	p := &backupPanel{rejectAutoDelete: true, items: []pterodactyl.Backup{{UUID: old.String(), Successful: true, CompletedAt: &older}, {UUID: last.String(), Successful: true, CompletedAt: &recent}}}
	for i, b := range []uuid.UUID{old, last} {
		if _, err := f.db.Exec(ctx, `INSERT INTO backup_runs(id,server_id,schedule_revision,scheduled_at,owner_id,panel_identifier,status,backup_uuid) VALUES($1,$2,1,$3,$4,'abcdef12','COMPLETED',$5)`, uuid.New(), f.id, now.Add(time.Duration(-72+i)*time.Hour), f.owner, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := worker(f, p).Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	v, err := f.s.Get(ctx, f.owner, f.id)
	if err != nil || v.Runs[0].Status != "SKIPPED" || v.Warning == "needs_inspection" {
		t.Fatalf("known no-mutation rejection became ambiguous: %+v %v", v, err)
	}
}

func TestLifecycleJobQueuedDuringRotationPreflightPreventsDeletion(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	now := due(t, f)
	if _, err := f.db.Exec(ctx, "UPDATE users SET is_admin=true WHERE id=$1", f.owner); err != nil {
		t.Fatal(err)
	}
	old, last := uuid.New(), uuid.New()
	older := now.Add(-48 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	p := &backupPanel{onListAt: 2, items: []pterodactyl.Backup{{UUID: old.String(), Successful: true, CompletedAt: &older}, {UUID: last.String(), Successful: true, CompletedAt: &recent}}}
	for i, b := range []uuid.UUID{old, last} {
		if _, err := f.db.Exec(ctx, `INSERT INTO backup_runs(id,server_id,schedule_revision,scheduled_at,owner_id,panel_identifier,status,backup_uuid) VALUES($1,$2,1,$3,$4,'abcdef12','COMPLETED',$5)`, uuid.New(), f.id, now.Add(time.Duration(-72+i)*time.Hour), f.owner, b); err != nil {
			t.Fatal(err)
		}
	}
	p.onList = func() {
		if _, err := (jobs.Service{DB: f.db}).Enqueue(ctx, f.owner, f.id, uuid.New(), jobs.Suspend, jobs.Input{Reason: "Suspend during backup preflight"}, "test"); err != nil {
			t.Error(err)
		}
	}
	if err := worker(f, p).Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if p.deletes != 0 || p.creates != 0 {
		t.Fatalf("mutation after real suspension was queued: deletes=%d creates=%d", p.deletes, p.creates)
	}
}

func TestStaleLeaseCannotModifyIntentOutcome(t *testing.T) {
	for _, action := range []string{"unknown", "rejected"} {
		t.Run(action, func(t *testing.T) {
			f := setup(t)
			p := &backupPanel{}
			now := due(t, f)
			w := worker(f, p)
			ctx := context.Background()
			if err := w.enqueue(ctx, now); err != nil {
				t.Fatal(err)
			}
			old, err := w.claim(ctx, now, []uuid.UUID{})
			if err != nil {
				t.Fatal(err)
			}
			conn, err := f.db.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			op, err := w.intent(ctx, conn, old, "CREATE", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.claim(ctx, now.Add(91*time.Second), []uuid.UUID{}); err != nil {
				t.Fatal(err)
			}
			if action == "unknown" {
				err = w.unknown(ctx, conn, old, op, "create_unconfirmed")
			} else {
				err = w.rejectIntent(ctx, conn, old, op, "schedule_changed")
			}
			if !errors.Is(err, errLease) {
				t.Fatalf("stale lease not rejected: %v", err)
			}
			var status string
			if err = f.db.QueryRow(ctx, "SELECT status FROM backup_intents WHERE id=$1", op).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "INTENDED" {
				t.Fatalf("stale worker altered intent to %s", status)
			}
		})
	}
}
func TestRestartAfterKnownNonCreationDoesNotRequireInspection(t *testing.T) {
	f := setup(t)
	p := &backupPanel{}
	now := due(t, f)
	w := worker(f, p)
	ctx := context.Background()
	if err := w.enqueue(ctx, now); err != nil {
		t.Fatal(err)
	}
	var run uuid.UUID
	if err := f.db.QueryRow(ctx, "SELECT id FROM backup_runs WHERE server_id=$1", f.id).Scan(&run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx, "INSERT INTO backup_intents(id,run_id,action,status) VALUES($1,$2,'CREATE','CONFIRMED')", uuid.New(), run); err != nil {
		t.Fatal(err)
	}
	if err := w.Tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	v, err := f.s.Get(ctx, f.owner, f.id)
	if err != nil || v.Runs[0].Status != "SKIPPED" || v.Warning == "needs_inspection" {
		t.Fatalf("known rejected request blocked backups: %+v %v", v, err)
	}
	if p.creates != 0 {
		t.Fatal("known rejection should not be retried")
	}
}

func TestDisabledScheduleStillReconcilesRecordedDelete(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			now := due(t, f)
			p := &backupPanel{}
			w := worker(f, p)
			if err := w.enqueue(ctx, now); err != nil {
				t.Fatal(err)
			}
			var run uuid.UUID
			if err := f.db.QueryRow(ctx, "SELECT id FROM backup_runs WHERE server_id=$1", f.id).Scan(&run); err != nil {
				t.Fatal(err)
			}
			op, target := uuid.New(), uuid.New()
			if _, err := f.db.Exec(ctx, "INSERT INTO backup_intents(id,run_id,action,target_uuid,status) VALUES($1,$2,'DELETE',$3,'INTENDED')", op, run, target); err != nil {
				t.Fatal(err)
			}
			if present {
				p.items = []pterodactyl.Backup{{UUID: target.String(), Locked: true}}
			}
			if _, err := f.s.Put(ctx, f.owner, f.id, Update{Hour: 3, Timezone: "UTC", Revision: 1}, "test"); err != nil {
				t.Fatal(err)
			}
			if err := w.Tick(ctx, now); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := f.db.QueryRow(ctx, "SELECT status FROM backup_intents WHERE id=$1", op).Scan(&status); err != nil {
				t.Fatal(err)
			}
			want := "CONFIRMED"
			if present {
				want = "UNKNOWN"
			}
			if status != want {
				t.Fatalf("recorded delete abandoned after disable: %s want %s", status, want)
			}
			if p.creates != 0 || p.deletes != 0 {
				t.Fatal("disabled schedule mutated backups")
			}
		})
	}
}
