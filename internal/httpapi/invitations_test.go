package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/admin"
	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/testutil"
)

func TestInvitationAPIEnforcesGateAndAdminPrivacy(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	mailer := &capturedMail{}
	s, err := auth.New(db, mailer)
	if err != nil {
		t.Fatal(err)
	}
	s.InviteOnly = true
	hash, err := auth.HashPassword("fixture-long-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"admin@example.com", "member@example.com"} {
		_, err = db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at,is_admin) VALUES($1,$2,$3,'Fixture',now(),$4)", uuid.New(), email, hash, email == "admin@example.com")
		if err != nil {
			t.Fatal(err)
		}
	}
	operator, err := s.Login(ctx, "admin@example.com", "fixture-long-password", "login")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.Login(ctx, "member@example.com", "fixture-long-password", "login")
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	a := &API{DB: db, Auth: s, Admin: &admin.Service{DB: db}, Origin: "http://localhost:3000", Log: slog.New(slog.NewTextHandler(&logs, nil))}
	h := a.Handler()
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var b bytes.Buffer
		if body != nil {
			json.NewEncoder(&b).Encode(body)
		}
		r := httptest.NewRequest(method, path, &b)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", a.Origin)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/auth/config", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"invite_required":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, method := range []string{"GET", "POST"} {
		if w := call(method, "/admin/invitations", member.Token, map[string]string{"email": "guest@example.com"}); w.Code != 403 {
			t.Fatal("non-admin", w.Code)
		}
	}
	w := call("POST", "/admin/invitations", operator.Token, map[string]string{"email": "guest@example.com"})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var issued struct {
		ID   string `json:"id"`
		Link string `json:"link"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(issued.Link)
	if err != nil || link.Host != "localhost:3000" || link.Path != "/register" || link.RawQuery != "" {
		t.Fatal("bad invitation link", err)
	}
	params, err := url.ParseQuery(link.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	invite := params.Get("invite")
	if len(invite) != 43 || params.Get("email") != "guest@example.com" {
		t.Fatal("bad invite params")
	}
	input := auth.RegisterInput{Email: "guest@example.com", Name: "Guest", Password: "guest-long-password"}
	if w = call("POST", "/auth/register", "", input); w.Code != 403 {
		t.Fatal("gate bypassed", w.Code)
	}
	input.InvitationToken = invite
	input.Email = "outsider@example.com"
	if w = call("POST", "/auth/register", "", input); w.Code != 403 {
		t.Fatal("email binding bypassed", w.Code)
	}
	input.Email = "guest@example.com"
	if w = call("POST", "/auth/register", "", input); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	verification := mailer.token
	if w = call("POST", "/auth/register", "", input); w.Code != 403 {
		t.Fatal("invite replay", w.Code)
	}
	if w = call("POST", "/auth/verify-email", "", map[string]string{"token": verification}); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w = call("GET", "/admin/invitations", operator.Token, nil); w.Code != 200 || strings.Contains(w.Body.String(), invite) || !strings.Contains(w.Body.String(), "USED") {
		t.Fatal("unsafe invitation list", w.Code)
	}
	if w = call("POST", "/admin/invitations/"+issued.ID+"/revoke", member.Token, map[string]string{}); w.Code != 403 {
		t.Fatal("non-admin revoke", w.Code)
	}
	if w = call("POST", "/admin/invitations/"+issued.ID+"/revoke", operator.Token, map[string]string{}); w.Code != 204 {
		t.Fatal(w.Code)
	}
	for _, secret := range []string{invite, verification, operator.Token, member.Token, input.Password} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("secret logged")
		}
	}
}
