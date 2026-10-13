package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

func TestManagementHTTPAuthorizationCORSAndStrictInput(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, other, id, node := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{owner, other} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'unused','Player',now())", u, u.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',100,2048,10240)", node); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO servers(id,owner_id,node_id,plan_id,pterodactyl_server_id,pterodactyl_identifier,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,$3,'10000000-0000-4000-8000-000000000001',1,'abcdef12','World','RUNNING','1.21.1','PAPER',2048,10240,100,2,0)`, id, owner, node); err != nil {
		t.Fatal(err)
	}
	tokens := map[uuid.UUID]string{owner: strings.Repeat("o", 43), other: strings.Repeat("x", 43)}
	for u, token := range tokens {
		hash := sha256.Sum256([]byte(token))
		if _, err := db.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash[:], u); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasSuffix(r.URL.Path, "/files/list") {
			json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"attributes": map[string]any{"identifier": "abcdef12", "status": nil}})
	}))
	defer panel.Close()
	client, err := pterodactyl.NewServerClient(panel.URL, "client-key", true)
	if err != nil {
		t.Fatal(err)
	}
	authentication, err := auth.New(db, &capturedMail{})
	if err != nil {
		t.Fatal(err)
	}
	controls := &control.Service{DB: db, Panel: client}
	manager := &management.Service{DB: db, Control: controls, Panel: client}
	a := &API{DB: db, Auth: authentication, Servers: server.Service{DB: db}, Control: controls, Management: manager, Origin: "http://localhost:3000", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := a.Handler()
	call := func(method, suffix, token, body, contentType string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/servers/"+id.String()+suffix, strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("Idempotency-Key", uuid.NewString())
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	backup := uuid.NewString()
	for _, route := range []struct{ method, path, body, kind string }{
		{"POST", "/plugins/check", `{}`, "application/json"},
		{"GET", "/client-requirements", "", "application/json"},
		{"GET", "/addons/simple-voice-chat", "", "application/json"},
		{"GET", "/addons/simple-voice-chat/network", "", "application/json"},
		{"PUT", "/addons/simple-voice-chat", `{"max_voice_distance":48,"whisper_distance":24,"enable_groups":true,"revision":"` + strings.Repeat("a", 64) + `"}`, "application/json"},
		{"POST", "/addons/simple-voice-chat/network", `{}`, "application/json"},
		{"GET", "/whitelist", "", "application/json"}, {"POST", "/whitelist", `{"action":"add","name":"Steve","revision":"` + strings.Repeat("a", 64) + `"}`, "application/json"},
		{"GET", "/members", "", "application/json"}, {"POST", "/members", `{"email":"friend@example.com","role":"ADMIN"}`, "application/json"}, {"PUT", "/members/" + other.String(), `{"role":"ADMIN"}`, "application/json"}, {"DELETE", "/members/" + other.String(), `{"confirmation":"` + other.String() + `"}`, "application/json"},
		{"GET", "/world", "", "application/json"}, {"GET", "/world/download", "", "application/json"}, {"POST", "/world/backup", "{}", "application/json"}, {"POST", "/world/reset", `{"backup":"` + backup + `","confirmation":"world","revision":"` + strings.Repeat("a", 64) + `"}`, "application/json"}, {"POST", "/world/upload?backup=" + backup + "&confirmation=world&revision=" + strings.Repeat("a", 64), "invalid zip", "application/octet-stream"},
		{"GET", "/files?path=/", "", "application/json"}, {"GET", "/files/text?path=/server.properties", "", "application/json"}, {"GET", "/files/download?path=/server.properties", "", "application/json"}, {"GET", "/settings", "", "application/json"}, {"GET", "/plugins", "", "application/json"}, {"GET", "/mods", "", "application/json"}, {"POST", "/mods/upload?name=test.jar", "abc", "application/octet-stream"}, {"DELETE", "/mods", `{"name":"Test.jar","confirmation":"Test.jar"}`, "application/json"}, {"GET", "/backups", "", "application/json"}, {"GET", "/backups/" + backup + "/download", "", "application/json"},
		{"POST", "/files", `{"action":"create","path":"/new.txt"}`, "application/json"}, {"POST", "/files/upload?path=/&name=new.txt", "abc", "application/octet-stream"}, {"POST", "/plugins/upload?name=test.jar", "abc", "application/octet-stream"}, {"POST", "/backups", `{"name":"Snapshot"}`, "application/json"}, {"POST", "/backups/" + backup + "/restore", `{"confirmation":"` + backup + `"}`, "application/json"}, {"DELETE", "/backups/" + backup, `{"confirmation":"` + backup + `"}`, "application/json"}, {"DELETE", "/plugins", `{"name":"Test.jar","confirmation":"Test.jar"}`, "application/json"},
	} {
		if w := call(route.method, route.path, "", route.body, route.kind); w.Code != 401 {
			t.Fatalf("unauthenticated %s: %d %s", route.path, w.Code, w.Body.String())
		}
		// Plugin upload validates the jar before business access, but HTTP authorization
		// must reject a foreign user before consuming it or calling Panel.
		if w := call(route.method, route.path, tokens[other], route.body, route.kind); w.Code != 404 {
			t.Fatalf("foreign %s: %d %s", route.path, w.Code, w.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("foreign requests contacted Panel", calls.Load())
	}
	for _, body := range []string{`{"action":"create","path":"/new","ram_mb":999999}`, `{"action":"delete","path":"/"}`, `{"action":"create","path":"/../host"}`} {
		if w := call("POST", "/files", tokens[owner], body, "application/json"); w.Code != 400 {
			t.Fatal("unsafe input accepted", w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"action":"add","name":"Steve","revision":"` + strings.Repeat("a", 64) + `","uuid":"` + owner.String() + `"}`, `{"action":"add","name":"Steve\nstop","revision":"` + strings.Repeat("a", 64) + `"}`, `{"action":"off","revision":"` + strings.Repeat("a", 64) + `"}`} {
		if w := call("POST", "/whitelist", tokens[owner], body, "application/json"); w.Code != 400 {
			t.Fatal("unsafe whitelist input accepted", w.Code, w.Body.String())
		}
	}
	if w := call("GET", "/files?path=/", tokens[owner], "", "application/json"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("OPTIONS", "/servers/"+id.String()+"/settings", nil)
	r.Header.Set("Origin", a.Origin)
	r.Header.Set("Access-Control-Request-Method", "PUT")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "PUT") || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
		t.Fatal("management CORS methods missing")
	}
}
