package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"minecraft-hosting/internal/server"
)

func TestStartupRecognizesProblemsWithoutReturningLogContents(t *testing.T) {
	for _, tc := range []struct{ log, code string }{
		{"Starting minecraft server version 26.3\nUnknownDependencyException: SecretTokenDependency", "MISSING_DEPENDENCY"},
		{"UnknownDependencyException: MissingLibrary\nStarting minecraft server version 26.3", "MISSING_DEPENDENCY"},
		{"UnsupportedClassVersionError password=secret", "JAVA_VERSION"},
		{"You need to agree to the EULA in order to run the server", "EULA_REQUIRED"},
		{"InvalidPluginException private-key=secret", "INCOMPATIBLE_ADDON"},
		{"World was saved with a newer version", "WORLD_VERSION"},
		{"java.lang.OutOfMemoryError", "MEMORY"},
		{"FAILED TO BIND TO PORT secret-host", "PORT_CONFLICT"},
		{"[Server thread/ERROR]: secret-unknown-error", "UNKNOWN_ERROR"},
	} {
		issues := startupIssues(tc.log)
		if len(issues) == 0 || issues[0].Code != tc.code {
			t.Fatal(tc, issues)
		}
		encoded, _ := json.Marshal(issues)
		if strings.Contains(strings.ToLower(string(encoded)), "secret") {
			t.Fatal("raw log leaked", string(encoded))
		}
	}
	if got := startupIssues("Starting minecraft server version 1.21\nOutOfMemoryError\nDone (1.2s)! For help, type help\nStarting minecraft server version 26.3\nDone (1.2s)! For help, type help\nInvalidPluginException"); len(got) != 0 {
		t.Fatal("historical or runtime error treated as startup", got)
	}
}
func TestStartupOwnershipAndSafeMissingLogFallback(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.s.Startup(ctx, f.other, f.id); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	if f.panel.calls != 0 {
		t.Fatal("foreign startup check reached Panel")
	}
	result, err := f.s.Startup(ctx, f.mod, f.id)
	if err != nil || result.Checked || result.Reason == "" {
		t.Fatal(result, err)
	}
}
