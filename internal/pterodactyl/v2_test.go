package pterodactyl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// Contract fields checked against Panel f154449a8075595d12b8b01fa52cab570d04056d:
// Application ServerTransformer and Models/Server. This is not a live Panel test.
func TestPanelV2StatusContract(t *testing.T) {
	external := uuid.New()
	for _, tc := range []struct {
		name, status                     string
		installed                        int
		wantInstalled, suspended, failed bool
	}{
		{name: "ready", status: "null", installed: 1, wantInstalled: true},
		{name: "suspended", status: `"suspended"`, installed: 1, wantInstalled: true, suspended: true},
		{name: "installing", status: `"installing"`, installed: 1},
		{name: "install failed", status: `"install_failed"`, installed: 1, failed: true},
		{name: "reinstall failed", status: `"reinstall_failed"`, installed: 1, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/application/servers/12" || r.Header.Get("Authorization") != "Bearer test-application-key" {
					t.Error("wrong route or credential")
				}
				w.Header().Set("Content-Type", "application/json")
				// Deliberately omit deprecated suspended: status must still enforce the hold.
				_, _ = fmt.Fprintf(w, `{"object":"server","attributes":{"id":12,"identifier":"abcd1234","external_id":%q,"status":%s,"user":7,"node":2,"allocation":51,"egg":3,"limits":{"memory":2048,"disk":10240,"cpu":100},"feature_limits":{"backups":2,"databases":0},"container":{"installed":%d}}}`, external.String(), tc.status, tc.installed)
			}))
			defer panel.Close()
			client, err := NewApplication(panel.URL, "test-application-key", true)
			if err != nil {
				t.Fatal(err)
			}
			server, err := client.GetServer(context.Background(), 12)
			if err != nil {
				t.Fatal(err)
			}
			if server.Installed != tc.wantInstalled || server.Suspended != tc.suspended || server.InstallFailed != tc.failed {
				t.Fatalf("unexpected lifecycle flags: installed=%t suspended=%t failed=%t", server.Installed, server.Suspended, server.InstallFailed)
			}
			if server.Request.UserID != 7 || server.Request.Limits.RAMMB != 2048 || server.Request.Limits.Backups != 2 {
				t.Fatal("lost ownership or purchased limits")
			}
		})
	}
}
