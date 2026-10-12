package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/jobs"
)

func TestPrivateRoutesRejectMissingAuthenticationBeforeBusinessLogic(t *testing.T) {
	// No database is opened. Reaching a database-dependent handler would fail,
	// proving authentication happens before body processing or privileged actions.
	a := &API{Auth: &auth.Service{}, Jobs: jobs.Service{DB: &pgxpool.Pool{}}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := a.Handler()
	id := "10000000-0000-4000-8000-000000000001"
	for _, route := range []string{
		"GET /feedback", "POST /feedback", "GET /admin/feedback", "PUT /admin/feedback/" + id,
		"GET /auth/session", "GET /account", "PUT /account/password", "POST /auth/logout",
		"GET /servers", "POST /servers", "GET /servers/" + id,
		"GET /servers/" + id + "/usage",
		"GET /servers/" + id + "/members", "POST /servers/" + id + "/members", "PUT /servers/" + id + "/members/" + id, "DELETE /servers/" + id + "/members/" + id,
		"GET /servers/" + id + "/world", "GET /servers/" + id + "/world/download", "POST /servers/" + id + "/world/backup", "POST /servers/" + id + "/world/reset", "POST /servers/" + id + "/world/upload",
		"GET /servers/" + id + "/eula", "POST /servers/" + id + "/eula",
		"POST /servers/" + id + "/start", "POST /servers/" + id + "/stop",
		"POST /servers/" + id + "/restart", "POST /servers/" + id + "/kill",
		"POST /servers/" + id + "/commands", "POST /servers/" + id + "/console-ticket",
		"GET /servers/" + id + "/files", "POST /servers/" + id + "/files",
		"GET /servers/" + id + "/files/text", "POST /servers/" + id + "/files/upload", "GET /servers/" + id + "/files/download",
		"GET /servers/" + id + "/settings", "PUT /servers/" + id + "/settings",
		"GET /servers/" + id + "/plugins", "POST /servers/" + id + "/plugins/upload", "DELETE /servers/" + id + "/plugins",
		"GET /servers/" + id + "/backups", "POST /servers/" + id + "/backups",
		"POST /servers/" + id + "/backups/" + id + "/restore", "DELETE /servers/" + id + "/backups/" + id,
		"GET /servers/" + id + "/backups/" + id + "/download",
		"GET /billing/config", "GET /orders", "POST /orders", "GET /orders/" + id,
		"POST /orders/" + id + "/checkout", "POST /orders/" + id + "/cancel", "POST /orders/" + id + "/mock-pay",
		"GET /subscriptions", "PUT /servers/" + id + "/subscription", "POST /servers/" + id + "/renewals",
		"GET /admin/plans", "POST /admin/plans", "PUT /admin/plans/" + id,
		"GET /admin/invitations", "POST /admin/invitations", "POST /admin/invitations/" + id + "/revoke",
		"GET /admin/users", "GET /admin/servers", "GET /admin/orders", "GET /admin/nodes", "GET /admin/audit",
		"POST /admin/servers/" + id + "/provision", "POST /admin/servers/" + id + "/retry",
		"POST /admin/servers/" + id + "/suspend", "POST /admin/servers/" + id + "/unsuspend",
		"POST /admin/servers/" + id + "/terminate", "GET /admin/jobs/" + id,
	} {
		t.Run(route, func(t *testing.T) {
			method, path, _ := strings.Cut(route, " ")
			r := httptest.NewRequest(method, path, strings.NewReader("invalid body"))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected authentication rejection, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
