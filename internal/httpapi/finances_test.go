package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/admin"
	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/testutil"
)

func TestFinanceHTTPRequiresAdminAndStrictCosts(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	authentication, err := auth.New(db, &capturedMail{})
	if err != nil {
		t.Fatal(err)
	}
	password := "finance-test-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for _, role := range []string{"admin", "customer"} {
		id := uuid.New()
		email := role + "@example.com"
		if _, err = db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,$3,$4,now(),$5)", id, email, hash, role, role == "admin"); err != nil {
			t.Fatal(err)
		}
		session, e := authentication.Login(ctx, email, password, "test")
		if e != nil {
			t.Fatal(e)
		}
		tokens[role] = session.Token
	}
	a := &API{DB: db, Auth: authentication, Admin: &admin.Service{DB: db}, Origin: "http://localhost:3000", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h := a.Handler()
	call := func(method, path, role string, body any) int {
		var b bytes.Buffer
		if body != nil {
			json.NewEncoder(&b).Encode(body)
		}
		r := httptest.NewRequest(method, path, &b)
		if role != "" {
			r.Header.Set("Authorization", "Bearer "+tokens[role])
		}
		r.Header.Set("Origin", a.Origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, path := range []string{"/admin/finances", "/admin/hosting-costs"} {
		method := "GET"
		if path == "/admin/hosting-costs" {
			method = "PUT"
		}
		if code := call(method, path, "", nil); code != 401 {
			t.Fatal("anonymous access", path, code)
		}
		if code := call(method, path, "customer", nil); code != 403 {
			t.Fatal("customer access", path, code)
		}
	}
	if code := call("GET", "/admin/finances?month=2026-10", "admin", nil); code != 200 {
		t.Fatal("admin cannot read report", code)
	}
	if code := call("GET", "/admin/finances?month=bad-input", "admin", nil); code != 400 {
		t.Fatal("invalid reporting month accepted", code)
	}
	if code := call("PUT", "/admin/hosting-costs", "admin", map[string]any{"price_cents": 1}); code != 400 {
		t.Fatal("plan price field accepted as a cost", code)
	}
}
