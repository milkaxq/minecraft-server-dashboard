package node

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/testutil"
)

func TestConcurrentReservationsAndRelease(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, node := uuid.New(), uuid.New()
	if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name) VALUES($1,'node-test@example.com','unused','Test')", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Dedicated',200,4096,20480)", node); err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{}
	for i := 1; i <= 20; i++ {
		id := uuid.New()
		ids = append(ids, id)
		if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','World','1.21.1','PAPER',2048,10240,100,2,0)`, id, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, "INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port) VALUES($1,$2,$3,'localhost',$4)", uuid.New(), node, i, 25564+i); err != nil {
			t.Fatal(err)
		}
	}
	reserve := func(id uuid.UUID) error {
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "SELECT id FROM servers WHERE id=$1 FOR UPDATE", id); err != nil {
			return err
		}
		a, err := Reserve(ctx, tx, id, 2048, 10240, 100)
		if err != nil {
			return err
		}
		b, err := Reserve(ctx, tx, id, 2048, 10240, 100)
		if err != nil {
			return err
		}
		if a != b {
			return ErrInvariant
		}
		return tx.Commit(ctx)
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			if err := reserve(id); err != nil && !errors.Is(err, ErrCapacity) {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	// SKIP LOCKED may defer contenders while the selected node is locked. Retry sequentially.
	for _, id := range ids {
		if err := reserve(id); err != nil && !errors.Is(err, ErrCapacity) {
			t.Fatal(err)
		}
	}
	var count, ram, disk, cpu int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM node_allocations WHERE reserved_server_id IS NOT NULL").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT allocated_ram_mb,allocated_disk_mb,allocated_cpu FROM nodes WHERE id=$1", node).Scan(&ram, &disk, &cpu); err != nil {
		t.Fatal(err)
	}
	if count != 2 || ram != 4096 || disk != 20480 || cpu != 200 {
		t.Fatalf("oversold or double-counted: %d %d %d %d", count, ram, disk, cpu)
	}
	for _, id := range ids {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = Release(ctx, tx, id); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = Release(ctx, tx, id); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(ctx, "SELECT allocated_ram_mb+allocated_disk_mb+allocated_cpu FROM nodes WHERE id=$1", node).Scan(&ram); err != nil || ram != 0 {
		t.Fatal("release not idempotent", err)
	}
}
