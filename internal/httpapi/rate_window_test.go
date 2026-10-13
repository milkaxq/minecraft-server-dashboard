package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/testutil"
	"minecraft-hosting/internal/user"
)

func TestDashboardReadsResetAfterMinuteWithoutConsumingWriteBudget(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	id := uuid.New()
	a := API{Auth: &auth.Service{DB: db}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h := a.liveLimit("management", 2, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	call := func(method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/test", nil)
		r = r.WithContext(context.WithValue(context.WithValue(r.Context(), requestKey, "rate-test"), userKey, user.User{ID: id}))
		w := httptest.NewRecorder()
		h(w, r)
		return w
	}
	for range 2 {
		if w := call("GET"); w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call("GET"); w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatal(w.Code, w.Header())
	}
	if w := call("PUT"); w.Code != 204 {
		t.Fatal("reads consumed write quota", w.Code)
	}
	_, err := db.Exec(ctx, "UPDATE auth_rate_limits SET window_start=now()-interval '61 seconds' WHERE bucket IN ($1,$2)", "live:management:read:"+id.String(), "live:management:write:"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	if w := call("GET"); w.Code != 204 {
		t.Fatal("minute did not reset", w.Code)
	}
	if w := call("PUT"); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := call("PUT"); w.Code != 429 || w.Header().Get("Retry-After") != "900" {
		t.Fatal("write window weakened", w.Code, w.Header())
	}
}
