package management

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"strings"
	"time"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"

	"github.com/google/uuid"
)

type InstallPreview struct {
	Project          string            `json:"project"`
	Software         string            `json:"software"`
	MinecraftVersion string            `json:"minecraft_version"`
	Files            []catalog.Package `json:"files"`
	Revision         string            `json:"revision"`
}

func catalogPermission(collection string) (server.Permission, error) {
	switch collection {
	case "plugins":
		return server.Plugins, nil
	case "mods":
		return server.Files, nil
	}
	return "", server.ErrInvalid
}
func catalogLoader(t Target, collection string) (string, error) {
	if collection == "plugins" && server.SupportsPlugins(t.Type) || collection == "mods" && server.SupportsMods(t.Type) {
		return strings.ToLower(t.Type), nil
	}
	return "", server.ErrInvalid
}
func (s *Service) CatalogSearch(ctx context.Context, user, id uuid.UUID, collection, query string) ([]catalog.Project, error) {
	p, err := catalogPermission(collection)
	if err != nil {
		return nil, err
	}
	t, err := s.access(ctx, user, id, p)
	if err != nil {
		return nil, err
	}
	loader, err := catalogLoader(t, collection)
	if err != nil {
		return nil, err
	}
	return s.catalogClient().Search(ctx, loader, t.Version, query)
}
func (s *Service) resolvePreview(ctx context.Context, t Target, collection, project string) (InstallPreview, error) {
	loader, err := catalogLoader(t, collection)
	if err != nil {
		return InstallPreview{}, err
	}
	files, err := s.catalogClient().Resolve(ctx, project, loader, t.Version)
	if err != nil {
		return InstallPreview{}, err
	}
	preview := InstallPreview{Project: project, Software: t.Type, MinecraftVersion: t.Version, Files: files}
	data, _ := json.Marshal(preview)
	preview.Revision = revision(data)
	return preview, nil
}
func (s *Service) CatalogPreview(ctx context.Context, user, id uuid.UUID, collection, project string) (InstallPreview, error) {
	p, err := catalogPermission(collection)
	if err != nil {
		return InstallPreview{}, err
	}
	t, err := s.access(ctx, user, id, p)
	if err != nil {
		return InstallPreview{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return s.resolvePreview(ctx, t, collection, project)
}

// CatalogInstall preflights the complete bundle before writing. Successful writes are
// durably idempotent; interrupted remote writes retain UNKNOWN status for inspection.
func (s *Service) CatalogInstall(ctx context.Context, user, id, key uuid.UUID, collection, project, expected, rid string) (any, error) {
	p, err := catalogPermission(collection)
	if err != nil {
		return nil, err
	}
	if len(expected) != 64 {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, p, "catalog.install", map[string]string{"collection": collection, "project": project, "revision": expected}, rid, func(t Target) (any, error) {
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		preview, err := s.resolvePreview(ctx, t, collection, project)
		if err != nil {
			return nil, err
		}
		if preview.Revision != expected {
			return nil, ErrChanged
		}
		folder := "/" + collection
		f, err := s.inspect(ctx, t, folder, true)
		if err != nil {
			return nil, err
		}
		if f != nil && f.IsFile {
			return nil, server.ErrInvalid
		}
		client := s.catalogClient()
		var total int64
		pending := []catalog.Package{}
		for _, file := range preview.Files {
			if f != nil {
				existing, err := s.inspect(ctx, t, path.Join(folder, file.Filename), true)
				if err != nil {
					return nil, err
				}
				if existing != nil {
					if !existing.IsFile || existing.Size != file.Size {
						return nil, ErrChanged
					}
					r, err := s.Panel.DownloadFile(ctx, t.Identifier, path.Join(folder, file.Filename))
					if err != nil {
						return nil, control.ErrUnavailable
					}
					h := sha512.New()
					n, err := io.Copy(h, io.LimitReader(r, file.Size+1))
					r.Close()
					if err != nil || n != file.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), file.SHA512) {
						return nil, ErrChanged
					}
					continue
				}
			}
			file.Data, err = client.Download(ctx, file)
			if err != nil {
				return nil, err
			}
			if collection == "plugins" {
				err = validatePluginForSoftware(t.Type, file.Filename, file.Data)
			} else {
				err = validateMod(t.Type, file.Filename, file.Data)
			}
			if err != nil {
				return nil, err
			}
			total += file.Size
			pending = append(pending, file)
		}
		usage, err := s.Control.Panel.Resources(ctx, t.Identifier)
		if err != nil {
			return nil, control.ErrUnavailable
		}
		if usage.State != "offline" {
			return nil, ErrOffline
		}
		if usage.DiskBytes+total > int64(t.DiskMB)*1048576 {
			return nil, server.ErrInvalid
		}
		// Recheck membership after network downloads and before issuing file mutations.
		if _, err = s.Control.Access(ctx, t.q, user, id, p); err != nil {
			return nil, err
		}
		if f == nil {
			if err = s.Panel.CreateFolder(ctx, t.Identifier, "/", collection); err != nil {
				return nil, err
			}
		}
		for _, file := range pending {
			if err = s.Panel.UploadFile(ctx, t.Identifier, folder, file.Filename, file.Data); err != nil {
				return nil, control.ErrUnknown
			}
		}
		return map[string]any{"status": "installed", "files": preview.Files, "restart_required": true}, nil
	})
}

func (s *Service) catalogClient() *catalog.Client {
	if s.Catalog != nil {
		return s.Catalog
	}
	return catalog.New()
}
