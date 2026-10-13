package management

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

var ErrMod = errors.New("upload a server mod for this loader; client-only mods and other loaders are incompatible")

func validateMod(kind, name string, data []byte) error {
	if Name(name) != nil || !strings.HasSuffix(strings.ToLower(name), ".jar") || len(data) > pterodactyl.MaxUploadBytes {
		return ErrMod
	}
	descriptor := map[string]string{"FABRIC": "fabric.mod.json", "FORGE": "META-INF/mods.toml", "NEOFORGE": "META-INF/neoforge.mods.toml"}[kind]
	if descriptor == "" {
		return ErrMod
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) > 10000 {
		return ErrMod
	}
	for _, f := range z.File {
		if f.Name != descriptor {
			continue
		}
		if f.UncompressedSize64 > 64<<10 {
			return ErrMod
		}
		r, err := f.Open()
		if err != nil {
			return ErrMod
		}
		content, err := io.ReadAll(io.LimitReader(r, (64<<10)+1))
		r.Close()
		if err != nil || len(content) == 0 || len(content) > 64<<10 || !utf8.Valid(content) {
			return ErrMod
		}
		if kind == "FABRIC" {
			var meta struct {
				ID          string `json:"id"`
				Environment string `json:"environment"`
			}
			if json.Unmarshal(content, &meta) != nil || meta.ID == "" || (meta.Environment != "" && meta.Environment != "*" && meta.Environment != "server") {
				return ErrMod
			}
		}
		return nil
	}
	return ErrMod
}

func validatePluginForSoftware(kind, name string, data []byte) error {
	return validatePluginDescriptor(name, data, kind != "SPIGOT")
}

func (s *Service) UploadMod(ctx context.Context, user, id, key uuid.UUID, name string, data []byte, safety BackupDecision, rid string) (any, error) {
	if Name(name) != nil || len(data) > pterodactyl.MaxUploadBytes {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.Files, "mod.upload", map[string]any{"name": name, "digest": revision(data), "safety": safety}, rid, func(t Target) (any, error) {
		if err := validateMod(t.Type, name, data); err != nil {
			return nil, err
		}
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		if err := s.checkBackupDecision(ctx, t, safety); err != nil {
			return nil, err
		}
		folder, err := s.inspect(ctx, t, "/mods", true)
		if err != nil {
			return nil, err
		}
		if folder == nil {
			if err = s.Panel.CreateFolder(ctx, t.Identifier, "/", "mods"); err != nil {
				return nil, err
			}
		}
		existing, err := s.inspect(ctx, t, path.Join("/mods", name), true)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, ErrChanged
		}
		if err = s.Panel.UploadFile(ctx, t.Identifier, "/mods", name, data); err != nil {
			return nil, err
		}
		return map[string]any{"status": "uploaded", "restart_required": true}, nil
	})
}
func (s *Service) Mods(ctx context.Context, user, id uuid.UUID) ([]pterodactyl.FileEntry, error) {
	t, err := s.access(ctx, user, id, server.Files)
	if err != nil {
		return nil, err
	}
	if !server.SupportsMods(t.Type) {
		return nil, ErrMod
	}
	v, err := s.inspect(ctx, t, "/mods", true)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return []pterodactyl.FileEntry{}, nil
	}
	if v.IsFile {
		return nil, server.ErrInvalid
	}
	entries, err := s.Panel.ListFiles(ctx, t.Identifier, "/mods")
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
func (s *Service) DeleteMod(ctx context.Context, user, id, key uuid.UUID, name, confirmation, rid string) (any, error) {
	if Name(name) != nil || !strings.HasSuffix(strings.ToLower(name), ".jar") || confirmation != name {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.Files, "mod.delete", map[string]string{"name": name}, rid, func(t Target) (any, error) {
		if !server.SupportsMods(t.Type) {
			return nil, ErrMod
		}
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		if _, err := s.inspect(ctx, t, "/mods/"+name, false); err != nil {
			return nil, err
		}
		if err := s.Panel.DeleteFile(ctx, t.Identifier, "/mods", name); err != nil {
			return nil, err
		}
		return map[string]any{"status": "deleted", "restart_required": true}, nil
	})
}
