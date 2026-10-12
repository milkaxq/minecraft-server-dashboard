package management

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

type WhitelistedPlayer struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type Whitelist struct {
	Enabled  bool                `json:"enabled"`
	Players  []WhitelistedPlayer `json:"players"`
	Revision string              `json:"revision"`
}

type WhitelistChange struct {
	Action    string `json:"action"`
	Name      string `json:"name,omitempty"`
	Revision  string `json:"revision"`
	Confirmed bool   `json:"confirmed"`
}

func validPlayerName(name string) bool {
	if len(name) < 3 || len(name) > 16 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func (s *Service) whitelist(ctx context.Context, t Target) (Whitelist, error) {
	properties, err := s.read(ctx, t, "/server.properties")
	if err != nil {
		return Whitelist{}, err
	}
	lines, err := parseProperties(properties.Content)
	if err != nil {
		return Whitelist{}, err
	}
	out := Whitelist{Players: []WhitelistedPlayer{}}
	for _, line := range lines {
		if line.key == "white-list" {
			out.Enabled = strings.EqualFold(line.value, "true")
		}
	}
	entry, err := s.inspect(ctx, t, "/whitelist.json", true)
	if err != nil {
		return out, err
	}
	content := "[]"
	if entry != nil {
		file, err := s.read(ctx, t, "/whitelist.json")
		if err != nil {
			return out, err
		}
		content = file.Content
		if err = json.Unmarshal([]byte(content), &out.Players); err != nil {
			return out, server.ErrInvalid
		}
		if out.Players == nil {
			out.Players = []WhitelistedPlayer{}
		}
	}
	for _, player := range out.Players {
		if !validPlayerName(player.Name) {
			return out, server.ErrInvalid
		}
		if _, err := uuid.Parse(player.UUID); err != nil {
			return out, server.ErrInvalid
		}
	}
	sort.Slice(out.Players, func(i, j int) bool {
		return strings.ToLower(out.Players[i].Name) < strings.ToLower(out.Players[j].Name)
	})
	out.Revision = revision([]byte(properties.Revision + "\x00" + content))
	return out, nil
}

func (s *Service) Whitelist(ctx context.Context, user, id uuid.UUID) (Whitelist, error) {
	t, err := s.access(ctx, user, id, server.Settings)
	if err != nil {
		return Whitelist{}, err
	}
	return s.whitelist(ctx, t)
}

func (in WhitelistChange) command() (string, error) {
	if len(in.Revision) != 64 {
		return "", server.ErrInvalid
	}
	switch in.Action {
	case "add", "remove":
		if !validPlayerName(in.Name) || in.Action == "remove" && !in.Confirmed {
			return "", server.ErrInvalid
		}
		return "minecraft:whitelist " + in.Action + " " + in.Name, nil
	case "on", "off":
		if in.Name != "" || in.Action == "off" && !in.Confirmed {
			return "", server.ErrInvalid
		}
		return "minecraft:whitelist " + in.Action, nil
	default:
		return "", server.ErrInvalid
	}
}

func (in WhitelistChange) applied(list Whitelist) bool {
	if in.Action == "on" {
		return list.Enabled
	}
	if in.Action == "off" {
		return !list.Enabled
	}
	for _, player := range list.Players {
		if strings.EqualFold(player.Name, in.Name) {
			return in.Action == "add"
		}
	}
	return in.Action == "remove"
}

// Minecraft resolves account UUIDs using its own account mode and cache. Never
// accept a UUID from the browser or rewrite its live whitelist behind its back.
func (s *Service) ChangeWhitelist(ctx context.Context, user, id, key uuid.UUID, in WhitelistChange, rid string) (any, error) {
	command, err := in.command()
	if err != nil {
		return nil, err
	}
	return s.mutate(ctx, user, id, key, server.Settings, "whitelist."+in.Action, in, rid, func(t Target) (any, error) {
		list, err := s.whitelist(ctx, t)
		if err != nil {
			return nil, err
		}
		if list.Revision != in.Revision {
			return nil, ErrChanged
		}
		if in.applied(list) {
			return map[string]any{"confirmed": true}, nil
		}
		usage, err := s.Control.Panel.Resources(ctx, t.Identifier)
		if err != nil {
			return nil, control.ErrUnavailable
		}
		if usage.State != "running" {
			return nil, control.ErrState
		}
		if err = s.Control.Panel.SendCommand(ctx, t.Identifier, command); err != nil {
			return nil, err
		}
		// Panel confirms command delivery, not Minecraft execution. Observe the
		// persisted result before presenting success; never repeat a timed-out command.
		wait, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-wait.Done():
				return map[string]any{"confirmed": false}, nil
			case <-ticker.C:
				list, err = s.whitelist(wait, t)
				if err == nil && in.applied(list) {
					return map[string]any{"confirmed": true}, nil
				}
			}
		}
	})
}
