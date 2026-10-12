package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/testutil"
)

func TestCookieSessionRestorationCSRFRevocationAndBearerCompatibility(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	authentication, err := auth.New(db, &capturedMail{})
	if err != nil {
		t.Fatal(err)
	}
	password := "cookie-fixture-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if _, err = db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,'cookie@example.com',$2,'Cookie',now())", id, hash); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	a := &API{DB: db, Auth: authentication, Origin: "https://dashboard.example.com", CookieSecure: true, Log: slog.New(slog.NewTextHandler(&logs, nil))}
	h := a.Handler()
	call := func(method, path, origin, csrf string, cookie *http.Cookie, body any, bearerToken string) *httptest.ResponseRecorder {
		var b bytes.Buffer
		if body != nil {
			if e := json.NewEncoder(&b).Encode(body); e != nil {
				t.Fatal(e)
			}
		}
		r := httptest.NewRequest(method, path, &b)
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if bearerToken != "" {
			r.Header.Set("Authorization", "Bearer "+bearerToken)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	credentials := map[string]string{"email": "cookie@example.com", "password": password}
	for _, origin := range []string{"", "https://evil.example.com"} {
		if w := call("POST", "/auth/session-login", origin, "", nil, credentials, ""); w.Code != 403 {
			t.Fatal("login CSRF", w.Code)
		}
	}
	w := call("POST", "/auth/session-login", a.Origin, "", nil, credentials, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("no session cookie")
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-blockyard_session" || !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge < 86000 {
		t.Fatal("unsafe cookie flags")
	}
	var session browserSession
	if err = json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), cookie.Value) || strings.Contains(w.Body.String(), `"token"`) || session.CSRFToken == cookie.Value {
		t.Fatal("credential exposed")
	}
	if w = call("GET", "/auth/session", a.Origin, "", cookie, nil, ""); w.Code != 200 || !strings.Contains(w.Body.String(), session.CSRFToken) {
		t.Fatal("restore", w.Code)
	}
	for _, tc := range []struct{ origin, csrf string }{{a.Origin, ""}, {a.Origin, "wrong"}, {"", session.CSRFToken}, {"https://evil.example.com", session.CSRFToken}} {
		if w = call("POST", "/auth/logout", tc.origin, tc.csrf, cookie, nil, ""); w.Code != 403 {
			t.Fatal("mutation CSRF", w.Code)
		}
	}
	// The CSRF token is not a bearer credential. Bearer clients still use the API.
	if w = call("GET", "/account", "", "", nil, nil, session.CSRFToken); w.Code != 401 {
		t.Fatal("csrf token granted login", w.Code)
	}
	if w = call("GET", "/account", "", "", cookie, nil, "invalid"); w.Code != 401 {
		t.Fatal("bearer fell back to cookie", w.Code)
	}
	// Successful cookie login rotates and invalidates the prior browser session.
	w = call("POST", "/auth/session-login", a.Origin, "", cookie, credentials, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	rotated := w.Result().Cookies()[0]
	var next browserSession
	if err = json.Unmarshal(w.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if rotated.Value == cookie.Value {
		t.Fatal("session fixation")
	}
	if w = call("GET", "/auth/session", a.Origin, "", cookie, nil, ""); w.Code != 401 {
		t.Fatal("old session survived rotation", w.Code)
	}
	if w = call("POST", "/auth/logout", a.Origin, session.CSRFToken, rotated, nil, ""); w.Code != 403 {
		t.Fatal("old CSRF accepted", w.Code)
	}
	w = call("POST", "/auth/logout", a.Origin, next.CSRFToken, rotated, nil, "")
	if w.Code != 204 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout", w.Code)
	}
	if w = call("GET", "/auth/session", a.Origin, "", rotated, nil, ""); w.Code != 401 {
		t.Fatal("logout restored", w.Code)
	}
	w = call("POST", "/auth/login", "", "", nil, credentials, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var legacy auth.LoginResult
	if err = json.Unmarshal(w.Body.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	if w = call("GET", "/account", "", "", nil, nil, legacy.Token); w.Code != 200 {
		t.Fatal("bearer regression", w.Code)
	}
	if w = call("GET", "/auth/session", a.Origin, "", nil, nil, legacy.Token); w.Code != 401 {
		t.Fatal("browser restore accepted bearer")
	}
	legacyCookie := &http.Cookie{Name: a.cookieName(), Value: legacy.Token}
	change := map[string]string{"current_password": password, "new_password": "new-cookie-fixture-password"}
	w = call("PUT", "/account/password", a.Origin, csrfToken(legacy.Token), legacyCookie, change, "")
	if w.Code != 204 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("cookie password change", w.Code, w.Body.String())
	}
	if w = call("GET", "/account", "", "", nil, nil, legacy.Token); w.Code != 401 {
		t.Fatal("password change left session active")
	}
	credentials["password"] = change["new_password"]
	w = call("POST", "/auth/session-login", a.Origin, "", nil, credentials, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	expiring := w.Result().Cookies()[0]
	if _, err = db.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if w = call("GET", "/account", "", "", nil, nil, legacy.Token); w.Code != 401 {
		t.Fatal("expiry ignored")
	}
	if w = call("GET", "/auth/session", a.Origin, "", expiring, nil, ""); w.Code != 401 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("expired browser session restored", w.Code)
	}
	for _, secret := range []string{cookie.Value, rotated.Value, session.CSRFToken, legacy.Token, password} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("session secret logged")
		}
	}
}
