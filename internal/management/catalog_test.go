package management

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/server"

	"github.com/google/uuid"
)

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCatalogOwnershipCompatibilityInstallAndReplay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var jar bytes.Buffer
	z := zip.NewWriter(&jar)
	w, _ := z.Create("plugin.yml")
	_, _ = w.Write([]byte("name: Test\nversion: 1.0\nmain: test.Main\n"))
	_ = z.Close()
	data := jar.Bytes()
	h := sha512.Sum512(data)
	requests := 0
	downloads := 0
	v := catalog.Version{ID: "release", ProjectID: "project", Name: "Test 1.0", Type: "release", Loaders: []string{"paper"}, Games: []string{"1.21.1"}, Files: []catalog.File{{URL: "https://cdn.modrinth.com/test.jar", Filename: "Test.jar", Size: int64(len(data)), Primary: true, Hashes: map[string]string{"sha512": hex.EncodeToString(h[:])}}}}
	c := catalog.New()
	c.HTTP.Transport = catalogTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		var content []byte
		if r.URL.Host == "cdn.modrinth.com" {
			downloads++
			content = data
		} else {
			switch r.URL.Path {
			case "/v2/project/project/version":
				if r.URL.Query().Get("loaders") != `["paper"]` || r.URL.Query().Get("game_versions") != `["1.21.1"]` {
					t.Fatal("frontend did not resolve server resources correctly")
				}
				content, _ = json.Marshal([]catalog.Version{v})
			case "/v2/project/project":
				content = []byte(`{"server_side":"required"}`)
			default:
				t.Fatal("unexpected catalog endpoint", r.URL)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(content))}, nil
	})
	f.s.Catalog = c
	for _, user := range []uuid.UUID{f.other, f.mod} {
		if _, err := f.s.CatalogPreview(ctx, user, f.id, "plugins", "project"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized catalog preview", err)
		}
	}
	for _, user := range []uuid.UUID{f.other, f.mod} {
		if _, err := f.s.CatalogInstall(ctx, user, f.id, uuid.New(), "plugins", "project", strings.Repeat("a", 64), "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized catalog installation", err)
		}
	}
	if requests != 0 {
		t.Fatal("unauthorized user reached catalog")
	}
	preview, err := f.s.CatalogPreview(ctx, f.owner, f.id, "plugins", "project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, uuid.New(), "plugins", "project", strings.Repeat("a", 64), "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("stale preview installed", err)
	}
	if downloads != 0 {
		t.Fatal("stale preview downloaded")
	}
	key := uuid.New()
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, key, "plugins", "project", preview.Revision, "test"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.panel.files["/plugins/Test.jar"], data) {
		t.Fatal("verified jar not uploaded")
	}
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, key, "plugins", "project", preview.Revision, "test"); err != nil {
		t.Fatal(err)
	}
	if downloads != 1 {
		t.Fatal("replay repeated download")
	}
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, uuid.New(), "plugins", "project", preview.Revision, "test"); err != nil {
		t.Fatal("existing identical dependency should be retained", err)
	}
	if downloads != 1 {
		t.Fatal("identical file downloaded again")
	}
	f.panel.files["/plugins/Test.jar"] = []byte("changed")
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, uuid.New(), "plugins", "project", preview.Revision, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("existing file overwritten", err)
	}
}
