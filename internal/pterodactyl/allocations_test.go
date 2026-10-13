package pterodactyl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllocationAPIsKeepCredentialsAndPurchasedLimitsSeparate(t *testing.T) {
	limits := Limits{RAMMB: 2048, DiskMB: 10240, CPU: 100, Backups: 2, Databases: 1}
	writes := 0
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/application/servers/12":
			if r.Header.Get("Authorization") != "Bearer application-secret" || r.URL.Query().Get("include") != "allocations" {
				t.Error("wrong discovery credential/include")
			}
			json.NewEncoder(w).Encode(map[string]any{"attributes": map[string]any{"id": 12, "relationships": map[string]any{"allocations": map[string]any{"data": []any{map[string]any{"attributes": map[string]int{"id": 51}}}}}}})
		case "/api/application/servers/12/build":
			if r.Method != "PATCH" || r.Header.Get("Authorization") != "Bearer application-secret" {
				t.Error("wrong mutation boundary")
			}
			var body struct {
				Primary       int            `json:"allocation"`
				Limits        map[string]int `json:"limits"`
				FeatureLimits map[string]int `json:"feature_limits"`
				Add           []int          `json:"add_allocations"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Primary != 51 || body.Limits["memory"] != 2048 || body.Limits["cpu"] != 100 || body.Limits["disk"] != 10240 || body.FeatureLimits["backups"] != 2 || body.FeatureLimits["databases"] != 1 || body.FeatureLimits["allocations"] != 0 || len(body.Add) != 1 || body.Add[0] != 52 {
				t.Error("untrusted limits/ports", body)
			}
			writes++
			w.WriteHeader(204)
		case "/api/client/servers/abcdef12/network/allocations":
			if r.Header.Get("Authorization") != "Bearer client-secret" {
				t.Error("application credential used by Client API")
			}
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"attributes": map[string]int{"id": 51}}, map[string]any{"attributes": map[string]int{"id": 52}}}})
		default:
			t.Error("unexpected endpoint", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer panel.Close()
	app, err := NewApplication(panel.URL, "application-secret", true)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := app.ServerAllocations(context.Background(), 12)
	if err != nil || len(ids) != 1 || ids[0] != 51 {
		t.Fatal(ids, err)
	}
	if err = app.AssignAllocation(context.Background(), AdditionalAllocationRequest{ServerID: 12, PrimaryAllocationID: 51, AdditionalAllocationID: 52, Limits: limits}); err != nil {
		t.Fatal(err)
	}
	client, err := NewServerClient(panel.URL, "client-secret", true)
	if err != nil {
		t.Fatal(err)
	}
	ids, err = client.NetworkAllocations(context.Background(), "abcdef12")
	if err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
	if err = app.AssignAllocation(context.Background(), AdditionalAllocationRequest{ServerID: 12, PrimaryAllocationID: 51, AdditionalAllocationID: 52}); err == nil || writes != 1 {
		t.Fatal("invalid limits reached Panel")
	}
}
