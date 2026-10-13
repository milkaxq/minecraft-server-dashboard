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
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

func TestCatalogUpdateVerifiesOldFileReplacesAndReplays(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	jar := func(version string) []byte {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		w, _ := z.Create("plugin.yml")
		_, _ = w.Write([]byte("name: Test\nversion: " + version + "\nmain: test.Main\n"))
		_ = z.Close()
		return b.Bytes()
	}
	oldData, newData := jar("1.0"), jar("2.0")
	currentData := oldData
	version := func(id, name string, data []byte) catalog.Version {
		h := sha512.Sum512(data)
		return catalog.Version{ID: id, ProjectID: "project", Name: id, Type: "release", Loaders: []string{"paper"}, Games: []string{"1.21.1"}, Files: []catalog.File{{URL: "https://cdn.modrinth.com/test.jar", Filename: name, Size: int64(len(data)), Primary: true, Hashes: map[string]string{"sha512": hex.EncodeToString(h[:])}}}}
	}
	v := version("v1", "Test-1.jar", oldData)
	downloads := 0
	c := catalog.New()
	c.HTTP.Transport = catalogTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case r.URL.Host == "cdn.modrinth.com":
			body = currentData
			downloads++
		case r.URL.Path == "/v2/project/project":
			body = []byte(`{"server_side":"required","client_side":"unsupported","title":"Test"}`)
		default:
			body, _ = json.Marshal([]catalog.Version{v})
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	f.s.Catalog = c
	preview, err := f.s.CatalogPreview(ctx, f.owner, f.id, "plugins", "project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, uuid.New(), "plugins", "project", preview.Revision, BackupDecision{WithoutBackup: true}, "test"); err != nil {
		t.Fatal(err)
	}
	v = version("v2", "Test-2.jar", newData)
	currentData = newData
	if _, err = f.s.CatalogUpdates(ctx, f.other, f.id, "plugins"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	updates, err := f.s.CatalogUpdates(ctx, f.owner, f.id, "plugins")
	if err != nil || len(updates) != 1 || updates[0].Status != "available" {
		t.Fatal(updates, err)
	}
	preview, err = f.s.CatalogPreview(ctx, f.owner, f.id, "plugins", "project")
	if err != nil || len(preview.Replaces) != 1 || preview.Replaces[0].Filename != "Test-1.jar" {
		t.Fatal(preview, err)
	}
	changed := bytes.Clone(oldData)
	changed[len(changed)-1] ^= 1
	f.panel.files["/plugins/Test-1.jar"] = changed
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, uuid.New(), "plugins", "project", preview.Revision, BackupDecision{WithoutBackup: true}, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("modified file replaced", err)
	}
	if downloads != 1 {
		t.Fatal("download before verification")
	}
	f.panel.files["/plugins/Test-1.jar"] = oldData
	key := uuid.New()
	for i := 0; i < 2; i++ {
		if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, key, "plugins", "project", preview.Revision, BackupDecision{WithoutBackup: true}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := f.panel.files["/plugins/Test-1.jar"]; ok {
		t.Fatal("old jar remains")
	}
	if !bytes.Equal(f.panel.files["/plugins/Test-2.jar"], newData) || downloads != 2 {
		t.Fatal("update or replay incorrect")
	}
	updates, err = f.s.CatalogUpdates(ctx, f.owner, f.id, "plugins")
	if err != nil || len(updates) != 1 || updates[0].Status != "current" {
		t.Fatal(updates, err)
	}

	// A tracked release may reuse its old filename; hash checks authorize only that
	// exact old content, and the new file must not be deleted as obsolete afterward.
	thirdData := jar("3.0")
	v = version("v3", "Test-2.jar", thirdData)
	currentData = thirdData
	preview, err = f.s.CatalogPreview(ctx, f.owner, f.id, "plugins", "project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, uuid.New(), "plugins", "project", preview.Revision, BackupDecision{WithoutBackup: true}, "test"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.panel.files["/plugins/Test-2.jar"], thirdData) {
		t.Fatal("same-filename replacement lost")
	}
	fourthData := jar("4.0")
	v = version("v4", "Test-4.jar", fourthData)
	currentData = fourthData
	preview, err = f.s.CatalogPreview(ctx, f.owner, f.id, "plugins", "project")
	if err != nil {
		t.Fatal(err)
	}
	f.s.Panel = &uncertainCatalogDelete{f.panel}
	key = uuid.New()
	for i := 0; i < 2; i++ {
		if _, err = f.s.CatalogInstall(ctx, f.owner, f.id, key, "plugins", "project", preview.Revision, BackupDecision{WithoutBackup: true}, "test"); !errors.Is(err, control.ErrUnknown) {
			t.Fatal("uncertain replacement repeated", err)
		}
	}
	if !bytes.Equal(f.panel.files["/plugins/Test-2.jar"], thirdData) || !bytes.Equal(f.panel.files["/plugins/Test-4.jar"], fourthData) {
		t.Fatal("uncertain files not retained for inspection")
	}
	if downloads != 4 {
		t.Fatal("uncertain request repeated remote install", downloads)
	}
}

type uncertainCatalogDelete struct{ *memoryPanel }

func (p *uncertainCatalogDelete) DeleteFile(context.Context, string, string, string) error {
	return errors.New("connection lost")
}
