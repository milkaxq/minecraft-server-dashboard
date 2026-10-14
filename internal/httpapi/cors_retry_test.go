package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSExposesRetryAfterOnlyToAllowedOrigin(t *testing.T) {
	a := &API{Origin: "http://localhost:3000", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, origin := range []string{a.Origin, "https://untrusted.example", ""} {
		t.Run(origin, func(t *testing.T) {
			called := false
			handler := a.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			r := httptest.NewRequest(http.MethodGet, "/join/test", nil)
			r.Header.Set("Origin", origin)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if origin != "" && origin != a.Origin {
				if called || w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Expose-Headers") != "" {
					t.Fatal("untrusted origin received access to the handler or its response headers")
				}
				return
			}
			if !called || w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "120" {
				t.Fatal("rate-limit response was not preserved")
			}
			if origin == a.Origin && (!strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), "Retry-After") || w.Header().Get("Access-Control-Allow-Origin") != a.Origin) {
				t.Fatal("allowed frontend cannot read the retry delay")
			}
			if origin == "" && w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("same-origin request unexpectedly received cross-origin access")
			}
		})
	}
}
