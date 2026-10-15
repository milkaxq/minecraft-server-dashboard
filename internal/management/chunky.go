package management

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

type ChunkyWorld struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// ChunkyWorlds lists command-safe, existing world roots without scanning terrain,
// reading level.dat or returning other server.properties values to the caller.
func (s *Service) ChunkyWorlds(ctx context.Context, user, id uuid.UUID) ([]ChunkyWorld, error) {
	t, err := s.access(ctx, user, id, server.Console)
	if err != nil {
		return nil, err
	}
	text, err := s.read(ctx, t, "/server.properties")
	if err != nil {
		return nil, err
	}
	properties, err := parseProperties(text.Content)
	if err != nil {
		return nil, err
	}
	name := "world"
	for _, line := range properties {
		if line.key == "level-name" {
			name = line.value
		}
	}
	if !chunkyWorldName(name) {
		return nil, ErrAddonConfig
	}
	candidates := []ChunkyWorld{{Name: name, Label: "Main world"}}
	if server.SupportsPlugins(t.Type) {
		candidates = append(candidates, ChunkyWorld{Name: name + "_nether", Label: "Nether"}, ChunkyWorld{Name: name + "_the_end", Label: "The End"})
	}
	entries, err := s.Panel.ListFiles(ctx, t.Identifier, "/")
	if err != nil {
		return nil, control.ErrUnavailable
	}
	out := []ChunkyWorld{}
	for _, candidate := range candidates {
		if !chunkyWorldName(candidate.Name) {
			continue
		}
		for _, entry := range entries {
			if entry.Name == candidate.Name && !entry.IsFile && !entry.IsSymlink {
				out = append(out, candidate)
				break
			}
		}
	}
	return out, nil
}

func chunkyWorldName(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	return strings.IndexFunc(value, func(c rune) bool {
		return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-')
	}) == -1
}
