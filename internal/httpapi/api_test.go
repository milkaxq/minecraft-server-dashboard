package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/database"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/server"
)

func TestStrictJSON(t *testing.T) {
	for _, body := range []string{`{"name":"world","ram_mb":999999}`, `{"name":"world"} {}`, `{"name":"world",`, strings.Repeat("x", 17000)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		var v server.CreateInput
		if decode(httptest.NewRecorder(), r, &v) == nil {
			t.Fatalf("accepted %s", body[:min(len(body), 80)])
		}
	}
}

type capturedMail struct{ token string }

func (m *capturedMail) SendVerification(_ context.Context, _ string, token string) error {
	m.token = token
	return nil
}

// Integration tests use a disposable schema in a dedicated PostgreSQL database.
func TestPostgresAPI(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal("migration not idempotent", err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO software_templates(server_type,egg_id,docker_image,startup,version_variable) VALUES('PAPER',1,'java','minecraft','MINECRAFT_VERSION'); INSERT INTO minecraft_versions(server_type,version) VALUES('PAPER','1.21.1')`); err != nil {
		t.Fatal(err)
	}
	mailer := &capturedMail{}
	authentication, err := auth.New(db, mailer)
	if err != nil {
		t.Fatal(err)
	}
	api := &API{DB: db, Auth: authentication, Plans: plan.Service{DB: db}, Servers: server.Service{DB: db}, Jobs: jobs.Service{DB: db}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Origin: "http://localhost:3000"}
	handler := api.Handler()
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var b bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&b).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &b)
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	must := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
		}
	}
	register := func(email string) (uuid.UUID, string) {
		t.Helper()
		w := call("POST", "/auth/register", "", map[string]string{"email": email, "password": "secure-test-password", "name": "Player"})
		must(w, 201)
		var u struct{ ID uuid.UUID }
		if err := json.Unmarshal(w.Body.Bytes(), &u); err != nil {
			t.Fatal(err)
		}
		must(call("POST", "/auth/login", "", map[string]string{"email": email, "password": "secure-test-password"}), 403)
		token := mailer.token
		must(call("POST", "/auth/verify-email", "", map[string]string{"token": token}), 204)
		must(call("POST", "/auth/verify-email", "", map[string]string{"token": token}), 400)
		w = call("POST", "/auth/login", "", map[string]string{"email": email, "password": "secure-test-password"})
		must(w, 200)
		var login auth.LoginResult
		if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
			t.Fatal(err)
		}
		return u.ID, login.Token
	}
	owner, ownerToken := register("owner@example.com")
	other, otherToken := register("other@example.com")
	must(call("POST", "/auth/register", "", map[string]any{"email": "admin@example.com", "password": "secure-test-password", "name": "Admin", "is_admin": true}), 400)
	must(call("GET", "/servers", "", nil), 401)
	must(call("GET", "/plans", "", nil), 200)
	must(call("GET", "/minecraft/options", "", nil), 200)
	input := map[string]any{"plan_id": "10000000-0000-4000-8000-000000000001", "name": "World", "minecraft_version": "1.21.1", "server_type": "PAPER"}
	input["ram_mb"] = 999999
	must(call("POST", "/servers", ownerToken, input), 400)
	delete(input, "ram_mb")
	input["minecraft_version"] = "1.99.9"
	must(call("POST", "/servers", ownerToken, input), 400)
	input["minecraft_version"] = "1.21.1"
	w := call("POST", "/servers", ownerToken, input)
	must(w, 201)
	var s server.Server
	if err = json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if s.OwnerID != owner || s.Status != server.PendingPayment || s.RAMMB != 2048 || s.DiskMB != 10240 || s.CPULimit != 100 {
		t.Fatalf("plan enforcement failed: %+v", s)
	}
	if strings.Contains(w.Body.String(), "pterodactyl") || strings.Contains(w.Body.String(), "internal_error") {
		t.Fatal("internal data exposed")
	}
	must(call("POST", "/admin/servers/"+s.ID.String()+"/provision", ownerToken, map[string]string{"reason": "forged grant"}), 403)
	must(call("GET", "/admin/jobs/"+uuid.NewString(), ownerToken, nil), 403)
	must(call("GET", "/servers/"+s.ID.String(), otherToken, nil), 404)
	must(call("GET", "/servers/"+uuid.NewString(), otherToken, nil), 404)
	w = call("GET", "/servers", otherToken, nil)
	must(w, 200)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("cross-user listing")
	}
	if _, err = db.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,'MODERATOR')", s.ID, other); err != nil {
		t.Fatal(err)
	}
	must(call("GET", "/servers/"+s.ID.String(), otherToken, nil), 200)
	if err = api.Servers.Authorize(ctx, other, s.ID, server.Restart); err != nil {
		t.Fatal(err)
	}
	if err = api.Servers.Authorize(ctx, other, s.ID, server.Files); err == nil {
		t.Fatal("moderator can edit files")
	}
	if _, err = db.Exec(ctx, "UPDATE plans SET enabled=false WHERE id=$1", s.PlanID); err != nil {
		t.Fatal(err)
	}
	must(call("POST", "/servers", ownerToken, input), 404)
	var auditCount int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE server_id=$1 AND action='server.created'", s.ID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("missing audit", err)
	}
	must(call("POST", "/auth/logout", ownerToken, nil), 204)
	must(call("GET", "/account", ownerToken, nil), 401)
	if _, err = db.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second' WHERE user_id=$1", other); err != nil {
		t.Fatal(err)
	}
	must(call("GET", "/account", otherToken, nil), 401)
	r := httptest.NewRequest("GET", "/plans", nil)
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	must(w, 403)
	bucket := "concurrent:" + uuid.NewString()
	errs := make(chan error, 10)
	for range 10 {
		go func() { errs <- authentication.RateLimit(ctx, bucket, 100) }()
	}
	for range 10 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var attempts int
	if err = db.QueryRow(ctx, "SELECT attempts FROM auth_rate_limits WHERE bucket=$1", bucket).Scan(&attempts); err != nil || attempts != 10 {
		t.Fatal("rate limiter lost updates", err)
	}
	for i := 0; i < 31; i++ {
		err = authentication.RateLimit(ctx, "limit-test", 30)
	}
	if err != auth.ErrRateLimited {
		t.Fatal("rate limit not enforced")
	}
	if err = authentication.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("ownership, resource enforcement, sessions, migrations and shared rate limits passed at %s", time.Now().UTC().Format(time.RFC3339))
}
