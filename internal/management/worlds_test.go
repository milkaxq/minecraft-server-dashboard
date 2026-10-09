package management

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

type worldPanel struct {
	*memoryPanel
	failRename bool
}

func (p *worldPanel) RenameFile(_ context.Context, _ string, root, from, to string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.failRename {
		return errors.New("remote rename response lost")
	}
	a, b := path.Join(root, from), path.Join(root, to)
	for n, v := range p.files {
		if n == a || strings.HasPrefix(n, a+"/") {
			p.files[b+strings.TrimPrefix(n, a)] = v
			delete(p.files, n)
		}
	}
	for n := range p.folders {
		if n == a || strings.HasPrefix(n, a+"/") {
			p.folders[b+strings.TrimPrefix(n, a)] = true
			delete(p.folders, n)
		}
	}
	return nil
}
func (p *worldPanel) DeleteFile(_ context.Context, _ string, root, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	a := path.Join(root, name)
	for n := range p.files {
		if n == a || strings.HasPrefix(n, a+"/") {
			delete(p.files, n)
		}
	}
	for n := range p.folders {
		if n == a || strings.HasPrefix(n, a+"/") {
			delete(p.folders, n)
		}
	}
	return nil
}
func worldFixture(t *testing.T) (fixture, *worldPanel) {
	f := setup(t)
	p := &worldPanel{memoryPanel: f.panel}
	f.s.Panel = p
	for _, name := range []string{"world", "world_nether", "world_the_end"} {
		p.folders["/"+name] = true
		p.files["/"+name+"/level.dat"] = []byte("saved-world")
	}
	return f, p
}
func worldArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, value := range files {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(value)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func safety(t *testing.T, f fixture) WorldChange {
	t.Helper()
	ctx := context.Background()
	result, err := f.s.WorldBackup(ctx, f.owner, f.id, uuid.New(), "test")
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.s.World(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	return WorldChange{Backup: result.(map[string]any)["uuid"].(string), Confirmation: w.Name, Revision: w.Revision}
}
func TestWorldZIPRejectsUnsafeAndOversizedContents(t *testing.T) {
	for _, files := range []map[string]string{
		{"level.dat": "world", "../outside": "bad"}, {"level.dat": "world", "/outside": "bad"},
		{"level.dat": "world", "a\\b": "bad"}, {"level.dat": "world", "a": "file", "a/b": "bad"},
		{"wrong/level.dat": "bad"}, {"world/level.dat": "ok", "plugins/evil.jar": "bad"},
		{"level.dat": "ok", "world/level.dat": "ambiguous"}, {"readme.txt": "missing world"},
	} {
		if _, _, err := validateWorldZIP(worldArchive(t, files), "world", "PAPER"); !errors.Is(err, server.ErrInvalid) {
			t.Fatalf("unsafe archive: %v %v", files, err)
		}
	}
	for _, files := range []map[string]string{{"level.dat": "ok", "region/r.0.0.mca": "ok"}, {"world/level.dat": "ok", "world_nether/DIM-1/region/a.mca": "ok"}} {
		if _, _, err := validateWorldZIP(worldArchive(t, files), "world", "PAPER"); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "level.dat"}
	h.SetMode(0777 | 1<<27)
	w, _ := z.CreateHeader(h)
	w.Write([]byte("/outside"))
	z.Close()
	if _, _, err := validateWorldZIP(b.Bytes(), "world", "PAPER"); err == nil {
		t.Fatal("symlink accepted")
	}
	b.Reset()
	z = zip.NewWriter(&b)
	w, _ = z.Create("level.dat")
	w.Write([]byte("one"))
	w, _ = z.Create("level.dat")
	w.Write([]byte("two"))
	z.Close()
	if _, _, err := validateWorldZIP(b.Bytes(), "world", "PAPER"); err == nil {
		t.Fatal("duplicate accepted")
	}
	data := worldArchive(t, map[string]string{"level.dat": strings.Repeat("a", (32<<20)+1)})
	if _, _, err := validateWorldZIP(data, "world", "PAPER"); err == nil {
		t.Fatal("compressed bomb accepted")
	}
}
func TestWorldAuthorizationOfflineBackupQuotaAndConfirmation(t *testing.T) {
	f, _ := worldFixture(t)
	ctx := context.Background()
	for _, user := range []uuid.UUID{f.other, f.mod} {
		if _, err := f.s.World(ctx, user, f.id); !errors.Is(err, server.ErrNotFound) {
			t.Fatal(err)
		}
		if _, err := f.s.WorldBackup(ctx, user, f.id, uuid.New(), "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal(err)
		}
		if _, _, err := f.s.WorldDownload(ctx, user, f.id); !errors.Is(err, server.ErrNotFound) {
			t.Fatal(err)
		}
		if _, err := f.s.ChangeWorld(ctx, user, f.id, uuid.New(), WorldChange{Backup: uuid.NewString(), Revision: strings.Repeat("a", 64), Confirmation: "world"}, nil, false, "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal(err)
		}
	}
	if f.panel.calls != 0 {
		t.Fatal("unauthorized request reached Panel")
	}
	f.panel.Power(ctx, f.identifier, pterodactyl.PowerStart)
	if _, err := f.s.WorldBackup(ctx, f.owner, f.id, uuid.New(), "test"); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	f.panel.Power(ctx, f.identifier, pterodactyl.PowerStop)
	in := safety(t, f)
	bad := in
	bad.Confirmation = "wrong"
	if _, err := f.s.ChangeWorld(ctx, f.owner, f.id, uuid.New(), bad, nil, false, "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal(err)
	}
	bad = in
	bad.Backup = uuid.NewString()
	if _, err := f.s.ChangeWorld(ctx, f.owner, f.id, uuid.New(), bad, nil, false, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("foreign safety backup", err)
	}
	f.panel.backups[0].CompletedAt = nil
	if _, err := f.s.ChangeWorld(ctx, f.owner, f.id, uuid.New(), in, nil, false, "test"); !errors.Is(err, control.ErrBusy) {
		t.Fatal("incomplete backup", err)
	}
	if _, err := f.s.NewBackup(ctx, f.owner, f.id, uuid.New(), "Second", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.WorldBackup(ctx, f.owner, f.id, uuid.New(), "test"); !errors.Is(err, ErrLimit) {
		t.Fatal("quota bypass", err)
	}
	if string(f.panel.files["/world/level.dat"]) != "saved-world" {
		t.Fatal("world mutated without completed backup")
	}
}
func TestWorldDownloadUploadResetAndReplay(t *testing.T) {
	f, _ := worldFixture(t)
	ctx := context.Background()
	r, name, err := f.s.WorldDownload(ctx, f.admin, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeFile(ctx, f.owner, f.id, uuid.New(), FileMutation{Action: "create", Path: "/during-download"}, "test"); !errors.Is(err, control.ErrBusy) {
		t.Fatal("download lock missing", err)
	}
	data, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || name != "world.zip" || len(z.File) != 3 {
		t.Fatal(name, err)
	}
	// Streaming closes its lock asynchronously; acquire/release waits for the goroutine.
	eventuallyUnlocked(t, f)
	in := safety(t, f)
	key := uuid.New()
	archive := worldArchive(t, map[string]string{"level.dat": "new-world", "region/one.mca": "new-region"})
	if _, err = f.s.ChangeWorld(ctx, f.admin, f.id, key, in, archive, true, "test"); err != nil {
		t.Fatal(err)
	}
	if string(f.panel.files["/world/level.dat"]) != "new-world" || f.panel.folders["/world_nether"] {
		t.Fatal("dimensions not replaced")
	}
	calls := f.panel.calls
	if _, err = f.s.ChangeWorld(ctx, f.admin, f.id, key, in, archive, true, "test"); err != nil {
		t.Fatal("replay", err)
	}
	if f.panel.calls != calls+1 {
		t.Fatal("upload replay mutated remote files")
	}
	in = safety(t, f)
	key = uuid.New()
	if _, err = f.s.ChangeWorld(ctx, f.owner, f.id, key, in, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if f.panel.folders["/world"] {
		t.Fatal("reset kept current world")
	}
	if _, err = f.s.ChangeWorld(ctx, f.owner, f.id, key, in, nil, false, "test"); err != nil {
		t.Fatal("reset replay", err)
	}
	if _, ok := f.panel.files["/server.properties"]; !ok {
		t.Fatal("reset changed settings")
	}
}
func TestWorldSafetyBackupInvalidatedByStartOrFiles(t *testing.T) {
	for _, mutation := range []string{"start", "file"} {
		t.Run(mutation, func(t *testing.T) {
			f, _ := worldFixture(t)
			ctx := context.Background()
			in := safety(t, f)
			if mutation == "start" {
				_, err := f.db.Exec(ctx, `INSERT INTO control_requests(id,server_id,user_id,idempotency_key,action,fingerprint,status,request_id) VALUES($1,$2,$3,$4,'start','test','SUCCEEDED','test')`, uuid.New(), f.id, f.owner, uuid.New())
				if err != nil {
					t.Fatal(err)
				}
			} else {
				f.panel.files["/world/level.dat"] = []byte("changed-world")
			}
			if _, err := f.s.ChangeWorld(ctx, f.owner, f.id, uuid.New(), in, nil, false, "test"); !errors.Is(err, ErrChanged) {
				t.Fatal("stale backup allowed", err)
			}
		})
	}
}
func TestUnknownWorldChangeNeverReplaysAndBlocksStart(t *testing.T) {
	f, p := worldFixture(t)
	ctx := context.Background()
	in := safety(t, f)
	key := uuid.New()
	p.failRename = true
	if _, err := f.s.ChangeWorld(ctx, f.owner, f.id, key, in, nil, false, "test"); !errors.Is(err, control.ErrUnknown) {
		t.Fatal(err)
	}
	calls := p.calls
	if _, err := f.s.ChangeWorld(ctx, f.owner, f.id, key, in, nil, false, "test"); !errors.Is(err, control.ErrUnknown) {
		t.Fatal(err)
	}
	if p.calls != calls+1 {
		t.Fatal("unknown reset replayed")
	}
	if err := f.s.Control.Power(ctx, f.owner, f.id, uuid.New(), pterodactyl.PowerStart, "", "test"); !errors.Is(err, control.ErrUnknown) {
		t.Fatal("started uncertain world", err)
	}
	if string(p.files["/world/level.dat"]) != "saved-world" {
		t.Fatal("original world lost on failed staging")
	}
}

func eventuallyUnlocked(t *testing.T, f fixture) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, e := f.db.Acquire(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		var held bool
		e = conn.QueryRow(context.Background(), "SELECT pg_try_advisory_lock(hashtextextended($1,0))", "server:"+f.id.String()).Scan(&held)
		if e != nil {
			conn.Release()
			t.Fatal(e)
		}
		if held {
			var ok bool
			_ = conn.QueryRow(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1,0))", "server:"+f.id.String()).Scan(&ok)
			conn.Release()
			return
		}
		conn.Release()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("world download did not release lock")
}

func TestWorldUploadEnforcesPurchasedDiskBeforeStaging(t *testing.T) {
	f, _ := worldFixture(t)
	ctx := context.Background()
	in := safety(t, f)
	if _, err := f.db.Exec(ctx, "UPDATE servers SET disk_mb=1 WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	archive := worldArchive(t, map[string]string{"level.dat": "new-world", "region/one.mca": strings.Repeat("a", 2<<20)})
	if _, err := f.s.ChangeWorld(ctx, f.owner, f.id, uuid.New(), in, archive, true, "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("disk limit bypassed", err)
	}
	if string(f.panel.files["/world/level.dat"]) != "saved-world" {
		t.Fatal("world changed before capacity validation")
	}
	for name := range f.panel.folders {
		if strings.HasPrefix(name, "/.hosting-world-") {
			t.Fatal("staged an over-quota world")
		}
	}
}

func TestWorldDiscoveryRejectsPropertiesTraversalAndSymlinks(t *testing.T) {
	for _, unsafe := range []string{"../other", "folder/world", ".private", "plugins"} {
		t.Run(unsafe, func(t *testing.T) {
			f, _ := worldFixture(t)
			f.panel.files["/server.properties"] = []byte("level-name=" + unsafe + "\n")
			if _, err := f.s.World(context.Background(), f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
				t.Fatal("unsafe world path accepted", err)
			}
		})
	}
	f, _ := worldFixture(t)
	f.panel.symlinks["/world/level.dat"] = true
	if _, err := f.s.World(context.Background(), f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("world symlink followed", err)
	}
}

func TestWorldZIPChecksCRCAndVanillaDimensions(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.CreateHeader(&zip.FileHeader{Name: "level.dat", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("world"))
	z.Close()
	data := b.Bytes()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := r.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	data[offset] ^= 0xff
	if _, _, err = validateWorldZIP(data, "world", "PAPER"); err == nil {
		t.Fatal("corrupt ZIP accepted")
	}
	archive := worldArchive(t, map[string]string{"world/level.dat": "world", "world_nether/level.dat": "nether"})
	if _, _, err = validateWorldZIP(archive, "world", "VANILLA"); err == nil {
		t.Fatal("Paper layout accepted for Vanilla")
	}
	archive = worldArchive(t, map[string]string{"level.dat": "world", "DIM-1/region/one.mca": "nether"})
	if _, _, err = validateWorldZIP(archive, "world", "VANILLA"); err != nil {
		t.Fatal("Vanilla dimension rejected", err)
	}
}
