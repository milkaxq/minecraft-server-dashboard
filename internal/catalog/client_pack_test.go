package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientPackPinsVoiceReleaseAndIncludesClientOnlyDependencies(t *testing.T) {
	c := New()
	file := File{Filename: "voice.jar", URL: "https://cdn.modrinth.com/voice.jar", Primary: true, Size: 10, Hashes: map[string]string{"sha512": strings.Repeat("a", 128), "sha1": strings.Repeat("b", 40)}}
	serverVersion := Version{ID: "server-release", ProjectID: "9eGKb6K1", Number: "bukkit-2.6.24", Type: "release", Loaders: []string{"paper"}, Games: []string{"26.3"}}
	clientVersion := Version{ID: "client-release", ProjectID: "9eGKb6K1", Number: "fabric-2.6.24+26.3", Type: "release", Loaders: []string{"fabric"}, Games: []string{"26.3"}, Files: []File{file}, Dependencies: []Dependency{{VersionID: "dependency", Type: "required"}}}
	dep := clientVersion
	dep.ID = "dependency"
	dep.ProjectID = "library"
	dep.Dependencies = nil
	dep.Files = []File{file}
	dep.Files[0].Filename = "library.jar"
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		var result any
		switch r.URL.Path {
		case "/v2/version/server-release":
			result = serverVersion
		case "/v2/project/9eGKb6K1/version":
			result = []Version{clientVersion}
		case "/v2/version/client-release":
			result = clientVersion
		case "/v2/version/dependency":
			result = dep
		case "/v2/project/library":
			result = map[string]string{"client_side": "required", "server_side": "unsupported"}
		case "/v2/project/9eGKb6K1":
			result = map[string]string{"client_side": "required", "server_side": "required"}
		default:
			t.Fatal("unexpected request", r.URL)
		}
		data, _ := json.Marshal(result)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})
	files, err := c.ResolveClient(context.Background(), "9eGKb6K1", "server-release", "fabric", "26.3")
	if err != nil || len(files) != 2 || files[0].ProjectID != "library" || files[1].SHA1 == "" {
		t.Fatal(files, err)
	}
	clientVersion.Type = "beta"
	if _, err = c.ResolveClient(context.Background(), "9eGKb6K1", "server-release", "fabric", "26.3"); !errors.Is(err, ErrPreviewClient) {
		t.Fatal("beta client build did not explain the stable-only policy", err)
	}
	clientVersion.Type = "release"
	clientVersion.Number = "fabric-2.6.25+26.3"
	if _, err = c.ResolveClient(context.Background(), "9eGKb6K1", "server-release", "fabric", "26.3"); err == nil {
		t.Fatal("silently substituted a different client release")
	}
}
