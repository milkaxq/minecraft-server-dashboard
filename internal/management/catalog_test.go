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
	key := uuid.New()
	v := catalog.Version{ID: "release", ProjectID: "project", Name: "Test 1.0", Type: "release", Loaders: []string{"paper"}, Games: []string{"1.21.1"}, Files: []catalog.File{{URL: "https://cdn.modrinth.com/test.jar", Filename: "Test.jar", Size: int64(len(data)), Primary: true, Hashes: map[string]string{"sha512": hex.EncodeToString(h[:])}}}}
	c := catalog.New()
	c.HTTP.Transport = catalogTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		var content []byte
		if r.URL.Host == "cdn.modrinth.com" {
			downloads++
			progress, err := f.s.CatalogProgress(ctx, f.owner, f.id, key, "plugins")
			if err != nil {
				t.Fatal("pending progress inaccessible during download", err)
			}
			state := progress.(map[string]any)
			if state["status"] != "PENDING" || state["completed"] != 0 || state["total"] != 2 {
				t.Fatal("progress simulated or missing", state)
			}
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
	rejectedKey := uuid.New()
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, rejectedKey, "plugins", "project", strings.Repeat("a", 64), "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("stale preview installed", err)
	}
	if downloads != 0 {
		t.Fatal("stale preview downloaded")
	}
	rejected, err := f.s.CatalogProgress(ctx, f.owner, f.id, rejectedKey, "plugins")
	if err != nil || rejected.(map[string]any)["status"] != "REJECTED" {
		t.Fatal("failed operation lost its progress metadata", rejected, err)
	}

	for _, user := range []uuid.UUID{f.other, f.mod} {
		if _, err := f.s.CatalogInstalled(ctx, user, f.id, "plugins"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized installed list", err)
		}
		if _, err := f.s.CatalogProgress(ctx, user, f.id, key, "plugins"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized progress", err)
		}
	}
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, key, "plugins", "project", preview.Revision, "test"); err != nil {
		t.Fatal(err)
	}
	progress, err := f.s.CatalogProgress(ctx, f.owner, f.id, key, "plugins")
	if err != nil {
		t.Fatal(err)
	}
	if progress.(map[string]any)["status"] != "SUCCEEDED" || progress.(map[string]any)["completed"] != 2 {
		t.Fatal("completion progress", progress)
	}
	if _, err = f.s.CatalogProgress(ctx, f.admin, f.id, key, "plugins"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("another actor read private operation", err)
	}
	installed, err := f.s.CatalogInstalled(ctx, f.owner, f.id, "plugins")
	if err != nil || len(installed.(map[string]any)["projects"].([]string)) != 1 {
		t.Fatal("installed receipt missing", installed, err)
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
	delete(f.panel.files, "/plugins/Test.jar")
	installed, err = f.s.CatalogInstalled(ctx, f.owner, f.id, "plugins")
	if err != nil || len(installed.(map[string]any)["projects"].([]string)) != 0 {
		t.Fatal("deleted plugin still installed", installed, err)
	}
	f.panel.files["/plugins/Test.jar"] = []byte("changed")
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, uuid.New(), "plugins", "project", preview.Revision, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("existing file overwritten", err)
	}
}

func TestCatalogSearchUsesStoredServerVersionAndAuthorizesBeforeProvider(t *testing.T) {
	f := setup(t)
	calls := 0
	c := catalog.New()
	c.HTTP.Transport = catalogTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v2/search" || !strings.Contains(r.URL.Query().Get("facets"), `"versions:1.21.1"`) || !strings.Contains(r.URL.Query().Get("facets"), `"categories:paper"`) {
			t.Fatal("search did not use stored server software/version", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"hits":[],"total_hits":0}`))}, nil
	})
	f.s.Catalog = c
	for _, user := range []uuid.UUID{f.other, f.mod} {
		if _, err := f.s.CatalogSearch(context.Background(), user, f.id, "plugins", catalog.SearchOptions{Sort: "downloads"}); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized search", err)
		}
	}
	if calls != 0 {
		t.Fatal("unauthorized request reached provider")
	}
	if _, err := f.s.CatalogSearch(context.Background(), f.owner, f.id, "plugins", catalog.SearchOptions{Sort: "arbitrary"}); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("invalid sort accepted", err)
	}
	if calls != 0 {
		t.Fatal("invalid options reached provider")
	}
	page, err := f.s.CatalogSearch(context.Background(), f.owner, f.id, "plugins", catalog.SearchOptions{Sort: "downloads", Category: "utility", Offset: 12})
	if err != nil || page.Software != "PAPER" || page.MinecraftVersion != "1.21.1" || calls != 1 {
		t.Fatal("stored compatibility not enforced", page, err)
	}
}
