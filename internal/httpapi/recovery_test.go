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

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/testutil"
)

func TestRecoveryAPIPrivacyValidationAndSessionRevocation(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	mailer := &capturedMail{}
	service, err := auth.New(db, mailer)
	if err != nil {
		t.Fatal(err)
	}
	service.RecoveryKey = bytes.Repeat([]byte{9}, 32)
	hash, err := auth.HashPassword("original-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,'player@example.com',$2,'Player',now())", uuid.New(), hash); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	a := &API{DB: db, Auth: service, Log: slog.New(slog.NewTextHandler(&logs, nil)), Origin: "http://localhost:3000"}
	handler := a.Handler()
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var data bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&data).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, &data)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", a.Origin)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	known := call("POST", "/auth/forgot-password", "", map[string]string{"email": "player@example.com"})
	unknown := call("POST", "/auth/forgot-password", "", map[string]string{"email": "unknown@example.com"})
	if known.Code != 202 || unknown.Code != 202 || known.Body.String() != unknown.Body.String() {
		t.Fatal("account enumeration", known.Code, unknown.Code)
	}
	if known.Header().Get("Cache-Control") != "no-store" || known.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("missing privacy headers")
	}
	if ok, err := service.DeliverRecoveryOnce(ctx, nil); !ok || err != nil {
		t.Fatal(ok, err)
	}
	resetToken := mailer.token
	login, err := service.Login(ctx, "player@example.com", "original-password", "login")
	if err != nil {
		t.Fatal(err)
	}
	change := map[string]string{"current_password": "wrong-password", "new_password": "replacement-password"}
	if w := call("PUT", "/account/password", login.Token, change); w.Code != 400 {
		t.Fatal("wrong current password", w.Code)
	}
	change["user_id"] = uuid.NewString()
	if w := call("PUT", "/account/password", login.Token, change); w.Code != 400 {
		t.Fatal("account override accepted", w.Code)
	}
	if w := call("POST", "/auth/reset-password", "", map[string]string{"token": "invalid", "password": "replacement-password"}); w.Code != 400 {
		t.Fatal(w.Code)
	}
	input := map[string]string{"token": resetToken, "password": "replacement-password"}
	if w := call("POST", "/auth/reset-password", "", input); w.Code != 204 {
		t.Fatal("reset failed", w.Code, w.Body.String())
	}
	if w := call("GET", "/account", login.Token, nil); w.Code != 401 {
		t.Fatal("old session survived", w.Code)
	}
	if w := call("POST", "/auth/reset-password", "", input); w.Code != 400 {
		t.Fatal("token replay accepted", w.Code)
	}
	for _, secret := range []string{resetToken, login.Token, "replacement-password", "original-password"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("secret leaked to logs")
		}
	}
}
