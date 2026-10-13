package feedback

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

func fixture(t *testing.T) (Service, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	db := testutil.Database(t)
	actor, other, admin := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{actor, other, admin} {
		if _, err := db.Exec(context.Background(), "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,'fixture','Tester',now(),$3)", id, id.String()+"@example.com", id == admin); err != nil {
			t.Fatal(err)
		}
	}
	return Service{DB: db}, actor, other, admin
}
func input() Input {
	return Input{Category: "BUG", Title: "Cannot join server", Message: "I clicked Start but Minecraft cannot connect."}
}
func TestReportOwnershipAdminResponseAndAuditPrivacy(t *testing.T) {
	s, actor, other, admin := fixture(t)
	ctx := context.Background()
	report, err := s.Create(ctx, actor, uuid.New(), input(), "create")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user  uuid.UUID
		admin bool
		count int
	}{{actor, false, 1}, {other, false, 0}, {admin, true, 1}, {admin, false, 0}} {
		list, e := s.List(ctx, tc.user, tc.admin, 0)
		if e != nil || len(list) != tc.count {
			t.Fatal("list isolation", e, len(list))
		}
	}
	if _, err = s.List(ctx, other, true, 0); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("non-admin list", err)
	}
	update := Update{Status: "REVIEWED", Response: "Try the address shown on Overview.", Revision: report.Revision}
	if _, err = s.Update(ctx, other, report.ID, update, "update"); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("non-admin response", err)
	}
	updated, err := s.Update(ctx, admin, report.ID, update, "update")
	if err != nil || updated.Revision != 2 || updated.Response != update.Response {
		t.Fatal(updated, err)
	}
	if _, err = s.Update(ctx, admin, report.ID, update, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale response overwrote", err)
	}
	list, err := s.List(ctx, actor, false, 0)
	if err != nil || list[0].Status != "REVIEWED" || list[0].Response != update.Response {
		t.Fatal("tester response", err)
	}
	var metadata string
	if err = s.DB.QueryRow(ctx, "SELECT string_agg(metadata::text,' ') FROM audit_logs WHERE action LIKE 'beta.feedback_%'").Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{report.Title, report.Message, updated.Response} {
		if strings.Contains(metadata, private) {
			t.Fatal("private content in audit")
		}
	}
	if _, err = s.DB.Exec(ctx, "UPDATE users SET is_admin=false WHERE id=$1", admin); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Update(ctx, admin, report.ID, Update{Status: "RESOLVED", Revision: 2}, "revoked"); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatal("stale admin privileges", err)
	}
}
func TestReportServerAccessAndInputValidation(t *testing.T) {
	s, actor, other, _ := fixture(t)
	ctx := context.Background()
	sid := uuid.New()
	if _, err := s.DB.Exec(ctx, "INSERT INTO servers(id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','World','1.21.1','PAPER',2048,10240,100,2,0)", sid, actor); err != nil {
		t.Fatal(err)
	}
	in := input()
	in.ServerID = &sid
	if _, err := s.Create(ctx, other, uuid.New(), in, "denied"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,'MODERATOR')", sid, other); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, other, uuid.New(), in, "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(ctx, "DELETE FROM server_members WHERE server_id=$1 AND user_id=$2", sid, other); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, other, uuid.New(), in, "removed"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	for _, bad := range []Input{{Category: "ADMIN", Title: in.Title, Message: in.Message}, {Category: "BUG", Title: "ok", Message: in.Message}, {Category: "BUG", Title: strings.Repeat("x", 121), Message: in.Message}, {Category: "BUG", Title: in.Title, Message: "short"}, {Category: "BUG", Title: in.Title, Message: strings.Repeat("x", 4001)}, {Category: "BUG", Title: "hello\nworld", Message: in.Message}, {Category: "BUG", Title: in.Title, Message: "unsafe\x00content"}} {
		if _, err := s.Create(ctx, actor, uuid.New(), bad, "invalid"); !errors.Is(err, server.ErrInvalid) {
			t.Fatal("invalid input accepted", err)
		}
	}
	if _, err := s.Create(ctx, actor, uuid.Nil, input(), "invalid"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, actor, false, -1); !errors.Is(err, server.ErrInvalid) {
		t.Fatal(err)
	}
}
func TestReportConcurrentIdempotencyAndDailyLimit(t *testing.T) {
	s, actor, _, _ := fixture(t)
	ctx := context.Background()
	key := uuid.New()
	var wg sync.WaitGroup
	results := make(chan Report, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v, e := s.Create(ctx, actor, key, input(), "retry"); results <- v; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id uuid.UUID
	for r := range results {
		if id == uuid.Nil {
			id = r.ID
		}
		if r.ID != id {
			t.Fatal("duplicate report")
		}
	}
	changed := input()
	changed.Message = "Different report using the same submission key."
	if _, err := s.Create(ctx, actor, key, changed, "conflict"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	for i := 1; i < 19; i++ {
		if _, err := s.Create(ctx, actor, uuid.New(), input(), "quota"); err != nil {
			t.Fatal(err)
		}
	}
	// Two different reports compete for the last slot, not just the same key.
	quotaResults := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := s.Create(ctx, actor, uuid.New(), input(), "last slot")
			quotaResults <- err
		}()
	}
	winners, limited := 0, 0
	for i := 0; i < 2; i++ {
		err := <-quotaResults
		if err == nil {
			winners++
		} else if errors.Is(err, auth.ErrRateLimited) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if winners != 1 || limited != 1 {
		t.Fatal("concurrent daily quota bypassed", winners, limited)
	}
	if r, err := s.Create(ctx, actor, key, input(), "replay after quota"); err != nil || r.ID != id {
		t.Fatal("safe retry blocked", err)
	}
	var count int
	if err := s.DB.QueryRow(ctx, "SELECT count(*) FROM beta_feedback WHERE author_id=$1", actor).Scan(&count); err != nil || count != 20 {
		t.Fatal("unexpected count", count, err)
	}
}

func TestFeedbackContextPrivacyValidationAndIdempotency(t *testing.T) {
	s, actor, _, _ := fixture(t)
	ctx := context.Background()
	ref := uuid.New()
	in := input()
	in.RequestReference = &ref
	in.SourcePage = "/dashboard"
	key := uuid.New()
	report, err := s.Create(ctx, actor, key, in, "context")
	if err != nil || report.RequestReference == nil || *report.RequestReference != ref || report.SourcePage != "/dashboard" {
		t.Fatal(report, err)
	}
	if _, err = s.Create(ctx, actor, key, in, "retry"); err != nil {
		t.Fatal(err)
	}
	other := uuid.New()
	in.RequestReference = &other
	if _, err = s.Create(ctx, actor, key, in, "changed"); !errors.Is(err, ErrConflict) {
		t.Fatal("changed context accepted", err)
	}
	for _, path := range []string{"https://example.com?token=secret", "/dashboard?token=secret", "/reset-password/private-token", "/servers/nope", "/dashboard#secret"} {
		in.SourcePage = path
		if _, err = s.Create(ctx, actor, uuid.New(), in, "unsafe"); !errors.Is(err, server.ErrInvalid) {
			t.Fatal("unsafe source", path, err)
		}
	}
}
