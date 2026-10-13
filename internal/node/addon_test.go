package node

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/testutil"
)

func TestConcurrentVoiceReservationsCannotConsumeSameOrGameAllocation(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, node := uuid.New(), uuid.New()
	if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name) VALUES($1,'voice-node@example.com','test','Test')", owner); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',400,8192,40960)", node); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := db.Exec(ctx, "INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port,voice_enabled) VALUES($1,$2,$3,'localhost',$4,true)", uuid.New(), node, i, 25564+i); err != nil {
			t.Fatal(err)
		}
	}
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, id := range ids {
		if _, err := db.Exec(ctx, "INSERT INTO servers(id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','Test','1.21.1','PAPER',2048,10240,100,2,0)", id, owner); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range ids[:2] {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Reserve(ctx, tx, id, 2048, 10240, 100); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for _, id := range ids[:2] {
		wg.Go(func() {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			defer tx.Rollback(ctx)
			reservation, _, err := ReserveVoicePort(ctx, tx, id)
			if errors.Is(err, ErrCapacity) {
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
			replay, _, err := ReserveVoicePort(ctx, tx, id)
			if err != nil || replay != reservation {
				t.Error("reservation not idempotent", err)
				return
			}
			if err = tx.Commit(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var count, overlap int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM server_addon_ports").Scan(&count); err != nil || count != 1 {
		t.Fatal("shared voice port", count, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM server_addon_ports p JOIN node_allocations a ON p.allocation_id=a.id WHERE a.reserved_server_id IS NOT NULL").Scan(&overlap); err != nil || overlap != 0 {
		t.Fatal("voice consumed a game port", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = Reserve(ctx, tx, ids[2], 2048, 10240, 100); !errors.Is(err, ErrCapacity) {
		t.Fatal("game reservation stole extra port", err)
	}
}

func TestEnableVoicePortsRequiresAdminConfirmationAndAtomicAvailability(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	admin, customer, node := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{admin, customer} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,'test','Test',now(),$3)", id, id.String()+"@example.com", id == admin); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',100,2048,10240)", node); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if _, err := db.Exec(ctx, "INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port) VALUES($1,$2,$3,'localhost',$4)", uuid.New(), node, i, 25564+i); err != nil {
			t.Fatal(err)
		}
	}
	if err := EnableVoicePorts(ctx, db, customer, node, []int{25565}, true); err == nil {
		t.Fatal("customer enabled ports")
	}
	if err := EnableVoicePorts(ctx, db, admin, node, []int{25565}, false); err == nil {
		t.Fatal("missing UDP readiness accepted")
	}
	if err := EnableVoicePorts(ctx, db, admin, node, []int{25565, 29999}, true); err == nil {
		t.Fatal("unknown port accepted")
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM node_allocations WHERE voice_enabled").Scan(&count); err != nil || count != 0 {
		t.Fatal("failed command partially enabled ports", count, err)
	}
	if err := EnableVoicePorts(ctx, db, admin, node, []int{25565, 25566}, true); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM node_allocations WHERE voice_enabled").Scan(&count); err != nil || count != 2 {
		t.Fatal("valid pool not enabled", count, err)
	}
}
