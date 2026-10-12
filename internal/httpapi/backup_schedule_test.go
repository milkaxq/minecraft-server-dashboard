package httpapi

import (
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/backups"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

func TestBackupScheduleHTTP(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, other, id := uuid.New(), uuid.New(), uuid.New()
	tokens := map[uuid.UUID]string{owner: strings.Repeat("o", 43), other: strings.Repeat("x", 43)}
	for user, token := range tokens {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'unused','Player',now())", user, user.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(token))
		if _, err := db.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash[:], user); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001','World','FAILED','1.21.1','PAPER',2048,10240,100,2,0)`, id, owner); err != nil {
		t.Fatal(err)
	}
	authentication, err := auth.New(db, &capturedMail{})
	if err != nil {
		t.Fatal(err)
	}
	a := &API{DB: db, Auth: authentication, Servers: server.Service{DB: db}, BackupSchedules: &backups.Service{DB: db, Control: &control.Service{DB: db}}, Origin: "http://localhost:3000", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h := a.Handler()
	call := func(method, token, body string, cookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/servers/"+id.String()+"/backup-schedule", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie {
			r.AddCookie(&http.Cookie{Name: "blockyard_dev_session", Value: token})
		} else {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"enabled":false,"hour":3,"minute":0,"timezone":"UTC","revision":0}`
	if w := call("GET", tokens[other], "", false); w.Code != 404 {
		t.Fatalf("outsider: %d %s", w.Code, w.Body.String())
	}
	if w := call("PUT", tokens[owner], body, true); w.Code != 403 {
		t.Fatalf("CSRF: %d %s", w.Code, w.Body.String())
	}
	if w := call("PUT", tokens[owner], strings.TrimSuffix(body, "}")+`,"backup_limit":100}`, false); w.Code != 400 {
		t.Fatalf("forged fields: %d %s", w.Code, w.Body.String())
	}
	if w := call("PUT", tokens[owner], body, false); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	if w := call("PUT", tokens[owner], body, false); w.Code != 409 {
		t.Fatalf("stale revision: %d %s", w.Code, w.Body.String())
	}
}
