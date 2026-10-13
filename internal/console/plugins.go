package console

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

type PluginState struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}
type PluginCheck struct {
	Confirmed bool          `json:"confirmed"`
	Plugins   []PluginState `json:"plugins"`
	CheckedAt time.Time     `json:"checked_at"`
	UptimeMS  int64         `json:"uptime_ms"`
	Reason    string        `json:"reason,omitempty"`
}

// CheckPlugins observes only fresh output from a fixed, read-only Minecraft
// command. Delivery alone and uncoloured output never establish enabled state.
func (s *Service) CheckPlugins(ctx context.Context, user, id, key uuid.UUID, rid string) (PluginCheck, error) {
	target, err := s.Control.Check(ctx, user, id, server.Plugins)
	if err != nil {
		return PluginCheck{}, err
	}
	var software string
	if err = s.DB.QueryRow(ctx, "SELECT server_type FROM servers WHERE id=$1", id).Scan(&software); err != nil {
		return PluginCheck{}, err
	}
	if !server.SupportsPlugins(software) {
		return PluginCheck{}, server.ErrInvalid
	}
	usage, err := s.Control.Panel.Resources(ctx, target.Identifier)
	if err != nil {
		return PluginCheck{}, control.ErrUnavailable
	}
	if usage.State != "running" {
		return PluginCheck{}, control.ErrState
	}
	if !s.Configured() {
		return PluginCheck{}, control.ErrNotConfigured
	}
	if !s.acquire(user) {
		return PluginCheck{}, control.ErrBusy
	}
	defer s.release(user)
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if s.Lifetime != nil {
		stop := context.AfterFunc(s.Lifetime, cancel)
		defer stop()
	}
	credentials, err := s.Source.WebsocketCredentials(ctx, target.Identifier)
	if err != nil {
		return PluginCheck{}, control.ErrUnavailable
	}
	if err = s.ValidateSocket(credentials.Socket); err != nil {
		return PluginCheck{}, err
	}
	upstream, response, err := websocket.Dial(ctx, credentials.Socket, &websocket.DialOptions{HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, HTTPHeader: http.Header{"Origin": []string{s.PanelOrigin}}})
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return PluginCheck{}, control.ErrUnavailable
	}
	defer upstream.CloseNow()
	upstream.SetReadLimit(64 << 10)
	if err = write(ctx, upstream, message{Event: "auth", Args: []string{credentials.Token}}); err != nil {
		return PluginCheck{}, control.ErrUnavailable
	}
	_, data, err := upstream.Read(ctx)
	var first message
	if err != nil || json.Unmarshal(data, &first) != nil || first.Event != "auth success" {
		return PluginCheck{}, control.ErrUnavailable
	}
	if _, err = s.Control.Check(ctx, user, id, server.Plugins); err != nil {
		return PluginCheck{}, err
	}
	// Control owns authorization, auditing and at-most-once command delivery.
	if err = s.Control.Command(ctx, user, id, key, "bukkit:plugins", rid); err != nil {
		return PluginCheck{}, err
	}
	out := PluginCheck{Plugins: []PluginState{}, CheckedAt: time.Now().UTC(), UptimeMS: usage.UptimeMillis, Reason: "No complete coloured plugin response was received. Open Console to inspect startup errors."}
	parser := pluginListParser{expected: -1, states: map[string]bool{}}
	wait, done := context.WithTimeout(ctx, 6*time.Second)
	defer done()
	for bytes, frames := 0, 0; frames < 128 && bytes < 256<<10; frames++ {
		_, data, e := upstream.Read(wait)
		if e != nil {
			break
		}
		bytes += len(data)
		var frame message
		if json.Unmarshal(data, &frame) != nil || frame.Event != "console output" {
			continue
		}
		for _, arg := range frame.Args {
			for _, line := range strings.Split(arg, "\n") {
				parser.add(line)
			}
		}
		if parser.complete() {
			out.Confirmed = true
			out.Reason = ""
			for name, enabled := range parser.states {
				out.Plugins = append(out.Plugins, PluginState{name, enabled})
			}
			break
		}
	}
	if _, err = s.Control.Check(ctx, user, id, server.Plugins); err != nil {
		return PluginCheck{}, err
	}
	after, err := s.Control.Panel.Resources(ctx, target.Identifier)
	if err != nil || after.State != "running" || after.UptimeMillis < usage.UptimeMillis {
		out.Confirmed = false
		out.Plugins = []PluginState{}
		out.Reason = "Server state changed during the check. Check again once it is online."
	}
	sort.Slice(out.Plugins, func(i, j int) bool { return out.Plugins[i].Name < out.Plugins[j].Name })
	return out, nil
}

var pluginHeader = regexp.MustCompile(`^(?:ℹ )?(?:Server )?Plugins \(([0-9]{1,3})\):`)
var consolePrefix = regexp.MustCompile(`^\[[0-9:]+(?: [A-Z]+)?\](?: \[[^\]]+\])?:\s*`)
var colourCode = regexp.MustCompile(`\x1b\[[0-9;]*m|§[0-9a-fk-or]`)
var pluginContinuation = regexp.MustCompile(`^[*A-Za-z0-9_.-]+(?:,\s*[*A-Za-z0-9_.-]+)*,?$`)
var pluginName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

type pluginListParser struct {
	expected int
	states   map[string]bool
	listing  bool
	invalid  bool
}

func (p *pluginListParser) complete() bool {
	return !p.invalid && p.expected >= 0 && len(p.states) == p.expected
}
func (p *pluginListParser) add(line string) {
	plain := colourCode.ReplaceAllString(line, "")
	plain = consolePrefix.ReplaceAllString(plain, "")
	header := pluginHeader.FindStringSubmatch(plain)
	if len(header) > 0 {
		// Startup inventory is not the command response and does not imply enabled.
		if strings.Contains(plain, "PluginInitializerManager") {
			return
		}
		if p.expected >= 0 {
			p.invalid = true
			return
		}
		p.expected, _ = strconv.Atoi(header[1])
		p.listing = true
	} else if !p.listing || !(strings.HasPrefix(plain, "- ") || strings.HasPrefix(plain, " ") || strings.HasPrefix(plain, "Paper Plugins") || strings.HasPrefix(plain, "Bukkit Plugins") || pluginContinuation.MatchString(plain)) {
		return
	}
	for _, loc := range colourCode.FindAllStringIndex(line, -1) {
		code := line[loc[0]:loc[1]]
		enabled, known := pluginColour(code)
		if !known {
			continue
		}
		rest := line[loc[1]:]
		if end := colourCode.FindStringIndex(rest); end != nil {
			rest = rest[:end[0]]
		}
		name := strings.TrimSpace(strings.TrimRight(rest, ", "))
		// Unknown decorations/alias formats remain unconfirmed, never guessed.
		if !pluginName.MatchString(name) {
			continue
		}
		if previous, exists := p.states[name]; exists && previous != enabled {
			p.invalid = true
			return
		}
		p.states[name] = enabled
	}
	if len(p.states) > p.expected {
		p.invalid = true
	}
}

func pluginColour(code string) (bool, bool) {
	if code == "§a" {
		return true, true
	}
	if code == "§c" {
		return false, true
	}
	if !strings.HasPrefix(code, "\x1b[") {
		return false, false
	}
	values := strings.Split(strings.TrimSuffix(strings.TrimPrefix(code, "\x1b["), "m"), ";")
	enabled, known := false, false
	for i := 0; i < len(values); i++ {
		switch values[i] {
		case "", "0", "39":
			known = false
		case "32", "92":
			enabled, known = true, true
		case "31", "91":
			enabled, known = false, true
		case "30", "33", "34", "35", "36", "37", "90", "93", "94", "95", "96", "97":
			known = false
		case "38":
			known = false
			if i+4 < len(values) && values[i+1] == "2" {
				rgb := strings.Join(values[i+2:i+5], ";")
				enabled = rgb == "85;255;85"
				known = enabled || rgb == "255;85;85"
				i += 4
			} else if i+2 < len(values) && values[i+1] == "5" {
				enabled = values[i+2] == "10"
				known = enabled || values[i+2] == "9"
				i += 2
			}
		}
	}
	return enabled, known
}
