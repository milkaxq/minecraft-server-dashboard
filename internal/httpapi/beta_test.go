package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

func TestBetaCreationAndOwnerDeletion(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	operator, owner, stranger := uuid.New(), uuid.New(), uuid.New()
	tokens := map[uuid.UUID]string{operator: strings.Repeat("a", 43), owner: strings.Repeat("o", 43), stranger: strings.Repeat("x", 43)}
	for id, token := range tokens {
		_, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,'unused','Player',now(),$3)", id, id.String()+"@example.com", id == operator)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(token))
		if _, err = db.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash[:], id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO software_templates(server_type,egg_id,docker_image,startup,version_variable) VALUES('PAPER',1,'java21','minecraft','MINECRAFT_VERSION'); INSERT INTO minecraft_versions(server_type,version,docker_image) VALUES('PAPER','26.3','java25')`); err != nil {
		t.Fatal(err)
	}
	authentication, err := auth.New(db, &capturedMail{})
	if err != nil {
		t.Fatal(err)
	}
	a := &API{DB: db, Auth: authentication, Plans: plan.Service{DB: db}, Servers: server.Service{DB: db}, Jobs: jobs.Service{DB: db}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Origin: "http://localhost:3000"}
	handler := a.Handler()
	call := func(method, path string, user uuid.UUID, key uuid.UUID, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key.String())
		r.Header.Set("Authorization", "Bearer "+tokens[user])
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	input := `{"plan_id":"10000000-0000-4000-8000-000000000001","name":"Latest world","server_type":"PAPER","minecraft_version":"26.3"}`
	key := uuid.New()
	if w := call("POST", "/admin/beta-servers", owner, key, input); w.Code != 403 {
		t.Fatal("non-admin beta creation", w.Code)
	}
	w := call("POST", "/admin/beta-servers", operator, key, input)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created server.Server
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.OwnerID != operator || created.Status != server.PaymentConfirmed || created.RAMMB != 2048 || created.MinecraftVersion != "26.3" {
		t.Fatal("invalid beta server", created)
	}
	w = call("POST", "/admin/beta-servers", operator, key, input)
	if w.Code != 200 {
		t.Fatal("idempotent replay", w.Code, w.Body.String())
	}
	var replay server.Server
	if err = json.Unmarshal(w.Body.Bytes(), &replay); err != nil || replay.ID != created.ID {
		t.Fatal("duplicate server", err)
	}
	if w = call("POST", "/admin/beta-servers", operator, key, strings.Replace(input, "Latest world", "Different world", 1)); w.Code != 409 {
		t.Fatal("key reused with different input", w.Code)
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE server_id=$1 AND action='PROVISION'", created.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("provisioning job missing or duplicate", count, err)
	}
	if w = call("POST", "/servers/"+created.ID.String()+"/terminate", stranger, uuid.New(), `{"reason":"Wrong user","confirmation":"`+created.ID.String()+`"}`); w.Code != 404 {
		t.Fatal("foreign deletion", w.Code)
	}
	// Normal owners can delete their own servers; they cannot create beta servers.
	owned, err := (server.Service{DB: db}).Create(ctx, owner, server.CreateInput{PlanID: uuid.MustParse("10000000-0000-4000-8000-000000000001"), Name: "Old world", ServerType: "PAPER", MinecraftVersion: "26.3"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	path := "/servers/" + owned.ID.String() + "/terminate"
	if w = call("POST", path, owner, uuid.New(), `{"reason":"Owner requested deletion","confirmation":"wrong"}`); w.Code != 400 {
		t.Fatal("missing confirmation", w.Code)
	}
	w = call("POST", path, owner, uuid.New(), `{"reason":"Owner requested deletion","confirmation":"`+owned.ID.String()+`"}`)
	if w.Code != 202 {
		t.Fatal("owner deletion", w.Code, w.Body.String())
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE server_id=$1 AND action='TERMINATE'", owned.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("termination job missing", count, err)
	}
}
