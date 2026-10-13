package management

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
)

type CatalogUpdate struct {
	Project string `json:"project"`
	Title   string `json:"title"`
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	Status  string `json:"status"`
}

// Check only catalog-tracked addons. Missing author metadata and unavailable
// releases remain explicit unknowns rather than assurances of compatibility.
func (s *Service) CatalogUpdates(ctx context.Context, user, id uuid.UUID, collection string) ([]CatalogUpdate, error) {
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
	setup, err := s.ClientRequirements(ctx, user, id)
	if err != nil {
		return nil, err
	}
	accessCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	out := []CatalogUpdate{}
	seen := map[string]bool{}
	for _, addon := range setup.Addons {
		if addon.ProjectID == "" || seen[addon.ProjectID] {
			continue
		}
		seen[addon.ProjectID] = true
		item := CatalogUpdate{Project: addon.ProjectID, Title: addon.Title, Current: addon.VersionName, Status: "unchecked"}
		if len(out) < 20 && ctx.Err() == nil {
			files, e := s.catalogClient().Resolve(ctx, addon.ProjectID, loader, t.Version)
			if e == nil {
				for _, file := range files {
					if file.ProjectID == addon.ProjectID {
						item.Latest = file.Name
						item.Status = "current"
						if file.VersionID != addon.VersionID {
							item.Status = "available"
						}
						break
					}
				}
			} else {
				item.Status = "unavailable"
			}
		}
		out = append(out, item)
	}
	if _, err = s.Control.Access(accessCtx, t.q, user, id, p); err != nil {
		return nil, err
	}
	return out, nil
}

// Replacement candidates come only from successful receipts for this exact
// Panel server. Manually uploaded files never become automatic deletion targets.
func (s *Service) catalogReplacements(ctx context.Context, t Target, collection string, next []catalog.Package) ([]catalog.Package, error) {
	wanted := map[string]catalog.Package{}
	for _, file := range next {
		wanted[file.ProjectID] = file
	}
	rows, err := s.DB.Query(ctx, `SELECT result FROM management_operations WHERE server_id=$1 AND action='catalog.install' AND status='SUCCEEDED' AND result->>'collection'=$2 ORDER BY updated_at DESC LIMIT 200`, t.ID, collection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]catalog.Package{}
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
		if receipt.Identifier != t.Identifier {
			continue
		}
		for _, file := range receipt.Files {
			if _, ok := known[file.Filename]; !ok {
				known[file.Filename] = file
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := []catalog.Package{}
	for _, old := range known {
		n, ok := wanted[old.ProjectID]
		if !ok || n.SHA512 == old.SHA512 {
			continue
		}
		if path.Base(old.Filename) != old.Filename || !strings.HasSuffix(strings.ToLower(old.Filename), ".jar") || len(old.SHA512) != 128 || old.Size <= 0 || old.Size > 64<<20 {
			return nil, ErrChanged
		}
		entry, e := s.inspect(ctx, t, path.Join("/"+collection, old.Filename), true)
		if e != nil {
			return nil, e
		}
		if entry == nil {
			continue
		}
		if !entry.IsFile || entry.Size != old.Size {
			return nil, ErrChanged
		}
		out = append(out, old)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Filename < out[j].Filename })
	return out, nil
}

func (s *Service) verifyCatalogFile(ctx context.Context, t Target, collection string, file catalog.Package) error {
	entry, err := s.inspect(ctx, t, path.Join("/"+collection, file.Filename), false)
	if err != nil {
		return ErrChanged
	}
	if !entry.IsFile || entry.Size != file.Size {
		return ErrChanged
	}
	r, err := s.Panel.DownloadFile(ctx, t.Identifier, path.Join("/"+collection, file.Filename))
	if err != nil {
		return control.ErrUnavailable
	}
	defer r.Close()
	h := sha512.New()
	n, err := io.Copy(h, io.LimitReader(r, file.Size+1))
	if err != nil {
		return control.ErrUnavailable
	}
	if n != file.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), file.SHA512) {
		return ErrChanged
	}
	return nil
}
