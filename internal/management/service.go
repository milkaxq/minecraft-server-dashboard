// Package management owns container files, settings, plugins and backups.
package management

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

var ErrOffline = errors.New("stop the server before changing files or restoring")
var ErrLimit = errors.New("backup allowance reached")
var ErrChanged = errors.New("file changed; reload before saving")
var ErrPlugin = errors.New("this server requires a compatible plugin jar")

type Service struct {
	Catalog *catalog.Client
	DB      *pgxpool.Pool
	Control *control.Service
	Panel   pterodactyl.ManagementClient
}
type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Target struct {
	q querier
	control.Target
	Type        string
	BackupLimit int
	DiskMB      int
	Version     string
}

func (s *Service) access(ctx context.Context, user, id uuid.UUID, p server.Permission) (Target, error) {
	return s.accessWith(ctx, s.DB, user, id, p)
}
func (s *Service) accessWith(ctx context.Context, q querier, user, id uuid.UUID, p server.Permission) (Target, error) {
	t, err := s.Control.Access(ctx, q, user, id, p)
	if err != nil {
		return Target{}, err
	}
	if s.Panel == nil {
		return Target{}, control.ErrNotConfigured
	}
	v := Target{Target: t, q: q}
	if err = q.QueryRow(ctx, "SELECT server_type,backup_limit,disk_mb,minecraft_version FROM servers WHERE id=$1", id).Scan(&v.Type, &v.BackupLimit, &v.DiskMB, &v.Version); err != nil {
		return v, err
	}
	if err = s.Panel.Ready(ctx, t.Identifier); err != nil {
		if errors.Is(err, pterodactyl.ErrConflict) {
			return v, control.ErrState
		}
		return v, control.ErrUnavailable
	}
	return v, nil
}
func (s *Service) offline(ctx context.Context, t Target) error {
	u, err := s.Control.Panel.Resources(ctx, t.Identifier)
	if err != nil {
		return control.ErrUnavailable
	}
	if u.State != "offline" {
		return ErrOffline
	}
	return nil
}

// No-follow checks every path component using Wings' symlink metadata. Wings
// remains responsible for its filesystem sandbox and races with in-container code.
func (s *Service) inspect(ctx context.Context, t Target, value string, allowMissing bool) (*pterodactyl.FileEntry, error) {
	if value == "/" {
		return nil, nil
	}
	parts := strings.Split(value[1:], "/")
	root := "/"
	for i, name := range parts {
		entries, err := s.Panel.ListFiles(ctx, t.Identifier, root)
		if err != nil {
			return nil, control.ErrUnavailable
		}
		var found *pterodactyl.FileEntry
		for _, v := range entries {
			if v.Name == name {
				copy := v
				found = &copy
				break
			}
		}
		if found == nil {
			if allowMissing && i == len(parts)-1 {
				return nil, nil
			}
			return nil, server.ErrNotFound
		}
		if found.IsSymlink || (i < len(parts)-1 && found.IsFile) {
			return nil, server.ErrInvalid
		}
		if i == len(parts)-1 {
			return found, nil
		}
		root = path.Join(root, name)
	}
	return nil, server.ErrInvalid
}
func (s *Service) Files(ctx context.Context, user, id uuid.UUID, dir string) ([]pterodactyl.FileEntry, error) {
	if _, err := Path(dir, true); err != nil {
		return nil, err
	}
	t, err := s.access(ctx, user, id, server.Files)
	if err != nil {
		return nil, err
	}
	v, err := s.inspect(ctx, t, dir, false)
	if err != nil {
		return nil, err
	}
	if v != nil && v.IsFile {
		return nil, server.ErrInvalid
	}
	entries, err := s.Panel.ListFiles(ctx, t.Identifier, dir)
	if err != nil {
		return nil, control.ErrUnavailable
	}
	return entries, nil
}

type TextFile struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

func revision(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func (s *Service) read(ctx context.Context, t Target, file string) (TextFile, error) {
	v, err := s.inspect(ctx, t, file, false)
	if err != nil {
		return TextFile{}, err
	}
	if v == nil || !v.IsFile || v.Size > pterodactyl.MaxTextBytes {
		return TextFile{}, server.ErrInvalid
	}
	data, err := s.Panel.ReadFile(ctx, t.Identifier, file)
	if err != nil {
		return TextFile{}, control.ErrUnavailable
	}
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return TextFile{}, server.ErrInvalid
	}
	return TextFile{file, string(data), revision(data)}, nil
}
func (s *Service) Text(ctx context.Context, user, id uuid.UUID, file string) (TextFile, error) {
	if _, err := Path(file, false); err != nil {
		return TextFile{}, err
	}
	t, err := s.access(ctx, user, id, server.Files)
	if err != nil {
		return TextFile{}, err
	}
	return s.read(ctx, t, file)
}

// mutate commits an intent before contacting Panel. An ambiguous write is never
// automatically repeated, and all mutations share the provisioning/power lock.
func (s *Service) mutate(ctx context.Context, user, id, key uuid.UUID, p server.Permission, action string, input any, requestID string, run func(Target) (any, error)) (any, error) {
	if key == uuid.Nil {
		return nil, server.ErrInvalid
	}
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	lock := "server:" + id.String()
	var held bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", lock).Scan(&held); err != nil {
		return nil, err
	}
	if !held {
		return nil, control.ErrBusy
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var ok bool
		if err := conn.QueryRow(clean, "SELECT pg_advisory_unlock(hashtextextended($1,0))", lock).Scan(&ok); err != nil || !ok {
			_ = conn.Conn().Close(clean)
		}
	}()
	t, err := s.accessWith(ctx, conn, user, id, p)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, server.ErrInvalid
	}
	fingerprint := revision(append([]byte(action+"\x00"), payload...))
	var previous, status string
	var data []byte
	err = conn.QueryRow(ctx, "SELECT fingerprint,status,result FROM management_operations WHERE server_id=$1 AND user_id=$2 AND idempotency_key=$3", id, user, key).Scan(&previous, &status, &data)
	if err == nil {
		if previous != fingerprint {
			return nil, control.ErrBusy
		}
		if status != "SUCCEEDED" {
			return nil, control.ErrUnknown
		}
		var result any
		_ = json.Unmarshal(data, &result)
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	operation := uuid.New()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "INSERT INTO management_operations(id,server_id,user_id,idempotency_key,action,fingerprint,status,request_id) VALUES($1,$2,$3,$4,$5,$6,'PENDING',$7)", operation, id, user, key, action, fingerprint, requestID); err != nil {
		return nil, err
	}
	if err = audit.Record(ctx, tx, user, id, "management.requested", requestID, map[string]any{"operation_id": operation, "action": action}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	// Membership, entitlement, or lifecycle intent may change while the durable
	// operation is recorded. Check again before issuing a remote mutation, as the
	// power/command service does. Revoked access must not use the earlier snapshot.
	var result any
	t.Target, err = s.Control.Access(ctx, conn, user, id, p)
	runErr := err
	if runErr == nil {
		result, runErr = run(t)
	}
	outcome := "SUCCEEDED"
	if runErr != nil {
		outcome = "UNKNOWN"
		if errors.Is(runErr, server.ErrInvalid) || errors.Is(runErr, server.ErrNotFound) || errors.Is(runErr, ErrOffline) || errors.Is(runErr, ErrLimit) || errors.Is(runErr, ErrChanged) || errors.Is(runErr, ErrPlugin) || errors.Is(runErr, ErrAddonConfig) || errors.Is(runErr, ErrAddonPort) || errors.Is(runErr, ErrMod) || errors.Is(runErr, catalog.ErrUnavailable) || errors.Is(runErr, control.ErrBusy) || errors.Is(runErr, control.ErrState) || pterodactyl.Rejected(runErr) {
			outcome = "REJECTED"
		}
	}
	// Result contains metadata only; never store file contents or signed URLs.
	data, err = json.Marshal(result)
	if err != nil {
		return nil, control.ErrUnknown
	}
	clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finish, err := conn.Begin(clean)
	if err != nil {
		return nil, control.ErrUnknown
	}
	defer finish.Rollback(context.Background())
	if _, err = finish.Exec(clean, "UPDATE management_operations SET status=$2,result=CASE WHEN action='catalog.install' AND $3::jsonb='null'::jsonb THEN result ELSE $3::jsonb END,updated_at=now() WHERE id=$1", operation, outcome, data); err != nil {
		return nil, control.ErrUnknown
	}
	if err = audit.Record(clean, finish, user, id, "management.completed", requestID, map[string]any{"operation_id": operation, "action": action, "outcome": outcome}); err != nil {
		return nil, control.ErrUnknown
	}
	if err = finish.Commit(clean); err != nil {
		return nil, control.ErrUnknown
	}
	if runErr != nil {
		if outcome == "REJECTED" {
			if pterodactyl.Rejected(runErr) {
				return nil, control.ErrUnavailable
			}
			return nil, runErr
		}
		return nil, control.ErrUnknown
	}
	return result, nil
}

type FileMutation struct {
	Action       string `json:"action"`
	Path         string `json:"path"`
	Destination  string `json:"destination,omitempty"`
	Content      string `json:"content,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
}

func (s *Service) ChangeFile(ctx context.Context, user, id, key uuid.UUID, in FileMutation, rid string) (any, error) {
	if _, err := Path(in.Path, false); err != nil {
		return nil, err
	}
	if len(in.Content) > pterodactyl.MaxTextBytes || !utf8.ValidString(in.Content) || strings.ContainsRune(in.Content, 0) {
		return nil, server.ErrInvalid
	}
	switch in.Action {
	case "create", "write", "folder":
	case "delete":
		if in.Confirmation != in.Path {
			return nil, server.ErrInvalid
		}
	case "rename":
		if _, err := Path(in.Destination, false); err != nil || path.Dir(in.Destination) != path.Dir(in.Path) {
			return nil, server.ErrInvalid
		}
	default:
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.Files, "file."+in.Action, in, rid, func(t Target) (any, error) {
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		v, err := s.inspect(ctx, t, in.Path, in.Action == "create" || in.Action == "folder")
		if err != nil {
			return nil, err
		}
		switch in.Action {
		case "create", "folder":
			if v != nil {
				return nil, ErrChanged
			}
			if in.Action == "folder" {
				err = s.Panel.CreateFolder(ctx, t.Identifier, path.Dir(in.Path), path.Base(in.Path))
			} else {
				err = s.Panel.WriteFile(ctx, t.Identifier, in.Path, []byte(in.Content))
			}
		case "write":
			if v == nil || !v.IsFile {
				return nil, server.ErrInvalid
			}
			original, e := s.read(ctx, t, in.Path)
			if e != nil {
				return nil, e
			}
			if in.Revision == "" || in.Revision != original.Revision {
				return nil, ErrChanged
			}
			err = s.Panel.WriteFile(ctx, t.Identifier, in.Path, []byte(in.Content))
		case "rename":
			existing, e := s.inspect(ctx, t, in.Destination, true)
			if e != nil {
				return nil, e
			}
			if existing != nil {
				return nil, ErrChanged
			}
			err = s.Panel.RenameFile(ctx, t.Identifier, path.Dir(in.Path), path.Base(in.Path), path.Base(in.Destination))
		case "delete":
			err = s.Panel.DeleteFile(ctx, t.Identifier, path.Dir(in.Path), path.Base(in.Path))
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": "saved", "restart_required": true}, nil
	})
}
func (s *Service) Upload(ctx context.Context, user, id, key uuid.UUID, dir, name string, data []byte, plugin bool, rid string) (any, error) {
	if _, err := Path(dir, true); err != nil {
		return nil, err
	}
	if err := Name(name); err != nil {
		return nil, err
	}
	if len(data) > pterodactyl.MaxUploadBytes {
		return nil, server.ErrInvalid
	}
	permission := server.Files
	action := "file.upload"
	if plugin {
		permission = server.Plugins
		action = "plugin.upload"
		if dir != "/plugins" || validatePlugin(name, data) != nil {
			return nil, ErrPlugin
		}
	}
	input := map[string]any{"directory": dir, "name": name, "digest": revision(data)}
	return s.mutate(ctx, user, id, key, permission, action, input, rid, func(t Target) (any, error) {
		if plugin && (!server.SupportsPlugins(t.Type) || validatePluginForSoftware(t.Type, name, data) != nil) {
			return nil, ErrPlugin
		}
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		if plugin {
			v, err := s.inspect(ctx, t, dir, true)
			if err != nil {
				return nil, err
			}
			if v == nil {
				if err = s.Panel.CreateFolder(ctx, t.Identifier, "/", "plugins"); err != nil {
					return nil, err
				}
			}
		}
		v, err := s.inspect(ctx, t, path.Join(dir, name), true)
		if err != nil {
			return nil, err
		}
		if v != nil {
			return nil, ErrChanged
		}
		if err = s.Panel.UploadFile(ctx, t.Identifier, dir, name, data); err != nil {
			return nil, err
		}
		return map[string]any{"status": "uploaded", "restart_required": true}, nil
	})
}
func validatePlugin(name string, data []byte) error {
	return validatePluginDescriptor(name, data, true)
}

func validatePluginDescriptor(name string, data []byte, allowPaper bool) error {
	if !strings.HasSuffix(strings.ToLower(name), ".jar") {
		return ErrPlugin
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) > 10000 {
		return ErrPlugin
	}
	for _, v := range z.File {
		if v.Name == "plugin.yml" || (allowPaper && v.Name == "paper-plugin.yml") {
			if v.UncompressedSize64 > 64<<10 {
				return ErrPlugin
			}
			r, err := v.Open()
			if err != nil {
				return ErrPlugin
			}
			content, e := io.ReadAll(io.LimitReader(r, (64<<10)+1))
			r.Close()
			if e != nil || len(content) == 0 || len(content) > 64<<10 || !utf8.Valid(content) {
				return ErrPlugin
			}
			return nil
		}
	}
	return ErrPlugin
}
func (s *Service) Plugins(ctx context.Context, user, id uuid.UUID) ([]pterodactyl.FileEntry, error) {
	t, err := s.access(ctx, user, id, server.Plugins)
	if err != nil {
		return nil, err
	}
	if !server.SupportsPlugins(t.Type) {
		return nil, ErrPlugin
	}
	v, err := s.inspect(ctx, t, "/plugins", true)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return []pterodactyl.FileEntry{}, nil
	}
	if v.IsFile {
		return nil, server.ErrInvalid
	}
	entries, err := s.Panel.ListFiles(ctx, t.Identifier, "/plugins")
	if err != nil {
		return nil, control.ErrUnavailable
	}
	out := []pterodactyl.FileEntry{}
	for _, v := range entries {
		if v.IsFile && !v.IsSymlink && strings.HasSuffix(strings.ToLower(v.Name), ".jar") {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *Service) DeletePlugin(ctx context.Context, user, id, key uuid.UUID, name, confirmation, rid string) (any, error) {
	if Name(name) != nil || !strings.HasSuffix(strings.ToLower(name), ".jar") || confirmation != name {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.Plugins, "plugin.delete", map[string]string{"name": name}, rid, func(t Target) (any, error) {
		if !server.SupportsPlugins(t.Type) {
			return nil, ErrPlugin
		}
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		if _, err := s.inspect(ctx, t, "/plugins/"+name, false); err != nil {
			return nil, err
		}
		if err := s.Panel.DeleteFile(ctx, t.Identifier, "/plugins", name); err != nil {
			return nil, err
		}
		return map[string]any{"status": "deleted", "restart_required": true}, nil
	})
}
func (s *Service) Download(ctx context.Context, user, id uuid.UUID, file string) (io.ReadCloser, int64, error) {
	if _, err := Path(file, false); err != nil {
		return nil, 0, err
	}
	t, err := s.access(ctx, user, id, server.Files)
	if err != nil {
		return nil, 0, err
	}
	v, err := s.inspect(ctx, t, file, false)
	if err != nil {
		return nil, 0, err
	}
	if v == nil || !v.IsFile {
		return nil, 0, server.ErrInvalid
	}
	r, err := s.Panel.DownloadFile(ctx, t.Identifier, file)
	if err != nil {
		return nil, 0, control.ErrUnavailable
	}
	return r, int64(t.DiskMB) * 1048576, nil
}
