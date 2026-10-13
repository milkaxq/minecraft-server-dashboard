package management

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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
func (s *Service) CatalogSearch(ctx context.Context, user, id uuid.UUID, collection string, options catalog.SearchOptions) (catalog.SearchPage, error) {
	p, err := catalogPermission(collection)
	if err != nil {
		return catalog.SearchPage{}, err
	}
	t, err := s.access(ctx, user, id, p)
	if err != nil {
		return catalog.SearchPage{}, err
	}
	loader, err := catalogLoader(t, collection)
	if err != nil {
		return catalog.SearchPage{}, err
	}
	if options.Validate() != nil {
		return catalog.SearchPage{}, server.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.catalogClient().SearchPage(ctx, loader, t.Version, options)
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
		report := func(phase string, completed, total int) error {
			data, _ := json.Marshal(map[string]any{"collection": collection, "project": project, "phase": phase, "completed": completed, "total": total})
			var updated bool
			return t.q.QueryRow(ctx, `UPDATE management_operations SET result=$4,updated_at=now() WHERE server_id=$1 AND user_id=$2 AND idempotency_key=$3 AND action='catalog.install' AND status='PENDING' RETURNING true`, id, user, key, data).Scan(&updated)
		}
		if err := report("Resolving compatible releases", 0, 0); err != nil {
			return nil, err
		}
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
		completed := 0
		units := len(preview.Files) * 2
		for _, file := range preview.Files {
			if err := report("Downloading and checking files", completed, units); err != nil {
				return nil, err
			}
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
					completed += 2
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
			completed++
		}
		if err := report("Checking server storage", completed, units); err != nil {
			return nil, err
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
			if err := report("Installing files", completed, units); err != nil {
				return nil, control.ErrUnknown
			}
			if err = s.Panel.UploadFile(ctx, t.Identifier, folder, file.Filename, file.Data); err != nil {
				return nil, control.ErrUnknown
			}
			completed++
		}
		return map[string]any{"collection": collection, "project": project, "identifier": t.Identifier, "phase": "Installed", "completed": units, "total": units, "status": "installed", "files": preview.Files, "restart_required": true}, nil
	})
}

func (s *Service) catalogClient() *catalog.Client {
	if s.Catalog != nil {
		return s.Catalog
	}
	return catalog.New()
}

// CatalogProgress exposes only safe metadata for this customer's operation.
func (s *Service) CatalogProgress(ctx context.Context, user, id, key uuid.UUID, collection string) (any, error) {
	p, err := catalogPermission(collection)
	if err != nil {
		return nil, err
	}
	if _, err = s.Control.Access(ctx, s.DB, user, id, p); err != nil {
		return nil, err
	}
	var status string
	var data []byte
	err = s.DB.QueryRow(ctx, `SELECT status,result FROM management_operations WHERE server_id=$1 AND user_id=$2 AND idempotency_key=$3 AND action='catalog.install' AND result->>'collection'=$4`, id, user, key, collection).Scan(&status, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, server.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var progress struct {
		Phase     string `json:"phase"`
		Completed int    `json:"completed"`
		Total     int    `json:"total"`
	}
	if err = json.Unmarshal(data, &progress); err != nil {
		return nil, err
	}
	return map[string]any{"status": status, "phase": progress.Phase, "completed": progress.Completed, "total": progress.Total}, nil
}

// Receipts identify catalog projects; current files ensure deleted packages lose their badge.
func (s *Service) CatalogInstalled(ctx context.Context, user, id uuid.UUID, collection string) (any, error) {
	p, err := catalogPermission(collection)
	if err != nil {
		return nil, err
	}
	t, err := s.access(ctx, user, id, p)
	if err != nil {
		return nil, err
	}
	if _, err = catalogLoader(t, collection); err != nil {
		return nil, err
	}
	folder := "/" + collection
	entry, err := s.inspect(ctx, t, folder, true)
	if err != nil {
		return nil, err
	}
	projects := []string{}
	if entry != nil && entry.IsFile {
		return nil, server.ErrInvalid
	}
	if entry == nil {
		return map[string]any{"projects": projects}, nil
	}
	files, err := s.Panel.ListFiles(ctx, t.Identifier, folder)
	if err != nil {
		return nil, control.ErrUnavailable
	}
	present := map[string]int64{}
	for _, file := range files {
		if file.IsFile && !file.IsSymlink {
			present[file.Name] = file.Size
		}
	}
	rows, err := s.DB.Query(ctx, `SELECT result FROM management_operations WHERE server_id=$1 AND action='catalog.install' AND status='SUCCEEDED' AND (result->>'collection'=$2 OR NOT result ? 'collection')`, id, collection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var data []byte
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		var receipt struct {
			Identifier string            `json:"identifier"`
			Files      []catalog.Package `json:"files"`
		}
		if err = json.Unmarshal(data, &receipt); err != nil {
			return nil, err
		}
		if receipt.Identifier != "" && receipt.Identifier != t.Identifier {
			continue
		}
		for _, file := range receipt.Files {
			size, exists := present[file.Filename]
			if exists && size == file.Size && file.ProjectID != "" && !seen[file.ProjectID] {
				projects = append(projects, file.ProjectID)
				seen[file.ProjectID] = true
			}
		}
	}
	return map[string]any{"projects": projects}, rows.Err()
}
