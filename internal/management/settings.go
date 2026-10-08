package management

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/uuid"

	"minecraft-hosting/internal/server"
)

type Settings struct {
	MOTD            string `json:"motd"`
	Difficulty      string `json:"difficulty"`
	Gamemode        string `json:"gamemode"`
	Hardcore        bool   `json:"hardcore"`
	PVP             bool   `json:"pvp"`
	MaxPlayers      int    `json:"max_players"`
	Whitelist       bool   `json:"whitelist"`
	OnlineMode      bool   `json:"online_mode"`
	ViewDistance    int    `json:"view_distance"`
	SpawnProtection int    `json:"spawn_protection"`
	Revision        string `json:"revision"`
}

func (s Settings) validate() error {
	if len(s.MOTD) > 256 || !utf8.ValidString(s.MOTD) || strings.ContainsAny(s.MOTD, "\r\n\x00") || s.MaxPlayers < 1 || s.MaxPlayers > 1000 || s.ViewDistance < 2 || s.ViewDistance > 32 || s.SpawnProtection < 0 || s.SpawnProtection > 1000 || len(s.Revision) != 64 {
		return server.ErrInvalid
	}
	for _, c := range s.MOTD {
		if c < 32 || c == 127 {
			return server.ErrInvalid
		}
	}
	if s.Difficulty != "peaceful" && s.Difficulty != "easy" && s.Difficulty != "normal" && s.Difficulty != "hard" {
		return server.ErrInvalid
	}
	if s.Gamemode != "survival" && s.Gamemode != "creative" && s.Gamemode != "adventure" && s.Gamemode != "spectator" {
		return server.ErrInvalid
	}
	return nil
}
func (s Settings) properties() map[string]string {
	return map[string]string{"motd": s.MOTD, "difficulty": s.Difficulty, "gamemode": s.Gamemode, "hardcore": strconv.FormatBool(s.Hardcore), "pvp": strconv.FormatBool(s.PVP), "max-players": strconv.Itoa(s.MaxPlayers), "white-list": strconv.FormatBool(s.Whitelist), "online-mode": strconv.FormatBool(s.OnlineMode), "view-distance": strconv.Itoa(s.ViewDistance), "spawn-protection": strconv.Itoa(s.SpawnProtection)}
}

type propertyLine struct{ raw, key, value string }

// parseProperties supports Java escapes, whitespace separators, continuation
// lines and duplicate keys. Unmanaged lines are preserved byte-for-byte.
func parseProperties(data string) ([]propertyLine, error) {
	physical := strings.SplitAfter(data, "\n")
	out := []propertyLine{}
	for i := 0; i < len(physical); i++ {
		raw := physical[i]
		logical := strings.TrimRight(physical[i], "\r\n")
		for {
			n := 0
			for j := len(logical) - 1; j >= 0 && logical[j] == '\\'; j-- {
				n++
			}
			if n%2 == 0 {
				break
			}
			if i+1 >= len(physical) {
				return nil, server.ErrInvalid
			}
			i++
			raw += physical[i]
			logical = logical[:len(logical)-1] + strings.TrimLeft(strings.TrimRight(physical[i], "\r\n"), " \t\f")
		}
		text := strings.TrimLeft(logical, " \t\f")
		line := propertyLine{raw: raw}
		if text == "" || text[0] == '#' || text[0] == '!' {
			out = append(out, line)
			continue
		}
		at := len(text)
		escaped := false
		for j := 0; j < len(text); j++ {
			c := text[j]
			if !escaped && (c == '=' || c == ':' || c == ' ' || c == '\t' || c == '\f') {
				at = j
				break
			}
			if c == '\\' {
				escaped = !escaped
			} else {
				escaped = false
			}
		}
		value := ""
		if at < len(text) {
			rest := strings.TrimLeft(text[at:], " \t\f")
			if len(rest) > 0 && (rest[0] == '=' || rest[0] == ':') {
				rest = rest[1:]
			}
			value = strings.TrimLeft(rest, " \t\f")
		}
		var err error
		if line.key, err = unescape(text[:at]); err != nil {
			return nil, err
		}
		if line.value, err = unescape(value); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, nil
}
func unescape(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		i++
		if i >= len(value) {
			return "", server.ErrInvalid
		}
		switch value[i] {
		case 't':
			out.WriteByte('\t')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			if i+4 >= len(value) {
				return "", server.ErrInvalid
			}
			n, err := strconv.ParseUint(value[i+1:i+5], 16, 16)
			if err != nil {
				return "", server.ErrInvalid
			}
			i += 4
			r := rune(n)
			if utf16.IsSurrogate(r) {
				if r > 0xDBFF || i+6 >= len(value) || value[i+1:i+3] != "\\u" {
					return "", server.ErrInvalid
				}
				second, err := strconv.ParseUint(value[i+3:i+7], 16, 16)
				if err != nil || second < 0xDC00 || second > 0xDFFF {
					return "", server.ErrInvalid
				}
				r = utf16.DecodeRune(r, rune(second))
				i += 6
			}
			out.WriteRune(r)
		default:
			out.WriteByte(value[i])
		}
	}
	if !utf8.ValidString(out.String()) {
		return "", server.ErrInvalid
	}
	return out.String(), nil
}
func escapeProperty(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '=', ':', '#', '!', ' ':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if r >= 32 && r <= 126 {
				b.WriteRune(r)
			} else {
				for _, n := range utf16.Encode([]rune{r}) {
					fmt.Fprintf(&b, "\\u%04x", n)
				}
			}
		}
	}
	return b.String()
}
func (s *Service) Settings(ctx context.Context, user, id uuid.UUID) (Settings, error) {
	t, err := s.access(ctx, user, id, server.Settings)
	if err != nil {
		return Settings{}, err
	}
	file, err := s.read(ctx, t, "/server.properties")
	if err != nil {
		return Settings{}, err
	}
	lines, err := parseProperties(file.Content)
	if err != nil {
		return Settings{}, err
	}
	props := map[string]string{}
	for _, v := range lines {
		if v.key != "" {
			props[v.key] = v.value
		}
	}
	get := func(k, def string) string {
		if v, ok := props[k]; ok {
			return v
		}
		return def
	}
	number := func(k string, def int) int {
		n, e := strconv.Atoi(get(k, strconv.Itoa(def)))
		if e != nil {
			return def
		}
		return n
	}
	boolean := func(k string, def bool) bool { return strings.EqualFold(get(k, strconv.FormatBool(def)), "true") }
	return Settings{MOTD: get("motd", "A Minecraft Server"), Difficulty: get("difficulty", "easy"), Gamemode: get("gamemode", "survival"), Hardcore: boolean("hardcore", false), PVP: boolean("pvp", true), MaxPlayers: number("max-players", 20), Whitelist: boolean("white-list", false), OnlineMode: boolean("online-mode", true), ViewDistance: number("view-distance", 10), SpawnProtection: number("spawn-protection", 16), Revision: file.Revision}, nil
}
func (s *Service) SaveSettings(ctx context.Context, user, id, key uuid.UUID, in Settings, rid string) (any, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	return s.mutate(ctx, user, id, key, server.Settings, "settings.save", in, rid, func(t Target) (any, error) {
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		file, err := s.read(ctx, t, "/server.properties")
		if err != nil {
			return nil, err
		}
		if file.Revision != in.Revision {
			return nil, ErrChanged
		}
		lines, err := parseProperties(file.Content)
		if err != nil {
			return nil, err
		}
		props := in.properties()
		var b strings.Builder
		for _, v := range lines {
			if _, managed := props[v.key]; !managed {
				b.WriteString(v.raw)
			}
		}
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		for _, k := range []string{"motd", "difficulty", "gamemode", "hardcore", "pvp", "max-players", "white-list", "online-mode", "view-distance", "spawn-protection"} {
			b.WriteString(k + "=" + escapeProperty(props[k]) + "\n")
		}
		if err = s.Panel.WriteFile(ctx, t.Identifier, "/server.properties", []byte(b.String())); err != nil {
			return nil, err
		}
		return map[string]any{"status": "saved", "restart_required": true, "revision": revision([]byte(b.String()))}, nil
	})
}
