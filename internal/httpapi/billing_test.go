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

	"minecraft-hosting/internal/admin"
	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/billing"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

func TestBillingAndAdminHTTPRequireOwnershipAndAuthority(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, other, operator := uuid.New(), uuid.New(), uuid.New()
	tokens := map[uuid.UUID]string{owner: strings.Repeat("o", 43), other: strings.Repeat("x", 43), operator: strings.Repeat("a", 43)}
	for user, token := range tokens {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,'unused','Player',now(),$3)", user, user.String()+"@example.com", user == operator); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(token))
		if _, err := db.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", hash[:], user); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO software_templates(server_type,egg_id,docker_image,startup,version_variable) VALUES('PAPER',1,'java','minecraft','MINECRAFT_VERSION');INSERT INTO minecraft_versions(server_type,version) VALUES('PAPER','1.21.1')`); err != nil {
		t.Fatal(err)
	}
	authentication, err := auth.New(db, &capturedMail{})
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := billing.NewMock("test-only-payment-secret-at-least-32-characters")
	payments := &billing.Service{DB: db, Provider: provider}
	a := &API{DB: db, Auth: authentication, Plans: plan.Service{DB: db}, Servers: server.Service{DB: db}, Jobs: jobs.Service{DB: db}, Billing: payments, Admin: &admin.Service{DB: db}, Origin: "http://localhost:3000", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := a.Handler()
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", uuid.NewString())
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	input := `{"plan_id":"10000000-0000-4000-8000-000000000001","name":"World","server_type":"PAPER","minecraft_version":"1.21.1"}`
	if w := call("POST", "/orders", tokens[owner], strings.TrimSuffix(input, "}")+`,"price_cents":1}`); w.Code != 400 {
		t.Fatal("price injected", w.Code, w.Body.String())
	}
	w := call("POST", "/orders", tokens[owner], input)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var order billing.Order
	if err = json.Unmarshal(w.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{{"GET", "/orders/" + order.ID.String()}, {"POST", "/orders/" + order.ID.String() + "/checkout"}, {"POST", "/orders/" + order.ID.String() + "/mock-pay"}, {"POST", "/orders/" + order.ID.String() + "/cancel"}, {"POST", "/servers/" + order.ServerID.String() + "/renewals"}, {"PUT", "/servers/" + order.ServerID.String() + "/subscription"}} {
		body := "{}"
		if route.method == "PUT" {
			body = `{"cancel_at_period_end":true}`
		}
		if w = call(route.method, route.path, tokens[other], body); w.Code != 404 {
			t.Fatal("foreign billing", route.path, w.Code, w.Body.String())
		}
	}
	if w = call("POST", "/payments/webhooks/mock", "", `{"status":"paid"}`); w.Code != 401 {
		t.Fatal("unsigned webhook", w.Code)
	}
	if w = call("POST", "/orders/"+order.ID.String()+"/mock-pay", tokens[owner], "{}"); w.Code != 200 {
		t.Fatal("payment", w.Code, w.Body.String())
	}
	if w = call("GET", "/subscriptions", tokens[other], ""); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("foreign subscriptions exposed", w.Body.String())
	}
	for _, path := range []string{"/admin/users", "/admin/servers", "/admin/orders", "/admin/nodes", "/admin/plans", "/admin/audit"} {
		if w = call("GET", path, tokens[owner], ""); w.Code != 403 {
			t.Fatal("customer read admin", path, w.Code)
		}
		if w = call("GET", path, tokens[operator], ""); w.Code != 200 {
			t.Fatal("admin list", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "mock_") || strings.Contains(w.Body.String(), "password_hash") {
			t.Fatal("private fields in admin data", path, w.Body.String())
		}
	}
	if w = call("POST", "/admin/plans", tokens[owner], `{"is_admin":true}`); w.Code != 403 {
		t.Fatal("administrator flag trusted")
	}
	if w = call("POST", "/admin/servers/"+order.ServerID.String()+"/suspend", tokens[owner], `{"reason":"client flag"}`); w.Code != 403 {
		t.Fatal("customer lifecycle control")
	}
}
