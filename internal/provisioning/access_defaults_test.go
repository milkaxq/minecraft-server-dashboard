package provisioning

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFirstInstallAccessDefaultsPreserveExistingSettings(t *testing.T) {
	for _, kind := range []string{"paper", "vanilla", "fabric", "forge", "neoforge", "spigot", "purpur"} {
		t.Run(kind, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "pterodactyl", "eggs", kind+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var egg struct {
				Scripts struct {
					Installation struct {
						Script string `json:"script"`
					} `json:"installation"`
				} `json:"scripts"`
			}
			if err = json.Unmarshal(data, &egg); err != nil {
				t.Fatal(err)
			}
			// Execute only the checked-in properties initialization block. No
			// download, installer or Minecraft process is executed by this test.
			block := regexp.MustCompile(`(?s)if \[ ! -f server\.properties \]; then\n.*?\nfi`).FindString(egg.Scripts.Installation.Script)
			if block == "" {
				t.Fatal("missing first-install properties defaults")
			}
			dir := t.TempDir()
			run := func() {
				c := exec.Command("sh", "-c", block)
				c.Dir = dir
				if output, e := c.CombinedOutput(); e != nil {
					t.Fatal(e, string(output))
				}
			}
			run()
			file := filepath.Join(dir, "server.properties")
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), "online-mode=false\n") || !strings.Contains(string(content), "white-list=false\n") {
				t.Fatal("wrong new-server access defaults", string(content))
			}
			existing := []byte("online-mode=true\nwhite-list=true\nmotd=Keep this\n")
			if err = os.WriteFile(file, existing, 0600); err != nil {
				t.Fatal(err)
			}
			run()
			content, err = os.ReadFile(file)
			if err != nil || string(content) != string(existing) {
				t.Fatal("reinstall overwrote owner settings", err)
			}
		})
	}
}
