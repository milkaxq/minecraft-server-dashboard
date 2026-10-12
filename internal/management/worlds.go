package management

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

const maxWorldEntries = 2000
const maxWorldExpanded = 128 << 20

type World struct {
	Name         string   `json:"name"`
	Directories  []string `json:"directories"`
	Generated    bool     `json:"generated"`
	Revision     string   `json:"revision"`
	SafetyBackup string   `json:"safety_backup,omitempty"`
}
type worldFile struct {
	Path  string
	Entry pterodactyl.FileEntry
}

// level-name is resolved from the container's properties, never from an upload.
// Only a top-level world is supported; custom nested paths fail closed.
func (s *Service) world(ctx context.Context, t Target) (World, []worldFile, error) {
	text, err := s.read(ctx, t, "/server.properties")
	if err != nil {
		return World{}, nil, err
	}
	lines, err := parseProperties(text.Content)
	if err != nil {
		return World{}, nil, err
	}
	name := "world"
	for _, line := range lines {
		if line.key == "level-name" {
			name = line.value
		}
	}
	if Name(name) != nil || strings.HasPrefix(name, ".") || name == "plugins" {
		return World{}, nil, server.ErrInvalid
	}
	w := World{Name: name, Directories: []string{}}
	candidates := []string{name}
	if server.SupportsPlugins(t.Type) {
		candidates = append(candidates, name+"_nether", name+"_the_end")
	}
	files := []worldFile{}
	var total int64
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > 32 {
			return server.ErrInvalid
		}
		entries, e := s.Panel.ListFiles(ctx, t.Identifier, dir)
		if e != nil {
			return control.ErrUnavailable
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		for _, v := range entries {
			if Name(v.Name) != nil || v.IsSymlink || v.Size < 0 {
				return server.ErrInvalid
			}
			target := path.Join(dir, v.Name)
			if _, e = Path(target, false); e != nil {
				return e
			}
			files = append(files, worldFile{target, v})
			if len(files) > maxWorldEntries {
				return server.ErrInvalid
			}
			if v.IsFile {
				if v.Size > int64(t.DiskMB)*1048576-total {
					return server.ErrInvalid
				}
				total += v.Size
			} else if e = walk(target, depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	for _, dir := range candidates {
		v, e := s.inspect(ctx, t, "/"+dir, true)
		if e != nil {
			return World{}, nil, e
		}
		if v == nil {
			continue
		}
		if v.IsFile {
			return World{}, nil, server.ErrInvalid
		}
		w.Directories = append(w.Directories, dir)
		files = append(files, worldFile{"/" + dir, *v})
		if e = walk("/"+dir, 0); e != nil {
			return World{}, nil, e
		}
	}
	for _, f := range files {
		if f.Path == "/"+name+"/level.dat" && f.Entry.IsFile {
			w.Generated = true
		}
	}
	data, _ := json.Marshal(struct {
		Properties string
		Files      []worldFile
	}{text.Revision, files})
	w.Revision = revision(data)
	return w, files, nil
}
func (s *Service) World(ctx context.Context, user, id uuid.UUID) (World, error) {
	t, err := s.access(ctx, user, id, server.Worlds)
	if err != nil {
		return World{}, err
	}
	w, _, err := s.world(ctx, t)
	if err != nil {
		return w, err
	}
	// Retain an in-progress safety backup across page reloads; completion is
	// checked separately against Panel immediately before any destructive action.
	if err = t.q.QueryRow(ctx, `SELECT COALESCE((SELECT m.result->>'uuid' FROM management_operations m WHERE m.server_id=$1 AND m.action='world.backup' AND m.status='SUCCEEDED' AND m.result->>'revision'=$2 AND NOT EXISTS(SELECT 1 FROM control_requests c WHERE c.server_id=m.server_id AND c.created_at>m.created_at AND c.action IN ('start','restart')) AND NOT EXISTS(SELECT 1 FROM management_operations n WHERE n.server_id=m.server_id AND n.created_at>m.created_at AND n.action NOT IN ('world.backup','backup.create','backup.delete','world.reset','world.upload') AND n.status IN ('PENDING','SUCCEEDED','UNKNOWN')) ORDER BY m.created_at DESC LIMIT 1),'')`, id, w.Revision).Scan(&w.SafetyBackup); err != nil {
		return w, err
	}
	return w, nil
}

// A safety backup records the stopped world's snapshot. Any subsequent app
// start, restore, or file mutation invalidates it, even if filenames are unchanged.
func (s *Service) WorldBackup(ctx context.Context, user, id, key uuid.UUID, rid string) (any, error) {
	return s.mutate(ctx, user, id, key, server.Worlds, "world.backup", struct{}{}, rid, func(t Target) (any, error) {
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		w, _, err := s.world(ctx, t)
		if err != nil {
			return nil, err
		}
		var unresolved bool
		if err = t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_operations WHERE server_id=$1 AND action IN ('backup.create','world.backup','world.reset','world.upload') AND status IN ('PENDING','UNKNOWN') AND NOT (user_id=$2 AND idempotency_key=$3))`, id, user, key).Scan(&unresolved); err != nil {
			return nil, err
		}
		if unresolved {
			return nil, control.ErrBusy
		}
		if err := scheduledBackupGuard(ctx, t); err != nil {
			return nil, err
		}
		list, err := s.Panel.ListBackups(ctx, t.Identifier)
		if err != nil {
			return nil, err
		}
		if len(list) >= t.BackupLimit {
			return nil, ErrLimit
		}
		b, err := s.Panel.CreateBackup(ctx, t.Identifier, "Before world change")
		if err != nil {
			return nil, err
		}
		if _, err = uuid.Parse(b.UUID); err != nil {
			return nil, control.ErrUnknown
		}
		return map[string]any{"uuid": b.UUID, "revision": w.Revision}, nil
	})
}

type WorldChange struct {
	Backup       string `json:"backup"`
	Confirmation string `json:"confirmation"`
	Revision     string `json:"revision"`
}

func (s *Service) checkWorldChange(ctx context.Context, t Target, in WorldChange) (World, error) {
	if err := s.offline(ctx, t); err != nil {
		return World{}, err
	}
	w, _, err := s.world(ctx, t)
	if err != nil {
		return w, err
	}
	if in.Confirmation != w.Name {
		return w, server.ErrInvalid
	}
	if in.Revision != w.Revision {
		return w, ErrChanged
	}
	var snapshot string
	var created time.Time
	err = t.q.QueryRow(ctx, `SELECT result->>'revision',created_at FROM management_operations WHERE server_id=$1 AND action='world.backup' AND status='SUCCEEDED' AND result->>'uuid'=$2 ORDER BY created_at DESC LIMIT 1`, t.ID, in.Backup).Scan(&snapshot, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrChanged
	}
	if err != nil {
		return w, err
	}
	if snapshot != w.Revision {
		return w, ErrChanged
	}
	var changed bool
	if err = t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM control_requests WHERE server_id=$1 AND created_at>$2 AND action IN ('start','restart')) OR EXISTS(SELECT 1 FROM management_operations WHERE server_id=$1 AND created_at>$2 AND action NOT IN ('world.backup','backup.create','backup.delete','world.reset','world.upload') AND status IN ('PENDING','SUCCEEDED','UNKNOWN')) OR EXISTS(SELECT 1 FROM management_operations WHERE server_id=$1 AND action IN ('world.reset','world.upload') AND status='UNKNOWN')`, t.ID, created).Scan(&changed); err != nil {
		return w, err
	}
	if changed {
		return w, ErrChanged
	}
	list, err := s.Panel.ListBackups(ctx, t.Identifier)
	if err != nil {
		return w, err
	}
	b, err := backupIn(list, in.Backup)
	if err != nil {
		return w, err
	}
	if !b.Successful || b.CompletedAt == nil {
		return w, control.ErrBusy
	}
	return w, nil
}

// ZIP contents are validated and uploaded individually through Wings. Neither
// an archive extractor nor any server file is opened on the application's host.
func validateWorldZIP(data []byte, name, kind string) (*zip.Reader, int64, error) {
	if len(data) == 0 || len(data) > pterodactyl.MaxUploadBytes {
		return nil, 0, server.ErrInvalid
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) > maxWorldEntries {
		return nil, 0, server.ErrInvalid
	}
	root := false
	wrapped := false
	for _, f := range z.File {
		if f.Name == "level.dat" && f.Mode().IsRegular() && f.UncompressedSize64 > 0 {
			root = true
		}
		if f.Name == name+"/level.dat" && f.Mode().IsRegular() && f.UncompressedSize64 > 0 {
			wrapped = true
		}
	}
	if root == wrapped {
		return nil, 0, server.ErrInvalid
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range z.File {
		n := strings.TrimSuffix(f.Name, "/")
		if len(strings.Split(n, "/")) > 32 {
			return nil, 0, server.ErrInvalid
		}
		for _, part := range strings.Split(n, "/") {
			if Name(part) != nil {
				return nil, 0, server.ErrInvalid
			}
		}
		if _, err = Path("/"+n, false); err != nil || len(n) > 900 || f.Mode().Type() != 0 && f.Mode().Type() != fs.ModeDir || f.Flags&1 != 0 {
			return nil, 0, server.ErrInvalid
		}
		if seen[n] {
			return nil, 0, server.ErrInvalid
		}
		seen[n] = true
		if !root {
			top := strings.Split(n, "/")[0]
			if top != name && !(server.SupportsPlugins(kind) && (top == name+"_nether" || top == name+"_the_end")) {
				return nil, 0, server.ErrInvalid
			}
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > 32<<20 {
			return nil, 0, server.ErrInvalid
		}
		total += int64(f.UncompressedSize64)
		if total > maxWorldExpanded {
			return nil, 0, server.ErrInvalid
		}
		r, e := f.Open()
		if e != nil {
			return nil, 0, server.ErrInvalid
		}
		copied, e := io.Copy(io.Discard, io.LimitReader(r, int64(f.UncompressedSize64)+1))
		r.Close()
		if e != nil || copied != int64(f.UncompressedSize64) {
			return nil, 0, server.ErrInvalid
		}
	}
	// A regular file cannot also be an ancestor directory.
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		for _, other := range z.File {
			if strings.HasPrefix(other.Name, f.Name+"/") {
				return nil, 0, server.ErrInvalid
			}
		}
	}
	return z, total, nil
}
func (s *Service) ChangeWorld(ctx context.Context, user, id, key uuid.UUID, in WorldChange, data []byte, upload bool, rid string) (any, error) {
	if _, err := uuid.Parse(in.Backup); err != nil || len(in.Revision) != 64 || Name(in.Confirmation) != nil {
		return nil, server.ErrInvalid
	}
	action := "world.reset"
	if upload {
		action = "world.upload"
	}
	input := struct {
		WorldChange
		Digest string
	}{in, revision(data)}
	return s.mutate(ctx, user, id, key, server.Worlds, action, input, rid, func(t Target) (any, error) {
		w, err := s.checkWorldChange(ctx, t, in)
		if err != nil {
			return nil, err
		}
		var z *zip.Reader
		var expanded int64
		if upload {
			z, expanded, err = validateWorldZIP(data, w.Name, t.Type)
			if err != nil {
				return nil, err
			}
			usage, e := s.Control.Panel.Resources(ctx, t.Identifier)
			if e != nil {
				return nil, control.ErrUnavailable
			}
			if usage.DiskBytes+expanded > int64(t.DiskMB)*1048576 {
				return nil, server.ErrInvalid
			}
		}
		stage := ".hosting-world-" + uuid.NewString()
		if err = s.Panel.CreateFolder(ctx, t.Identifier, "/", stage); err != nil {
			return nil, err
		}
		// After the first write, every failure is ambiguous, including a definitive
		// HTTP rejection. Retain staging/old files for operator recovery; never replay.
		fail := func() (any, error) { return nil, control.ErrUnknown }
		if err = s.Panel.CreateFolder(ctx, t.Identifier, "/"+stage, "old"); err != nil {
			return fail()
		}
		newDirs := map[string]bool{}
		if upload {
			if err = s.Panel.CreateFolder(ctx, t.Identifier, "/"+stage, "new"); err != nil {
				return fail()
			}
			made := map[string]bool{"/" + stage + "/new": true}
			var mkdir func(string) error
			mkdir = func(dir string) error {
				if made[dir] {
					return nil
				}
				if e := mkdir(path.Dir(dir)); e != nil {
					return e
				}
				if e := s.Panel.CreateFolder(ctx, t.Identifier, path.Dir(dir), path.Base(dir)); e != nil {
					return e
				}
				made[dir] = true
				return nil
			}
			root := false
			for _, f := range z.File {
				if f.Name == "level.dat" && f.Mode().IsRegular() && f.UncompressedSize64 > 0 {
					root = true
				}
			}
			for _, f := range z.File {
				n := strings.TrimSuffix(f.Name, "/")
				if root {
					n = w.Name + "/" + n
				}
				newDirs[strings.Split(n, "/")[0]] = true
				dest := "/" + stage + "/new/" + n
				if f.FileInfo().IsDir() {
					if mkdir(dest) != nil {
						return fail()
					}
					continue
				}
				if mkdir(path.Dir(dest)) != nil {
					return fail()
				}
				r, e := f.Open()
				if e != nil {
					return fail()
				}
				content, e := io.ReadAll(io.LimitReader(r, 32<<20+1))
				r.Close()
				if e != nil {
					return fail()
				}
				if s.Panel.UploadFile(ctx, t.Identifier, path.Dir(dest), path.Base(dest), content) != nil {
					return fail()
				}
			}
		}
		// Keep the original world until every replacement file has arrived.
		for _, dir := range w.Directories {
			if s.Panel.RenameFile(ctx, t.Identifier, "/", dir, stage+"/old/"+dir) != nil {
				return fail()
			}
		}
		dirs := []string{}
		for dir := range newDirs {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)
		for _, dir := range dirs {
			if s.Panel.RenameFile(ctx, t.Identifier, "/", stage+"/new/"+dir, dir) != nil {
				return fail()
			}
		}
		if s.Panel.DeleteFile(ctx, t.Identifier, "/", stage) != nil {
			return fail()
		}
		return map[string]any{"status": "saved", "restart_required": true, "backup": in.Backup}, nil
	})
}

// Keep the server's shared lock until streaming finishes, preventing an app
// power or file mutation from racing a consistent stopped-world download.
func (s *Service) WorldDownload(ctx context.Context, user, id uuid.UUID) (io.ReadCloser, string, error) {
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return nil, "", err
	}
	var held bool
	release := func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if held {
			var unlocked bool
			if e := conn.QueryRow(clean, "SELECT pg_advisory_unlock(hashtextextended($1,0))", "server:"+id.String()).Scan(&unlocked); e != nil || !unlocked {
				_ = conn.Conn().Close(clean)
			}
		}
		conn.Release()
	}
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", "server:"+id.String()).Scan(&held); err != nil {
		release()
		return nil, "", err
	}
	if !held {
		release()
		return nil, "", control.ErrBusy
	}
	t, err := s.accessWith(ctx, conn, user, id, server.Worlds)
	if err != nil {
		release()
		return nil, "", err
	}
	if err = s.offline(ctx, t); err != nil {
		release()
		return nil, "", err
	}
	w, files, err := s.world(ctx, t)
	if err != nil {
		release()
		return nil, "", err
	}
	if !w.Generated {
		release()
		return nil, "", server.ErrNotFound
	}
	reader, writer := io.Pipe()
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	go func() {
		defer release()
		defer cancel()
		z := zip.NewWriter(writer)
		streamErr := func() error {
			for _, f := range files {
				if !f.Entry.IsFile {
					continue
				}
				dest, e := z.Create(strings.TrimPrefix(f.Path, "/"))
				if e != nil {
					return e
				}
				r, e := s.Panel.DownloadFile(streamCtx, t.Identifier, f.Path)
				if e != nil {
					return e
				}
				n, e := io.Copy(dest, io.LimitReader(r, f.Entry.Size+1))
				r.Close()
				if e != nil {
					return e
				}
				if n != f.Entry.Size {
					return errors.New("world changed during download")
				}
			}
			return z.Close()
		}()
		_ = writer.CloseWithError(streamErr)
	}()
	return &worldStream{reader: reader, cancel: cancel}, fmt.Sprintf("%s.zip", w.Name), nil
}

type worldStream struct {
	reader *io.PipeReader
	cancel context.CancelFunc
}

func (s *worldStream) Read(data []byte) (int, error) { return s.reader.Read(data) }
func (s *worldStream) Close() error                  { s.cancel(); return s.reader.Close() }
