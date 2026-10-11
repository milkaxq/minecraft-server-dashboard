package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/admin"
	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/testutil"
)

func TestFeedbackAPIPrivacyValidationAndLogSafety(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	authentication, err := auth.New(db, &capturedMail{})
	if err != nil {
		t.Fatal(err)
	}
	password := "feedback-fixture-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for _, name := range []string{"tester", "outsider", "admin"} {
		email := name + "@example.com"
		if _, err = db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,$3,$4,now(),$5)", uuid.New(), email, hash, name, name == "admin"); err != nil {
			t.Fatal(err)
		}
		session, e := authentication.Login(ctx, email, password, "fixture")
		if e != nil {
			t.Fatal(e)
		}
		tokens[name] = session.Token
	}
	var logs bytes.Buffer
	a := &API{DB: db, Auth: authentication, Admin: &admin.Service{DB: db}, Origin: "http://localhost:3000", Log: slog.New(slog.NewTextHandler(&logs, nil))}
	h := a.Handler()
	call := func(method, path, actor, key string, body any) *httptest.ResponseRecorder {
		var b bytes.Buffer
		if body != nil {
			if e := json.NewEncoder(&b).Encode(body); e != nil {
				t.Fatal(e)
			}
		}
		r := httptest.NewRequest(method, path, &b)
		r.Header.Set("Authorization", "Bearer "+tokens[actor])
		r.Header.Set("Origin", a.Origin)
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	in := map[string]any{"category": "BUG", "title": "PRIVATE report title", "message": "PRIVATE message from a tester."}
	key := uuid.NewString()
	if w := call("POST", "/feedback", "tester", "", in); w.Code != 400 {
		t.Fatal("missing submission key", w.Code)
	}
	invalid := map[string]any{"category": "BUG", "title": "Valid title", "message": "Valid message here", "status": "RESOLVED"}
	if w := call("POST", "/feedback", "tester", uuid.NewString(), invalid); w.Code != 400 {
		t.Fatal("unknown field accepted", w.Code)
	}
	foreign := map[string]any{"category": "BUG", "title": "Valid title", "message": "Valid message here", "server_id": uuid.NewString()}
	if w := call("POST", "/feedback", "tester", uuid.NewString(), foreign); w.Code != 404 {
		t.Fatal("unauthorized server", w.Code)
	}
	w := call("POST", "/feedback", "tester", key, in)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var report struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	replay := call("POST", "/feedback", "tester", key, in)
	if replay.Code != 201 || replay.Body.String() != w.Body.String() {
		t.Fatal("duplicate submission", replay.Code)
	}
	if w = call("GET", "/feedback", "outsider", "", nil); w.Code != 200 || strings.Contains(w.Body.String(), report.ID) {
		t.Fatal("report leaked", w.Code)
	}
	if w = call("GET", "/admin/feedback", "tester", "", nil); w.Code != 403 {
		t.Fatal("admin bypass", w.Code)
	}
	update := map[string]any{"status": "RESOLVED", "response": "PRIVATE host response", "revision": report.Revision}
	if w = call("PUT", "/admin/feedback/"+report.ID, "tester", "", update); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = call("PUT", "/admin/feedback/"+report.ID, "admin", "", update); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("PUT", "/admin/feedback/"+report.ID, "admin", "", update); w.Code != 409 {
		t.Fatal("stale update accepted", w.Code)
	}
	if w = call("GET", "/feedback", "tester", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "PRIVATE host response") {
		t.Fatal("reply missing", w.Code)
	}
	if w = call("GET", "/feedback?offset=-1", "tester", "", nil); w.Code != 400 {
		t.Fatal(w.Code)
	}
	for _, private := range []string{"PRIVATE report title", "PRIVATE message from a tester.", "PRIVATE host response", password, tokens["tester"], key} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("private report content logged")
		}
	}
}
