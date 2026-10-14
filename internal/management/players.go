package management

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

type BannedPlayer struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}
type PlayerBans struct {
	Players  []BannedPlayer `json:"players"`
	Revision string         `json:"revision"`
}
type PlayerAction struct {
	Action    string `json:"action"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
	Revision  string `json:"revision"`
	Confirmed bool   `json:"confirmed"`
}

func (in PlayerAction) command() (string, error) {
	if !in.Confirmed || !validPlayerName(in.Name) || len(in.Revision) != 64 || !utf8.ValidString(in.Reason) || utf8.RuneCountInString(in.Reason) > 160 || strings.TrimSpace(in.Reason) != in.Reason {
		return "", server.ErrInvalid
	}
	for _, r := range in.Reason {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return "", server.ErrInvalid
		}
	}
	switch in.Action {
	case "kick", "ban":
		if in.Reason == "" {
			return "", server.ErrInvalid
		}
		return "minecraft:" + in.Action + " " + in.Name + " " + in.Reason, nil
	case "unban":
		if in.Reason != "" {
			return "", server.ErrInvalid
		}
		return "minecraft:pardon " + in.Name, nil
	default:
		return "", server.ErrInvalid
	}
}
func (s *Service) playerBans(ctx context.Context, t Target) (PlayerBans, error) {
	out := PlayerBans{Players: []BannedPlayer{}}
	entry, err := s.inspect(ctx, t, "/banned-players.json", true)
	if err != nil {
		return out, err
	}
	content := "[]"
	if entry != nil {
		file, err := s.read(ctx, t, "/banned-players.json")
		if err != nil {
			return out, err
		}
		content = file.Content
		if err = json.Unmarshal([]byte(content), &out.Players); err != nil {
			return out, server.ErrInvalid
		}
		if out.Players == nil {
			out.Players = []BannedPlayer{}
		}
	}
	for _, p := range out.Players {
		if !validPlayerName(p.Name) {
			return out, server.ErrInvalid
		}
		if _, err := uuid.Parse(p.UUID); err != nil {
			return out, server.ErrInvalid
		}
	}
	sort.Slice(out.Players, func(i, j int) bool {
		return strings.ToLower(out.Players[i].Name) < strings.ToLower(out.Players[j].Name)
	})
	out.Revision = revision([]byte(content))
	return out, nil
}
func (s *Service) PlayerBans(ctx context.Context, user, id uuid.UUID) (PlayerBans, error) {
	t, err := s.access(ctx, user, id, server.Settings)
	if err != nil {
		return PlayerBans{}, err
	}
	return s.playerBans(ctx, t)
}
func (in PlayerAction) applied(list PlayerBans) bool {
	for _, p := range list.Players {
		if strings.EqualFold(p.Name, in.Name) {
			return in.Action == "ban"
		}
	}
	return in.Action == "unban"
}
func (s *Service) ChangePlayer(ctx context.Context, user, id, key uuid.UUID, in PlayerAction, rid string) (any, error) {
	command, err := in.command()
	if err != nil {
		return nil, err
	}
	return s.mutate(ctx, user, id, key, server.Settings, "player."+in.Action, in, rid, func(t Target) (any, error) {
		list, err := s.playerBans(ctx, t)
		if err != nil {
			return nil, err
		}
		if list.Revision != in.Revision {
			return nil, ErrChanged
		}
		usage, err := s.Control.Panel.Resources(ctx, t.Identifier)
		if err != nil {
			return nil, control.ErrUnavailable
		}
		if usage.State != "running" {
			return nil, control.ErrState
		}
		if in.Action != "kick" && in.applied(list) {
			return map[string]any{"confirmed": true}, nil
		}
		if err = s.Control.Panel.SendCommand(ctx, t.Identifier, command); err != nil {
			return nil, err
		}
		// Command delivery cannot prove a kick: public status samples can hide players.
		if in.Action == "kick" {
			return map[string]any{"confirmed": false, "sent": true}, nil
		}
		wait, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-wait.Done():
				return map[string]any{"confirmed": false, "sent": true}, nil
			case <-ticker.C:
				list, err = s.playerBans(wait, t)
				if err == nil && in.applied(list) {
					return map[string]any{"confirmed": true, "sent": true}, nil
				}
			}
		}
	})
}
