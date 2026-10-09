package management

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

const EULAURL = "https://www.minecraft.net/en-us/eula"

type EULAStatus struct {
	Accepted  bool   `json:"accepted"`
	CanAccept bool   `json:"can_accept"`
	URL       string `json:"url"`
}

func eulaAccepted(content string) bool {
	lines, err := parseProperties(content)
	if err != nil {
		return false
	}
	accepted := false
	for _, line := range lines {
		if line.key == "eula" {
			accepted = strings.EqualFold(line.value, "true")
		}
	}
	return accepted
}

// Accepted receives only the identifier resolved by Control.Access. Check the
// fixed container file without following symlinks; never infer consent from DB state.
func (s *Service) Accepted(ctx context.Context, identifier string) (bool, error) {
	if s.Panel == nil {
		return false, control.ErrNotConfigured
	}
	t := Target{Target: control.Target{Identifier: identifier}}
	entry, err := s.inspect(ctx, t, "/eula.txt", true)
	if err != nil || entry == nil {
		return false, err
	}
	file, err := s.read(ctx, t, "/eula.txt")
	if err != nil {
		return false, err
	}
	return eulaAccepted(file.Content), nil
}

func (s *Service) EULA(ctx context.Context, user, id uuid.UUID) (EULAStatus, error) {
	t, err := s.access(ctx, user, id, server.View)
	if err != nil {
		return EULAStatus{}, err
	}
	accepted, err := s.Accepted(ctx, t.Identifier)
	return EULAStatus{accepted, t.Role == server.Owner, EULAURL}, err
}

func (s *Service) AcceptEULA(ctx context.Context, user, id, key uuid.UUID, accepted bool, requestID string) (any, error) {
	if !accepted {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.AcceptEULA, "eula.accept", map[string]any{"accepted": true, "url": EULAURL}, requestID, func(t Target) (any, error) {
		already, err := s.Accepted(ctx, t.Identifier)
		if err != nil {
			return nil, err
		}
		if !already {
			if err := s.offline(ctx, t); err != nil {
				return nil, err
			}
			// Replace only the dedicated agreement file, never server.properties.
			if err := s.Panel.WriteFile(ctx, t.Identifier, "/eula.txt", []byte("# Accepted explicitly by the server owner via the hosting dashboard.\n# "+EULAURL+"\neula=true\n")); err != nil {
				return nil, err
			}
		}
		return EULAStatus{true, true, EULAURL}, nil
	})
}
