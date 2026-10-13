package provisioning

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

type voiceFault struct {
	*faultPanel
	calls         atomic.Int32
	lose, notSent bool
	onRead        func()
}

func (p *voiceFault) ServerAllocations(ctx context.Context, id int) ([]int, error) {
	values, err := p.faultPanel.ServerAllocations(ctx, id)
	if p.onRead != nil {
		callback := p.onRead
		p.onRead = nil
		callback()
	}
	return values, err
}
func (p *voiceFault) AssignAllocation(ctx context.Context, request pterodactyl.AdditionalAllocationRequest) error {
	p.calls.Add(1)
	if p.notSent {
		return fmt.Errorf("transport timeout")
	}
	err := p.faultPanel.AssignAllocation(ctx, request)
	if err == nil && p.lose {
		return fmt.Errorf("lost reply")
	}
	return err
}
func TestVoicePortReservationIdempotencyLimitsReconciliationAndRelease(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			s := f.create(t)
			f.enqueue(t, s, jobs.Provision)
			f.once(t)
			if _, err := f.db.Exec(ctx, "UPDATE node_allocations SET voice_enabled=true"); err != nil {
				t.Fatal(err)
			}
			panel := &voiceFault{faultPanel: f.panel, lose: lost}
			f.worker.Panel = panel
			key := uuid.New()
			input := jobs.Input{Reason: "prepare voice connection"}
			job, err := f.queue.Enqueue(ctx, f.owner, s.ID, key, jobs.VoicePort, input, "voice-test")
			if err != nil {
				t.Fatal(err)
			}
			again, err := f.queue.Enqueue(ctx, f.owner, s.ID, key, jobs.VoicePort, input, "replay")
			if err != nil || again.ID != job.ID {
				t.Fatal("enqueue not idempotent", err)
			}
			f.once(t)
			var status string
			if err = f.db.QueryRow(ctx, "SELECT status FROM jobs WHERE id=$1", job.ID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if lost {
				if status != "FAILED" {
					t.Fatal("ambiguous assignment not retained", status)
				}
				if _, err = f.queue.Enqueue(ctx, f.owner, s.ID, uuid.New(), jobs.VoicePort, input, "reconcile"); err != nil {
					t.Fatal(err)
				}
				f.once(t)
			} else if status != "SUCCEEDED" {
				t.Fatal("setup failed", status)
			}
			if panel.calls.Load() != 1 {
				t.Fatal("allocation assignment repeated", panel.calls.Load())
			}
			f.state(t, s, server.Running)
			f.capacity(t, 2048, 10240, 100)
			var extra, primary int
			if err = f.db.QueryRow(ctx, "SELECT a.pterodactyl_allocation_id,p.status FROM server_addon_ports p JOIN node_allocations a ON a.id=p.allocation_id WHERE p.server_id=$1", s.ID).Scan(&extra, &status); err != nil || status != "READY" {
				t.Fatal("port not ready", err, status)
			}
			if err = f.db.QueryRow(ctx, "SELECT pterodactyl_allocation_id FROM node_allocations WHERE reserved_server_id=$1", s.ID).Scan(&primary); err != nil || primary == extra {
				t.Fatal("game allocation changed", err)
			}
			remote, err := panel.FindServerByExternalID(ctx, s.ID)
			if err != nil || remote.Request.Limits.RAMMB != 2048 || remote.Request.AllocationID != primary {
				t.Fatal("plan or default allocation changed", err)
			}
			f.enqueue(t, s, jobs.Terminate)
			f.once(t)
			f.capacity(t, 0, 0, 0)
			var count int
			if err = f.db.QueryRow(ctx, "SELECT count(*) FROM server_addon_ports WHERE server_id=$1", s.ID).Scan(&count); err != nil || count != 0 {
				t.Fatal("extra allocation not released", err)
			}
		})
	}
}
func TestVoicePortUnknownAbsentIsNeverBlindlyAssignedAgain(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	s := f.create(t)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	if _, err := f.db.Exec(ctx, "UPDATE node_allocations SET voice_enabled=true"); err != nil {
		t.Fatal(err)
	}
	panel := &voiceFault{faultPanel: f.panel, notSent: true}
	f.worker.Panel = panel
	input := jobs.Input{Reason: "prepare voice connection"}
	for range 2 {
		if _, err := f.queue.Enqueue(ctx, f.owner, s.ID, uuid.New(), jobs.VoicePort, input, "uncertain"); err != nil {
			t.Fatal(err)
		}
		f.once(t)
	}
	if panel.calls.Load() != 1 {
		t.Fatal("uncertain mutation repeated", panel.calls.Load())
	}
	var status string
	if err := f.db.QueryRow(ctx, "SELECT status FROM server_addon_ports WHERE server_id=$1", s.ID).Scan(&status); err != nil || status != "FAILED" {
		t.Fatal("reservation lost", status, err)
	}
}
func TestVoicePortAuthorizationNoCapacityAndRevokedAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	s := f.create(t)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	input := jobs.Input{Reason: "prepare voice connection"}
	if _, err := f.queue.Enqueue(ctx, f.admin, s.ID, uuid.New(), jobs.VoicePort, input, "foreign"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("unrelated user allocated port", err)
	}
	j, err := f.queue.Enqueue(ctx, f.owner, s.ID, uuid.New(), jobs.VoicePort, input, "capacity")
	if err != nil {
		t.Fatal(err)
	}
	f.once(t)
	var status string
	if err = f.db.QueryRow(ctx, "SELECT status FROM jobs WHERE id=$1", j.ID).Scan(&status); err != nil || status != "FAILED" {
		t.Fatal("no capacity not surfaced", status, err)
	}
	if _, err = f.db.Exec(ctx, "UPDATE node_allocations SET voice_enabled=true"); err != nil {
		t.Fatal(err)
	}
	j, err = f.queue.Enqueue(ctx, f.owner, s.ID, uuid.New(), jobs.VoicePort, input, "revoked")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, "UPDATE servers SET owner_id=$2 WHERE id=$1", s.ID, f.admin); err != nil {
		t.Fatal(err)
	}
	panel := &voiceFault{faultPanel: f.panel}
	f.worker.Panel = panel
	f.once(t)
	if panel.calls.Load() != 0 {
		t.Fatal("revoked user mutated Panel")
	}
}

func TestVoicePortRechecksMembershipAfterNetworkReads(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	s := f.create(t)
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	if _, err := f.db.Exec(ctx, "UPDATE node_allocations SET voice_enabled=true"); err != nil {
		t.Fatal(err)
	}
	panel := &voiceFault{faultPanel: f.panel, onRead: func() {
		if _, err := f.db.Exec(ctx, "UPDATE servers SET owner_id=$2 WHERE id=$1", s.ID, f.admin); err != nil {
			t.Fatal(err)
		}
	}}
	f.worker.Panel = panel
	if _, err := f.queue.Enqueue(ctx, f.owner, s.ID, uuid.New(), jobs.VoicePort, jobs.Input{Reason: "prepare voice connection"}, "late-revoke"); err != nil {
		t.Fatal(err)
	}
	f.once(t)
	if panel.calls.Load() != 0 {
		t.Fatal("late revocation reached Panel mutation")
	}
	var attempted bool
	if err := f.db.QueryRow(ctx, "SELECT assignment_attempted FROM server_addon_ports WHERE server_id=$1", s.ID).Scan(&attempted); err != nil || attempted {
		t.Fatal("definitively unsent request marked ambiguous", attempted, err)
	}
}
