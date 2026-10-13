package management

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

// ClientAddon contains player setup information, never raw configuration or credentials.
type ClientAddon struct {
	Filename        string `json:"filename"`
	Title           string `json:"title"`
	VersionID       string `json:"version_id,omitempty"`
	VersionName     string `json:"version_name,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	ClientSide      string `json:"client_side"`
	ConfigurationID string `json:"configuration_id,omitempty"`
	VoiceRequired   *bool  `json:"voice_required,omitempty"`
}
type ClientRequirements struct {
	Software         string        `json:"software"`
	MinecraftVersion string        `json:"minecraft_version"`
	Addons           []ClientAddon `json:"addons"`
}

// ClientRequirements is available to every server member with view permission.
// It reconciles installation receipts with files currently present through Panel.
// Uploaded and legacy JARs remain unknown; filenames are not proof of compatibility.
func (s *Service) ClientRequirements(ctx context.Context, user, id uuid.UUID) (ClientRequirements, error) {
	t, err := s.access(ctx, user, id, server.View)
	if err != nil {
		return ClientRequirements{}, err
	}
	out := ClientRequirements{Software: t.Type, MinecraftVersion: t.Version, Addons: []ClientAddon{}}
	collection := ""
	if server.SupportsPlugins(t.Type) {
		collection = "plugins"
	} else if server.SupportsMods(t.Type) {
		collection = "mods"
	} else {
		return out, nil
	}
	folder := "/" + collection
	entry, err := s.inspect(ctx, t, folder, true)
	if err != nil {
		return out, err
	}
	if entry == nil {
		return out, nil
	}
	if entry.IsFile {
		return out, server.ErrInvalid
	}
	files, err := s.Panel.ListFiles(ctx, t.Identifier, folder)
	if err != nil {
		return out, control.ErrUnavailable
	}
	known := map[string]catalog.Package{}
	rows, err := s.DB.Query(ctx, `SELECT result FROM management_operations WHERE server_id=$1 AND action='catalog.install' AND status='SUCCEEDED' AND (result->>'collection'=$2 OR NOT result ? 'collection') ORDER BY updated_at DESC`, id, collection)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err = rows.Scan(&data); err != nil {
			return out, err
		}
		var receipt struct {
			Identifier string            `json:"identifier"`
			Files      []catalog.Package `json:"files"`
		}
		if err = json.Unmarshal(data, &receipt); err != nil {
			return out, err
		}
		if receipt.Identifier != "" && receipt.Identifier != t.Identifier {
			continue
		}
		for _, file := range receipt.Files {
			if _, exists := known[file.Filename]; !exists {
				known[file.Filename] = file
			}
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	for _, file := range files {
		if !file.IsFile || file.IsSymlink || !strings.HasSuffix(strings.ToLower(file.Name), ".jar") {
			continue
		}
		addon := ClientAddon{Filename: file.Name, Title: file.Name, ClientSide: "unknown"}
		if meta, ok := known[file.Name]; ok && meta.Size == file.Size {
			addon.ProjectID = meta.ProjectID
			addon.VersionID = meta.VersionID
			addon.VersionName = meta.Name
			addon.ClientSide = catalog.ClientRequirement("", meta.ClientSide)
			if meta.Title != "" {
				addon.Title = meta.Title
			}
		}
		// Match the existing guided adapter. A manually uploaded matching name is
		// still unverified, so only catalog identity establishes voice metadata.
		if addon.ProjectID == "9eGKb6K1" {
			addon.ConfigurationID = VoiceAddon
			addon.Title = "Simple Voice Chat"
			content, exists, e := s.voiceFile(ctx, t)
			if e == nil && exists {
				values, _, e := voiceValues(content)
				if e == nil {
					addon.VoiceRequired = &values.Required
				}
			}
		}
		out.Addons = append(out.Addons, addon)
	}
	sort.Slice(out.Addons, func(i, j int) bool { return out.Addons[i].Filename < out.Addons[j].Filename })
	// Membership may have changed while querying Panel; do not return stale access.
	if _, err = s.Control.Access(ctx, t.q, user, id, server.View); err != nil {
		return ClientRequirements{}, err
	}
	return out, nil
}
