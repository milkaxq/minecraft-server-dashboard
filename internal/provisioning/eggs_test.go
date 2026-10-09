package provisioning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedInstallersPreserveIsolationAndConsent(t *testing.T) {
	for _, kind := range []string{"spigot", "purpur", "fabric", "forge", "neoforge"} {
		t.Run(kind, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../deploy/pterodactyl/eggs", kind+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var egg struct {
				Startup string            `json:"startup"`
				Images  map[string]string `json:"docker_images"`
				Scripts struct {
					Installation struct {
						Container string `json:"container"`
						Script    string `json:"script"`
					} `json:"installation"`
				} `json:"scripts"`
				Variables []struct {
					Editable bool `json:"user_editable"`
				} `json:"variables"`
			}
			if err = json.Unmarshal(data, &egg); err != nil {
				t.Fatal(err)
			}
			for _, image := range egg.Images {
				if image == egg.Scripts.Installation.Container {
					t.Fatal("privileged installer reused as customer runtime")
				}
			}
			if !strings.Contains(egg.Startup, "MaxRAMPercentage=75.0") {
				t.Fatal("Java native memory headroom removed")
			}
			if strings.Contains(egg.Scripts.Installation.Script, "eula=true") || strings.Contains(egg.Scripts.Installation.Script, "eval ") {
				t.Fatal("implicit consent or executable download input")
			}
			if !strings.Contains(egg.Scripts.Installation.Script, "set -euo pipefail") || !strings.Contains(egg.Scripts.Installation.Script, "--fail") {
				t.Fatal("installer masks failed downloads")
			}
			for _, v := range egg.Variables {
				if v.Editable {
					t.Fatal("customer can change operator-managed loader")
				}
			}
		})
	}
}
