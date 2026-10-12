package provisioning

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

type receiptFiles struct {
	entries []pterodactyl.FileEntry
	receipt []byte
	err     error
}

func (r receiptFiles) ListFiles(context.Context, string, string) ([]pterodactyl.FileEntry, error) {
	return r.entries, r.err
}
func (r receiptFiles) ReadFile(context.Context, string, string) ([]byte, error) {
	return r.receipt, r.err
}
func TestInstallationReceiptRejectsIncompleteAndWrongRelease(t *testing.T) {
	files := receiptFiles{entries: []pterodactyl.FileEntry{{Name: ".blockyard-install.json", IsFile: true, Size: 100}, {Name: "unix_args.txt", IsFile: true, Size: 100}}, receipt: []byte(`{"software":"FORGE","minecraft_version":"1.21.1","loader_version":"1.21.1-52.1.0"}`)}
	env := map[string]string{"FORGE_VERSION": "1.21.1-52.1.0"}
	verify := func(f receiptFiles) error {
		return (ReceiptVerifier{Files: f}).VerifyInstallation(context.Background(), "abcd1234", "FORGE", "1.21.1", env)
	}
	if err := verify(files); err != nil {
		t.Fatal(err)
	}
	bad := files
	bad.receipt = []byte(`{"software":"FORGE","minecraft_version":"1.20.1","loader_version":"1.21.1-52.1.0"}`)
	if !errors.Is(verify(bad), ErrInstallation) {
		t.Fatal("wrong Minecraft version accepted")
	}
	bad = files
	bad.receipt = []byte(`{"software":"FORGE","minecraft_version":"1.21.1","loader_version":"other"}`)
	if !errors.Is(verify(bad), ErrInstallation) {
		t.Fatal("wrong loader accepted")
	}
	bad = files
	bad.entries = nil
	if !errors.Is(verify(bad), ErrInstallation) {
		t.Fatal("missing launcher accepted")
	}
	bad = files
	bad.entries = append([]pterodactyl.FileEntry{}, files.entries...)
	bad.entries[0].IsSymlink = true
	if !errors.Is(verify(bad), ErrInstallation) {
		t.Fatal("symlink receipt accepted")
	}
	bad = files
	bad.err = errors.New("network failure")
	if errors.Is(verify(bad), ErrInstallation) || verify(bad) == nil {
		t.Fatal("transient network error classified as failed installation")
	}
}

type rejectInstallation struct{}

func (rejectInstallation) VerifyInstallation(context.Context, string, string, string, map[string]string) error {
	return ErrInstallation
}
func TestFailedReceiptAndAdministratorRetry(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.worker.Verifier = rejectInstallation{}
	template := SoftwareTemplate{ServerType: "FABRIC", EggID: 2, Image: "java21", Startup: "launcher", VersionVariable: "MC_VERSION", Versions: []string{"1.21.1"}}
	if err := ConfigureSoftware(ctx, f.db, f.admin, template); err != nil {
		t.Fatal(err)
	}
	s, err := (server.Service{DB: f.db}).Create(ctx, f.owner, server.CreateInput{PlanID: uuid.MustParse("10000000-0000-4000-8000-000000000001"), Name: "Incomplete installer", ServerType: "FABRIC", MinecraftVersion: "1.21.1"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Failed)
	f.capacity(t, 2048, 10240, 100)
	// Failed remote servers retain reservations until a confirmed remote deletion.
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Provisioning)
	f.worker.Verifier = acceptInstallation{}
	f.due(t)
	f.once(t)
	f.state(t, s, server.Running)
	if f.panel.creates.Load() != 1 {
		t.Fatal("retry created another remote server")
	}
}

func TestReceiptVerifierMissingDoesNotConsumeCapacity(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if err := ConfigureSoftware(ctx, f.db, f.admin, SoftwareTemplate{ServerType: "FABRIC", EggID: 2, Image: "java21", Startup: "launcher", VersionVariable: "MC_VERSION", Versions: []string{"1.21.1"}}); err != nil {
		t.Fatal(err)
	}
	s, err := (server.Service{DB: f.db}).Create(ctx, f.owner, server.CreateInput{PlanID: uuid.MustParse("10000000-0000-4000-8000-000000000001"), Name: "Missing verification configuration", ServerType: "FABRIC", MinecraftVersion: "1.21.1"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	f.enqueue(t, s, jobs.Provision)
	f.once(t)
	f.state(t, s, server.Failed)
	f.capacity(t, 0, 0, 0)
	if f.panel.creates.Load() != 0 {
		t.Fatal("remote server created without verification configuration")
	}
}
