package server

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/testutil"
)

func TestMemberLifecyclePermissionsReplayAndAudit(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	s := Service{DB: db}
	owner, friend, other, unverified, id := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{owner, friend, other, unverified} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'test-only','Player',CASE WHEN $1::uuid<>$3::uuid THEN now() END)", u, u.String()+"@example.com", unverified); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','World','1.21.1','PAPER',2048,10240,100,2,0)`, id, owner); err != nil {
		t.Fatal(err)
	}
	in := MemberChange{Email: friend.String() + "@example.com", Role: Moderator}
	key := uuid.New()
	if _, err := s.ChangeMember(ctx, other, id, key, uuid.Nil, "add", in, "test"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.ListMembers(ctx, other, id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	bad := in
	bad.Role = Owner
	if _, err := s.ChangeMember(ctx, owner, id, uuid.New(), uuid.Nil, "add", bad, "test"); !errors.Is(err, ErrInvalid) {
		t.Fatal("ownership transfer", err)
	}
	bad = in
	bad.Email = unverified.String() + "@example.com"
	if _, err := s.ChangeMember(ctx, owner, id, uuid.New(), uuid.Nil, "add", bad, "test"); !errors.Is(err, ErrMemberUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.ChangeMember(ctx, owner, id, key, uuid.Nil, "add", in, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeMember(ctx, owner, id, key, uuid.Nil, "add", in, "test"); err != nil {
		t.Fatal("add replay", err)
	}
	if err := s.Authorize(ctx, friend, id, Console); err != nil {
		t.Fatal(err)
	}
	if err := s.Authorize(ctx, friend, id, Files); !errors.Is(err, ErrNotFound) {
		t.Fatal("moderator file access", err)
	}
	if _, err := s.ListMembers(ctx, friend, id); !errors.Is(err, ErrNotFound) {
		t.Fatal("member enumerated accounts", err)
	}
	bad = in
	bad.Role = Admin
	if _, err := s.ChangeMember(ctx, owner, id, key, uuid.Nil, "add", bad, "test"); !errors.Is(err, ErrMemberConflict) {
		t.Fatal("key reused", err)
	}
	if _, err := s.ChangeMember(ctx, owner, id, uuid.New(), uuid.Nil, "add", in, "test"); !errors.Is(err, ErrMemberConflict) {
		t.Fatal("duplicate add", err)
	}
	update := uuid.New()
	if _, err := s.ChangeMember(ctx, owner, id, update, friend, "update", MemberChange{Role: Admin}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Authorize(ctx, friend, id, Files); err != nil {
		t.Fatal(err)
	}
	for _, p := range []Permission{Members, Billing, AcceptEULA, Delete} {
		if err := s.Authorize(ctx, friend, id, p); !errors.Is(err, ErrNotFound) {
			t.Fatal("admin owner permission", p, err)
		}
	}
	if _, err := s.ChangeMember(ctx, friend, id, uuid.New(), friend, "update", MemberChange{Role: Admin}, "test"); !errors.Is(err, ErrNotFound) {
		t.Fatal("self escalation", err)
	}
	if _, err := s.ChangeMember(ctx, owner, id, uuid.New(), owner, "remove", MemberChange{Confirmation: owner.String()}, "test"); !errors.Is(err, ErrInvalid) {
		t.Fatal("owner removed", err)
	}
	if _, err := s.ChangeMember(ctx, owner, id, uuid.New(), friend, "remove", MemberChange{Confirmation: "wrong"}, "test"); !errors.Is(err, ErrInvalid) {
		t.Fatal("unconfirmed removal", err)
	}
	remove := uuid.New()
	if _, err := s.ChangeMember(ctx, owner, id, remove, friend, "remove", MemberChange{Confirmation: friend.String()}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeMember(ctx, owner, id, remove, friend, "remove", MemberChange{Confirmation: friend.String()}, "test"); err != nil {
		t.Fatal("remove replay", err)
	}
	if _, err := s.ChangeMember(ctx, owner, id, update, friend, "update", MemberChange{Role: Admin}, "test"); err != nil {
		t.Fatal("update replay after removal", err)
	}
	if err := s.Authorize(ctx, friend, id, View); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked access remained", err)
	}
	members, err := s.ListMembers(ctx, owner, id)
	if err != nil || len(members) != 1 || members[0].Role != Owner {
		t.Fatal(members, err)
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE server_id=$1 AND action LIKE 'member.%'", id).Scan(&count); err != nil || count != 3 {
		t.Fatal("audit/replay", count, err)
	}
	// A file/power operation already holding the common lock prevents role changes.
	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var held bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", "server:"+id.String()).Scan(&held); err != nil || !held {
		t.Fatal(err)
	}
	if _, err = s.ChangeMember(ctx, owner, id, uuid.New(), uuid.Nil, "add", in, "test"); !errors.Is(err, ErrMemberConflict) {
		t.Fatal("server lock ignored", err)
	}
	var released bool
	conn.QueryRow(ctx, "SELECT pg_advisory_unlock(hashtextextended($1,0))", "server:"+id.String()).Scan(&released)
}

func TestMemberLimitConcurrentAdds(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	s := Service{DB: db}
	owner, id := uuid.New(), uuid.New()
	if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'test-only','Owner',now())", owner, owner.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,name,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','World','1.21.1','PAPER',2048,10240,100,2,0)`, id, owner); err != nil {
		t.Fatal(err)
	}
	emails := []string{}
	for i := 0; i < MaxMembers+1; i++ {
		u := uuid.New()
		email := u.String() + "@example.com"
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'test-only','Friend',now())", u, email); err != nil {
			t.Fatal(err)
		}
		if i < MaxMembers-1 {
			if _, err := db.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,'MODERATOR')", id, u); err != nil {
				t.Fatal(err)
			}
		} else {
			emails = append(emails, email)
		}
	}
	outcomes := make(chan error, 2)
	for _, email := range emails {
		go func(email string) {
			_, err := s.ChangeMember(ctx, owner, id, uuid.New(), uuid.Nil, "add", MemberChange{Email: email, Role: Moderator}, "test")
			outcomes <- err
		}(email)
	}
	for range emails {
		err := <-outcomes
		if err != nil && !errors.Is(err, ErrMemberConflict) {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM server_members WHERE server_id=$1", id).Scan(&count); err != nil || count > MaxMembers {
		t.Fatal("member limit exceeded", count, err)
	}
}
