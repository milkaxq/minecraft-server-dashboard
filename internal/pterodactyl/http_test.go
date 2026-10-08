package pterodactyl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestApplicationProtocolAndCredentialSeparation(t *testing.T) {
	external, userID := uuid.New(), uuid.New()
	var applicationCalls, clientCalls atomic.Int32
	response := func(w http.ResponseWriter, attrs any) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"object": "server", "attributes": attrs}); err != nil {
			t.Error(err)
		}
	}
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/application/") {
			applicationCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer application-secret" {
				t.Error("wrong Application credential")
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/client/") {
			clientCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer client-secret" {
				t.Error("wrong Client credential")
			}
		}
		if r.Header.Get("Accept") != "application/vnd.pterodactyl.v1+json" {
			t.Error("missing API media type")
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/application/servers":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
				return
			}
			limits := payload["limits"].(map[string]any)
			features := payload["feature_limits"].(map[string]any)
			allocation := payload["allocation"].(map[string]any)
			if limits["memory"] != float64(2048) || limits["disk"] != float64(10240) || limits["cpu"] != float64(100) || limits["swap"] != float64(0) || allocation["default"] != float64(51) || features["backups"] != float64(2) || payload["start_on_completion"] != false || payload["skip_scripts"] != false || payload["oom_disabled"] != false {
				t.Error("incorrect constrained provisioning payload")
			}
			if payload["external_id"] != external.String() || payload["user"] != float64(7) || payload["egg"] != float64(3) {
				t.Error("incorrect trusted IDs")
			}
			response(w, map[string]any{"id": 12, "external_id": external, "identifier": "abcd1234", "node": 2, "user": 7, "egg": 3, "allocation": 51, "status": "installing", "limits": limits, "feature_limits": features, "container": map[string]int{"installed": 0}})
		case r.Method == "POST" && r.URL.Path == "/api/application/users":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
				return
			}
			if payload["root_admin"] != false || payload["external_id"] != userID.String() || len(payload["password"].(string)) < 32 {
				t.Error("unsafe Panel user")
			}
			response(w, map[string]any{"id": 7, "external_id": userID})
		case r.Method == "GET" && r.URL.Path == "/api/application/users/external/"+userID.String():
			response(w, map[string]any{"id": 7, "external_id": userID})
		case r.Method == "GET" && r.URL.Path == "/api/application/nodes/2/allocations":
			page := r.URL.Query().Get("page")
			id := 51
			if page == "2" {
				id = 52
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"id": id, "ip": "192.0.2.1", "alias": "play.example.com", "port": 25565, "assigned": false}}}, "meta": map[string]any{"pagination": map[string]int{"total_pages": 2}}})
		case r.Method == "GET" && r.URL.Path == "/api/client/servers/abcd1234/resources":
			response(w, map[string]any{"current_state": "running", "resources": map[string]any{"cpu_absolute": 12.5, "memory_bytes": 1024, "disk_bytes": 2048, "uptime": 3000}})
		case r.Method == "POST" && (strings.HasSuffix(r.URL.Path, "/power") || strings.HasSuffix(r.URL.Path, "/command")):
			w.WriteHeader(204)
		case r.Method == "DELETE":
			w.WriteHeader(404)
		default:
			w.WriteHeader(404)
		}
	}))
	defer panel.Close()
	application, err := NewApplication(panel.URL, "application-secret", true)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewServerClient(panel.URL, "client-secret", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u, err := application.CreateUser(ctx, UserRequest{ExternalID: userID, Email: "player@example.com", Name: "Player"})
	if err != nil || u.ID != 7 {
		t.Fatal("CreateUser", err)
	}
	if _, err = application.FindUserByExternalID(ctx, userID); err != nil {
		t.Fatal(err)
	}
	s, err := application.CreateServer(ctx, ServerRequest{ExternalID: external, UserID: 7, NodeID: 2, AllocationID: 51, EggID: 3, Name: "World", Image: "java21", Startup: "minecraft", Environment: map[string]string{"MINECRAFT_VERSION": "1.21.1"}, Limits: Limits{RAMMB: 2048, DiskMB: 10240, CPU: 100, Backups: 2}})
	if err != nil || s.ID != 12 || s.Installed || s.Request.NodeID != 2 || s.Request.Limits.RAMMB != 2048 {
		t.Fatal("CreateServer", err)
	}
	allocations, err := application.ListAllocations(ctx, 2)
	if err != nil || len(allocations) != 2 || allocations[1].ID != 52 {
		t.Fatal("allocation pagination", err)
	}
	if err = application.DeleteServer(ctx, 12); err != nil {
		t.Fatal("idempotent deletion", err)
	}
	usage, err := client.Resources(ctx, "abcd1234")
	if err != nil || usage.CPUPercent != 12.5 || usage.UptimeMillis != 3000 {
		t.Fatal("resource decoding", err)
	}
	if err = client.Power(ctx, "abcd1234", PowerStart); err != nil {
		t.Fatal(err)
	}
	if err = client.SendCommand(ctx, "abcd1234", "list"); err != nil {
		t.Fatal(err)
	}
	before := clientCalls.Load()
	if err = client.Power(ctx, "../application/users", PowerStart); !errors.Is(err, ErrInvalid) {
		t.Fatal("path injection accepted")
	}
	if err = client.SendCommand(ctx, "abcd1234", "list\nstop"); !errors.Is(err, ErrInvalid) {
		t.Fatal("multiline command accepted")
	}
	if clientCalls.Load() != before {
		t.Fatal("invalid inputs reached Panel")
	}
	if applicationCalls.Load() == 0 || clientCalls.Load() == 0 {
		t.Fatal("missing exercised APIs")
	}
}
func TestTransportRejectsRedirectsAndDoesNotExposeSecrets(t *testing.T) {
	var leaked atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(302)
		_, _ = io.WriteString(w, "remote-password-secret")
	}))
	defer source.Close()
	client, err := NewApplication(source.URL, "application-secret", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetServer(context.Background(), 1)
	if err == nil || leaked.Load() || strings.Contains(err.Error(), "application-secret") || strings.Contains(err.Error(), "remote-password-secret") {
		t.Fatal("unsafe transport", err)
	}
	if _, err = NewApplication(source.URL, "key", false); err == nil {
		t.Fatal("production plaintext Panel accepted")
	}
	if _, err = NewApplication("https://user:password@panel.example.com", "key", false); err == nil {
		t.Fatal("URL credentials accepted")
	}
	if Rejected(&APIError{Status: 502}) || !Rejected(&APIError{Status: 422}) {
		t.Fatal("ambiguous and definitive errors misclassified")
	}
}

func TestApplicationReadsAndLifecycle(t *testing.T) {
	external := uuid.New()
	suspended, installed := false, true
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer application-secret" {
			t.Error("missing privileged credential")
		}
		switch r.Method {
		case "GET":
			attrs := map[string]any{"id": 12, "external_id": external, "identifier": "abcd1234", "node": 2, "user": 7, "egg": 3, "allocation": 51, "suspended": suspended, "status": nil, "limits": map[string]int{"memory": 2048, "disk": 10240, "cpu": 100}, "feature_limits": map[string]int{"backups": 2, "databases": 0}, "container": map[string]int{"installed": 0}}
			if installed {
				attrs["container"] = map[string]int{"installed": 1}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"attributes": attrs})
		case "POST":
			switch {
			case strings.HasSuffix(r.URL.Path, "/unsuspend"):
				suspended = false
			case strings.HasSuffix(r.URL.Path, "/suspend"):
				suspended = true
			case strings.HasSuffix(r.URL.Path, "/reinstall"):
				installed = false
			default:
				t.Error("unexpected lifecycle route")
			}
			w.WriteHeader(204)
		default:
			t.Error("unexpected method")
		}
	}))
	defer source.Close()
	client, err := NewApplication(source.URL, "application-secret", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := client.FindServerByExternalID(ctx, external)
	if err != nil || !s.Installed || s.Request.UserID != 7 || s.Request.Limits.Backups != 2 {
		t.Fatal("application server decode", err)
	}
	if err = client.SuspendServer(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	s, err = client.GetServer(ctx, s.ID)
	if err != nil || !s.Suspended {
		t.Fatal("suspend", err)
	}
	if err = client.UnsuspendServer(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	s, err = client.GetServer(ctx, s.ID)
	if err != nil || s.Suspended {
		t.Fatal("unsuspend", err)
	}
	if err = client.ReinstallServer(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	s, err = client.GetServer(ctx, s.ID)
	if err != nil || s.Installed {
		t.Fatal("reinstall", err)
	}
}
