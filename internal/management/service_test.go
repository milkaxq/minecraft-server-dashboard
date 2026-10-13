package management

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

// memoryPanel is a test fixture, never a production substitute for Wings.
type memoryPanel struct {
	*pterodactyl.Mock
	mu                       sync.Mutex
	calls, created, restored int
	files                    map[string][]byte
	folders                  map[string]bool
	symlinks                 map[string]bool
	backups                  []pterodactyl.Backup
	uncertain                bool
	busy                     bool
}

func (p *memoryPanel) Ready(context.Context, string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.busy {
		return pterodactyl.ErrConflict
	}
	return nil
}
func (p *memoryPanel) ListFiles(_ context.Context, _ string, dir string) ([]pterodactyl.FileEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	out := []pterodactyl.FileEntry{}
	for name, data := range p.files {
		if path.Dir(name) == dir {
			out = append(out, pterodactyl.FileEntry{Name: path.Base(name), Size: int64(len(data)), IsFile: true, IsSymlink: p.symlinks[name]})
		}
	}
	for name := range p.folders {
		if path.Dir(name) == dir {
			out = append(out, pterodactyl.FileEntry{Name: path.Base(name), IsSymlink: p.symlinks[name]})
		}
	}
	return out, nil
}
func (p *memoryPanel) ReadFile(_ context.Context, _ string, file string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	v, ok := p.files[file]
	if !ok {
		return nil, pterodactyl.ErrNotFound
	}
	return bytes.Clone(v), nil
}
func (p *memoryPanel) WriteFile(_ context.Context, _ string, file string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.files[file] = bytes.Clone(data)
	return nil
}
func (p *memoryPanel) CreateFolder(_ context.Context, _ string, root, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.folders[path.Join(root, name)] = true
	return nil
}
func (p *memoryPanel) RenameFile(_ context.Context, _ string, root, from, to string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	a, b := path.Join(root, from), path.Join(root, to)
	p.files[b] = p.files[a]
	delete(p.files, a)
	return nil
}
func (p *memoryPanel) DeleteFile(_ context.Context, _ string, root, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	delete(p.files, path.Join(root, name))
	return nil
}
func (p *memoryPanel) UploadFile(ctx context.Context, id, dir, name string, data []byte) error {
	return p.WriteFile(ctx, id, path.Join(dir, name), data)
}
func (p *memoryPanel) DownloadFile(ctx context.Context, id, file string) (io.ReadCloser, error) {
	data, err := p.ReadFile(ctx, id, file)
	return io.NopCloser(bytes.NewReader(data)), err
}
func (p *memoryPanel) ListBackups(context.Context, string) ([]pterodactyl.Backup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return append([]pterodactyl.Backup{}, p.backups...), nil
}
func (p *memoryPanel) CreateBackup(_ context.Context, _ string, name string) (pterodactyl.Backup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.created++
	done := "2026-10-07T10:00:00Z"
	b := pterodactyl.Backup{UUID: uuid.NewString(), Name: name, Successful: true, Locked: true, CompletedAt: &done}
	p.backups = append(p.backups, b)
	if p.uncertain {
		return b, errors.New("lost private response")
	}
	return b, nil
}
func (p *memoryPanel) RestoreBackup(context.Context, string, string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.restored++
	return nil
}
func (p *memoryPanel) DeleteBackup(_ context.Context, _ string, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	for i, b := range p.backups {
		if b.UUID == id {
			p.backups = append(p.backups[:i], p.backups[i+1:]...)
			return nil
		}
	}
	return pterodactyl.ErrNotFound
}
func (p *memoryPanel) DownloadBackup(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("backup")), nil
}

type fixture struct {
	s                            *Service
	panel                        *memoryPanel
	db                           *pgxpool.Pool
	owner, admin, mod, other, id uuid.UUID
	identifier                   string
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := testutil.Database(t)
	ctx := context.Background()
	f := fixture{db: db, owner: uuid.New(), admin: uuid.New(), mod: uuid.New(), other: uuid.New(), id: uuid.New()}
	for _, user := range []uuid.UUID{f.owner, f.admin, f.mod, f.other} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'test-only','Player',now())", user, user.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	p := &memoryPanel{Mock: pterodactyl.NewMock(), files: map[string][]byte{"/server.properties": []byte("# Keep me\nlevel-name=world\nserver-port=25565\nmotd=First\nmotd=Hello\\u0021\nonline-mode=true\nmax-players=10\n")}, folders: map[string]bool{}, symlinks: map[string]bool{}}
	user, err := p.CreateUser(ctx, pterodactyl.UserRequest{ExternalID: f.owner, Email: "player@example.com", Name: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := p.CreateServer(ctx, pterodactyl.ServerRequest{ExternalID: f.id, UserID: user.ID, NodeID: 1, AllocationID: 1, EggID: 1, Name: "World", Image: "java", Startup: "minecraft", Limits: pterodactyl.Limits{RAMMB: 2048, DiskMB: 10240, CPU: 100, Backups: 2}})
	if err != nil {
		t.Fatal(err)
	}

	node := uuid.New()
	if _, err = db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',200,4096,20480)", node); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO servers(id,owner_id,node_id,plan_id,pterodactyl_server_id,pterodactyl_identifier,name,status,minecraft_version,server_type,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,$5,'10000000-0000-4000-8000-000000000001',$3,$4,'World','RUNNING','1.21.1','PAPER',2048,10240,100,2,0)`, f.id, f.owner, remote.ID, remote.Identifier, node); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,'ADMIN'),($1,$3,'MODERATOR')", f.id, f.admin, f.mod); err != nil {
		t.Fatal(err)
	}
	f.panel = p
	f.identifier = remote.Identifier
	f.s = &Service{DB: db, Control: &control.Service{DB: db, Panel: p}, Panel: p}
	return f
}
func TestAuthorizationBeforePanelAndFileSafety(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, user := range []uuid.UUID{f.other, f.mod} {
		checks := []func() error{
			func() error { _, e := f.s.Files(ctx, user, f.id, "/"); return e }, func() error { _, e := f.s.Text(ctx, user, f.id, "/server.properties"); return e }, func() error { _, e := f.s.Settings(ctx, user, f.id); return e }, func() error { _, e := f.s.Backups(ctx, user, f.id); return e }, func() error { _, e := f.s.Plugins(ctx, user, f.id); return e }, func() error {
				_, e := f.s.ChangeFile(ctx, user, f.id, uuid.New(), FileMutation{Action: "create", Path: "/new"}, "test")
				return e
			}, func() error { _, e := f.s.NewBackup(ctx, user, f.id, uuid.New(), "backup", "test"); return e }, func() error {
				_, e := f.s.Upload(ctx, user, f.id, uuid.New(), "/", "new", []byte("a"), false, BackupDecision{WithoutBackup: true}, "test")
				return e
			}, func() error { _, _, e := f.s.Download(ctx, user, f.id, "/server.properties"); return e }, func() error {
				_, e := f.s.ChangeBackup(ctx, user, f.id, uuid.New(), uuid.NewString(), "restore", "bad", "test")
				return e
			}}
		for i, check := range checks {
			err := check()
			if i == 9 {
				if !errors.Is(err, server.ErrInvalid) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, server.ErrNotFound) {
				t.Fatalf("check %d: %v", i, err)
			}
		}
	}
	if f.panel.calls != 0 {
		t.Fatal("unauthorized request reached Panel", f.panel.calls)
	}
	if _, err := f.s.Files(ctx, f.owner, f.id, "/../other"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal(err)
	}
	f.panel.folders["/linked"] = true
	f.panel.symlinks["/linked"] = true
	if _, err := f.s.Files(ctx, f.owner, f.id, "/linked"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("symlink followed", err)
	}
	if _, err := f.s.ChangeFile(ctx, f.owner, f.id, uuid.New(), FileMutation{Action: "delete", Path: "/server.properties", Confirmation: "wrong"}, "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("unconfirmed deletion", err)
	}
	if _, err := f.s.ChangeFile(ctx, f.owner, f.id, uuid.New(), FileMutation{Action: "create", Path: "/server.properties"}, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("overwrite permitted", err)
	}
}

type revokeBeforeMutationPanel struct {
	*memoryPanel
	revoke func(context.Context) error
}

func (p *revokeBeforeMutationPanel) Ready(ctx context.Context, id string) error {
	if err := p.memoryPanel.Ready(ctx, id); err != nil {
		return err
	}
	return p.revoke(ctx)
}

func TestRevokedMembershipStopsAnAlreadyAuthorizedFileMutation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.s.Panel = &revokeBeforeMutationPanel{memoryPanel: f.panel, revoke: func(ctx context.Context) error {
		_, err := f.db.Exec(ctx, "DELETE FROM server_members WHERE server_id=$1 AND user_id=$2", f.id, f.admin)
		return err
	}}
	key := uuid.New()
	_, err := f.s.ChangeFile(ctx, f.admin, f.id, key, FileMutation{Action: "create", Path: "/revoked.txt", Content: "should not be written"}, "revocation-test")
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("revoked member was not rejected: %v", err)
	}
	if _, exists := f.panel.files["/revoked.txt"]; exists {
		t.Fatal("revoked member wrote a file")
	}
	var status string
	if err := f.db.QueryRow(ctx, "SELECT status FROM management_operations WHERE server_id=$1 AND user_id=$2 AND idempotency_key=$3", f.id, f.admin, key).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "REJECTED" {
		t.Fatalf("unissued mutation must not be ambiguous: %s", status)
	}
}
func TestSettingsPreservePropertiesAndProtectRevisions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in, err := f.s.Settings(ctx, f.admin, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if in.MOTD != "Hello!" {
		t.Fatal(in)
	}
	in.MOTD = "Our world 🌍"
	in.Difficulty = "hard"
	in.ViewDistance = 12
	in.Gamemode = "survival"
	in.MaxPlayers = 30
	if err = f.panel.Power(ctx, f.identifier, pterodactyl.PowerStart); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveSettings(ctx, f.owner, f.id, uuid.New(), in, "test"); !errors.Is(err, ErrOffline) {
		t.Fatal("running settings write", err)
	}
	f.panel.Power(ctx, f.identifier, pterodactyl.PowerStop)
	key := uuid.New()
	if _, err = f.s.SaveSettings(ctx, f.admin, f.id, key, in, "test"); err != nil {
		t.Fatal(err)
	}
	data := string(f.panel.files["/server.properties"])
	if !strings.Contains(data, "# Keep me\nlevel-name=world\nserver-port=25565\n") || strings.Count(data, "motd=") != 1 || !strings.Contains(data, "online-mode=true") {
		t.Fatal("unmanaged properties lost", data)
	}
	if _, err = f.s.SaveSettings(ctx, f.admin, f.id, key, in, "test"); err != nil {
		t.Fatal("replay failed", err)
	}
	if _, err = f.s.SaveSettings(ctx, f.admin, f.id, uuid.New(), in, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("stale overwrite allowed", err)
	}
	now, err := f.s.Settings(ctx, f.owner, f.id)
	if err != nil || now.MOTD != in.MOTD || now.MaxPlayers != 30 {
		t.Fatal(now, err)
	}
	var ram, disk, cpu int
	if err = f.db.QueryRow(ctx, "SELECT ram_mb,disk_mb,cpu_limit FROM servers WHERE id=$1", f.id).Scan(&ram, &disk, &cpu); err != nil {
		t.Fatal(err)
	}
	if ram != 2048 || disk != 10240 || cpu != 100 {
		t.Fatal("plan resources changed")
	}
	var logs string
	if err = f.db.QueryRow(ctx, "SELECT COALESCE(string_agg(metadata::text,' '),'') FROM audit_logs WHERE server_id=$1", f.id).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs, "Our world") || strings.Contains(logs, "server.properties") {
		t.Fatal("settings contents in audit")
	}
}
func TestFilesAndPaperPluginLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	key := uuid.New()
	in := FileMutation{Action: "create", Path: "/notes.txt", Content: "first"}
	if _, err := f.s.ChangeFile(ctx, f.owner, f.id, key, in, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ChangeFile(ctx, f.owner, f.id, key, in, "test"); err != nil {
		t.Fatal(err)
	}
	text, err := f.s.Text(ctx, f.owner, f.id, "/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeFile(ctx, f.owner, f.id, uuid.New(), FileMutation{Action: "write", Path: text.Path, Revision: text.Revision, Content: "second"}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeFile(ctx, f.owner, f.id, uuid.New(), FileMutation{Action: "rename", Path: "/notes.txt", Destination: "/renamed.txt"}, "test"); err != nil {
		t.Fatal(err)
	}
	stream, _, err := f.s.Download(ctx, f.owner, f.id, "/renamed.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(stream)
	stream.Close()
	if string(data) != "second" {
		t.Fatal(string(data))
	}
	if _, err = f.s.Upload(ctx, f.owner, f.id, uuid.New(), "/plugins", "Test.jar", pluginJar(t, "plugin.yml"), true, BackupDecision{WithoutBackup: true}, "test"); err != nil {
		t.Fatal(err)
	}
	plugins, err := f.s.Plugins(ctx, f.owner, f.id)
	if err != nil || len(plugins) != 1 {
		t.Fatal(plugins, err)
	}
	if _, err = f.s.DeletePlugin(ctx, f.owner, f.id, uuid.New(), "Test.jar", "Test.jar", "test"); err != nil {
		t.Fatal(err)
	}
	f.db.Exec(ctx, "UPDATE servers SET server_type='VANILLA' WHERE id=$1", f.id)
	if _, err = f.s.Upload(ctx, f.owner, f.id, uuid.New(), "/plugins", "Test.jar", pluginJar(t, "plugin.yml"), true, BackupDecision{WithoutBackup: true}, "test"); !errors.Is(err, ErrPlugin) {
		t.Fatal("Vanilla plugin accepted", err)
	}
}
func TestBackupLimitsOwnershipAndReplay(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	key := uuid.New()
	result, err := f.s.NewBackup(ctx, f.owner, f.id, key, "Before changes", "test")
	if err != nil {
		t.Fatal(err)
	}
	backup := result.(pterodactyl.Backup)
	if _, err = f.s.NewBackup(ctx, f.owner, f.id, key, "Before changes", "test"); err != nil {
		t.Fatal(err)
	}
	if f.panel.created != 1 {
		t.Fatal("duplicate backup")
	}
	if _, err = f.s.NewBackup(ctx, f.admin, f.id, uuid.New(), "Second", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.NewBackup(ctx, f.owner, f.id, uuid.New(), "Over limit", "test"); !errors.Is(err, ErrLimit) {
		t.Fatal("backup limit bypassed", err)
	}
	foreign := uuid.NewString()
	if _, err = f.s.ChangeBackup(ctx, f.owner, f.id, uuid.New(), foreign, "restore", foreign, "test"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("foreign backup accepted", err)
	}
	f.panel.Power(ctx, f.identifier, pterodactyl.PowerStart)
	if _, err = f.s.ChangeBackup(ctx, f.owner, f.id, uuid.New(), backup.UUID, "restore", backup.UUID, "test"); !errors.Is(err, ErrOffline) {
		t.Fatal("online restore accepted", err)
	}
	f.panel.Power(ctx, f.identifier, pterodactyl.PowerStop)
	restoreKey := uuid.New()
	if _, err = f.s.ChangeBackup(ctx, f.owner, f.id, restoreKey, backup.UUID, "restore", backup.UUID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ChangeBackup(ctx, f.owner, f.id, restoreKey, backup.UUID, "restore", backup.UUID, "test"); err != nil {
		t.Fatal(err)
	}
	if f.panel.restored != 1 {
		t.Fatal("duplicate restore")
	}
	if _, err = f.s.ChangeBackup(ctx, f.owner, f.id, uuid.New(), backup.UUID, "delete", backup.UUID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.NewBackup(ctx, f.owner, f.id, uuid.New(), "Free slot", "test"); err != nil {
		t.Fatal(err)
	}
}
func TestUnknownBackupCreationBlocksRepetitionAndCapacityConsumption(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.panel.uncertain = true
	key := uuid.New()
	if _, err := f.s.NewBackup(ctx, f.owner, f.id, key, "Unknown", "test"); !errors.Is(err, control.ErrUnknown) {
		t.Fatal(err)
	}
	if _, err := f.s.NewBackup(ctx, f.owner, f.id, key, "Unknown", "test"); !errors.Is(err, control.ErrUnknown) {
		t.Fatal(err)
	}
	if _, err := f.s.NewBackup(ctx, f.owner, f.id, uuid.New(), "Retry", "test"); !errors.Is(err, control.ErrBusy) {
		t.Fatal("ambiguous backup capacity reused", err)
	}
	if f.panel.created != 1 {
		t.Fatal("unknown result repeated")
	}
}
func TestConcurrentBackupAllowance(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var failures = make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.NewBackup(ctx, f.owner, f.id, uuid.New(), "Concurrent", "test")
			if err != nil && !errors.Is(err, control.ErrBusy) && !errors.Is(err, ErrLimit) {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if f.panel.created > 2 {
		t.Fatal("oversold backup slots", f.panel.created)
	}
}
func TestPanelRestoreStateBlocksControls(t *testing.T) {
	f := setup(t)
	f.panel.busy = true
	ctx := context.Background()
	if err := f.s.Control.Power(ctx, f.owner, f.id, uuid.New(), pterodactyl.PowerStart, "", "test"); !errors.Is(err, control.ErrState) {
		t.Fatal("power during restore", err)
	}
	if _, err := f.s.Files(ctx, f.owner, f.id, "/"); !errors.Is(err, control.ErrState) {
		t.Fatal("files during restore", err)
	}
}
