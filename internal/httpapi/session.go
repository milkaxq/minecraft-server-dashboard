package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/user"
)

func (a *API) cookieName() string {
	if a.CookieSecure {
		return "__Host-blockyard_session"
	}
	return "blockyard_dev_session"
}
func (a *API) sessionToken(r *http.Request) (string, bool) {
	// Explicit bearer requests never fall back to ambient browser credentials.
	if r.Header.Get("Authorization") != "" {
		return bearer(r), false
	}
	cookies := r.CookiesNamed(a.cookieName())
	if len(cookies) != 1 {
		return "", false
	}
	return cookies[0].Value, true
}
func sessionCredential(a *API, r *http.Request) string { token, _ := a.sessionToken(r); return token }
func csrfToken(token string) string {
	// Domain-separated from the session digest. The random session credential is
	// never returned to browser JavaScript; its derived CSRF token is not a login.
	digest := sha256.Sum256([]byte("blockyard/csrf/v1:" + token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
func (a *API) validCSRF(r *http.Request, token string) bool {
	return len(token) == 43 && r.Header.Get("Origin") == a.Origin && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrfToken(token))) == 1
}
func (a *API) setSessionCookie(w http.ResponseWriter, token string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(), Value: token, Path: "/", HttpOnly: true, Secure: a.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: int(time.Until(expiry).Seconds())})
}
func (a *API) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(), Value: "", Path: "/", HttpOnly: true, Secure: a.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

type browserSession struct {
	User      user.User `json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
	CSRFToken string    `json:"csrf_token"`
}

func (a *API) cookieLogin(w http.ResponseWriter, r *http.Request) {
	// Require the exact configured browser origin even before a session exists.
	if r.Header.Get("Origin") != a.Origin {
		write(w, 403, map[string]string{"error": "origin denied", "request_id": requestID(r)})
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Auth.Login(r.Context(), input.Email, input.Password, requestID(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// Rotate an existing browser session after successful authentication only.
	if old, ok := a.sessionToken(r); ok {
		if err = a.Auth.Logout(r.Context(), old); err != nil {
			_ = a.Auth.Logout(r.Context(), result.Token)
			a.fail(w, r, err)
			return
		}
	}
	a.setSessionCookie(w, result.Token, result.ExpiresAt)
	write(w, 200, browserSession{result.User, result.ExpiresAt, csrfToken(result.Token)})
}
func (a *API) session(w http.ResponseWriter, r *http.Request) {
	token, cookieAuth := a.sessionToken(r)
	if !cookieAuth {
		a.fail(w, r, auth.ErrCredentials)
		return
	}
	result, err := a.Auth.Session(r.Context(), token)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, browserSession{result.User, result.ExpiresAt, csrfToken(token)})
}
