package pterodactyl

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestMockIdempotencyAndIsolation(t *testing.T) {
	ctx := context.Background()
	m := NewMock()
	u, err := m.CreateUser(ctx, UserRequest{ExternalID: uuid.New(), Email: "test@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	req := ServerRequest{ExternalID: uuid.New(), UserID: u.ID, NodeID: 1, AllocationID: 1, EggID: 1, Name: "world", Image: "java", Startup: "minecraft", Limits: Limits{RAMMB: 2048, DiskMB: 10240, CPU: 100}, Environment: map[string]string{"VERSION": "1.21.1"}}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := m.CreateServer(ctx, req)
			if err != nil || s.ID != 1 {
				t.Errorf("idempotency failed: %v", err)
			}
		}()
	}
	wg.Wait()
	s, err := m.FindServerByExternalID(ctx, req.ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	s.Request.Environment["VERSION"] = "mutated"
	req.Environment["VERSION"] = "mutated"
	s, err = m.GetServer(ctx, 1)
	if err != nil || s.Request.Environment["VERSION"] != "1.21.1" {
		t.Fatal("mock leaked mutable maps")
	}
	if _, err = m.CreateServer(ctx, req); !errors.Is(err, ErrConflict) {
		t.Fatal("conflicting retry accepted")
	}
	if err = m.SuspendServer(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err = m.Power(ctx, s.Identifier, PowerStart); !errors.Is(err, ErrConflict) {
		t.Fatal("suspended server started")
	}
	if err = m.DeleteServer(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteServer(ctx, 1); err != nil {
		t.Fatal("delete not idempotent")
	}
	if _, err = m.Resources(ctx, s.Identifier); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted resources still visible")
	}
}
