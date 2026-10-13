package management

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

var ErrAddonConfig = errors.New("addon configuration needs review in Files")
var ErrAddonPort = errors.New("prepare a voice chat port before saving")

const VoiceAddon = "simple-voice-chat"

type AddonField struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Help  string  `json:"help"`
	Kind  string  `json:"kind"`
	Min   float64 `json:"min,omitempty"`
	Max   float64 `json:"max,omitempty"`
}
type VoiceValues struct {
	Distance   float64 `json:"max_voice_distance"`
	Whisper    float64 `json:"whisper_distance"`
	Groups     bool    `json:"enable_groups"`
	Required   bool    `json:"force_voice_chat"`
	Recording  bool    `json:"allow_recording"`
	Spectators bool    `json:"spectator_interaction"`
}

func voiceDefaults() VoiceValues {
	return VoiceValues{Distance: 48, Whisper: 24, Groups: true, Recording: true}
}
func (v VoiceValues) valid() bool {
	return !math.IsNaN(v.Distance) && !math.IsInf(v.Distance, 0) && !math.IsNaN(v.Whisper) && !math.IsInf(v.Whisper, 0) && v.Distance >= 1 && v.Distance <= 256 && v.Whisper >= 1 && v.Whisper <= v.Distance
}
func (v VoiceValues) properties() map[string]string {
	return map[string]string{"max_voice_distance": strconv.FormatFloat(v.Distance, 'f', -1, 64), "whisper_distance": strconv.FormatFloat(v.Whisper, 'f', -1, 64), "enable_groups": strconv.FormatBool(v.Groups), "force_voice_chat": strconv.FormatBool(v.Required), "allow_recording": strconv.FormatBool(v.Recording), "spectator_interaction": strconv.FormatBool(v.Spectators)}
}

type VoiceNetwork struct {
	Status     string `json:"status"`
	Hostname   string `json:"hostname,omitempty"`
	Port       int    `json:"port,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Allocation int    `json:"-"`
}
type VoiceConfig struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Fields     []AddonField `json:"fields"`
	Values     VoiceValues  `json:"values"`
	Defaults   VoiceValues  `json:"defaults"`
	Revision   string       `json:"revision"`
	Installed  bool         `json:"installed"`
	Exists     bool         `json:"config_exists"`
	Configured bool         `json:"configured"`
	Network    VoiceNetwork `json:"network"`
}
type VoiceInput struct {
	VoiceValues
	Revision string `json:"revision"`
}

func voicePath(kind string) (string, string, error) {
	if server.SupportsPlugins(kind) {
		return "/plugins/voicechat/voicechat-server.properties", "plugins", nil
	}
	if server.SupportsMods(kind) {
		return "/config/voicechat/voicechat-server.properties", "mods", nil
	}
	return "", "", server.ErrInvalid
}
func (s *Service) voiceFile(ctx context.Context, t Target) (string, bool, error) {
	file, _, err := voicePath(t.Type)
	if err != nil {
		return "", false, err
	}
	entry, err := s.inspect(ctx, t, file, true)
	if errors.Is(err, server.ErrNotFound) || err == nil && entry == nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	text, err := s.read(ctx, t, file)
	return text.Content, true, err
}
func voiceRevision(content string, exists bool) string {
	return revision([]byte(fmt.Sprintf("%t\x00%s", exists, content)))
}
func (s *Service) voiceInstalled(ctx context.Context, t Target) (bool, error) {
	_, collection, err := voicePath(t.Type)
	if err != nil {
		return false, err
	}
	folder := "/" + collection
	entry, err := s.inspect(ctx, t, folder, true)
	if err != nil {
		return false, err
	}
	if entry == nil {
		return false, nil
	}
	if entry.IsFile {
		return false, server.ErrInvalid
	}
	files, err := s.Panel.ListFiles(ctx, t.Identifier, folder)
	if err != nil {
		return false, control.ErrUnavailable
	}
	for _, file := range files {
		name := strings.ToLower(file.Name)
		// Standard vendor filenames and catalog downloads contain 'voicechat'.
		// No code is executed or trusted based on this detection.
		if file.IsFile && !file.IsSymlink && strings.Contains(name, "voicechat") && strings.HasSuffix(name, ".jar") {
			return true, nil
		}
	}
	return false, nil
}
func (s *Service) voiceNetwork(ctx context.Context, t Target) (VoiceNetwork, error) {
	n := VoiceNetwork{Status: "NONE"}
	err := t.q.QueryRow(ctx, `SELECT p.status,a.hostname,a.port,a.pterodactyl_allocation_id FROM server_addon_ports p JOIN node_allocations a ON a.id=p.allocation_id WHERE p.server_id=$1`, t.ID).Scan(&n.Status, &n.Hostname, &n.Port, &n.Allocation)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return n, err
	}
	var status string
	var code *string
	err = t.q.QueryRow(ctx, `SELECT status,last_error FROM jobs WHERE server_id=$1 AND action='VOICE_PORT' ORDER BY created_at DESC,id DESC LIMIT 1`, t.ID).Scan(&status, &code)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return n, err
	}
	if status == "PENDING" || status == "RUNNING" {
		n.Status = "PREPARING"
	}
	if status == "FAILED" {
		n.Status = "FAILED"
		if code != nil {
			n.ErrorCode = *code
		}
	}
	return n, nil
}
func voiceValues(content string) (VoiceValues, map[string]string, error) {
	v := voiceDefaults()
	props := map[string]string{}
	lines, err := parseProperties(content)
	if err != nil {
		return v, props, ErrAddonConfig
	}
	for _, line := range lines {
		if line.key != "" {
			props[line.key] = line.value
		}
	}
	for _, number := range []struct {
		key  string
		dest *float64
	}{{"max_voice_distance", &v.Distance}, {"whisper_distance", &v.Whisper}} {
		if raw, found := props[number.key]; found {
			value, e := strconv.ParseFloat(raw, 64)
			if e != nil {
				return v, props, ErrAddonConfig
			}
			*number.dest = value
		}
	}
	for _, toggle := range []struct {
		key  string
		dest *bool
	}{{"enable_groups", &v.Groups}, {"force_voice_chat", &v.Required}, {"allow_recording", &v.Recording}, {"spectator_interaction", &v.Spectators}} {
		if raw, found := props[toggle.key]; found {
			if !strings.EqualFold(raw, "true") && !strings.EqualFold(raw, "false") {
				return v, props, ErrAddonConfig
			}
			*toggle.dest = strings.EqualFold(raw, "true")
		}
	}
	if !v.valid() {
		return v, props, ErrAddonConfig
	}
	return v, props, nil
}
func (s *Service) VoiceChat(ctx context.Context, user, id uuid.UUID) (VoiceConfig, error) {
	target, err := s.Control.AccessAddonRead(ctx, s.DB, user, id)
	if err != nil {
		return VoiceConfig{}, err
	}
	t := Target{Target: target, q: s.DB}
	if err = s.DB.QueryRow(ctx, "SELECT server_type,disk_mb FROM servers WHERE id=$1", id).Scan(&t.Type, &t.DiskMB); err != nil {
		return VoiceConfig{}, err
	}
	if s.Panel == nil {
		return VoiceConfig{}, control.ErrNotConfigured
	}
	if err = s.Panel.Ready(ctx, t.Identifier); err != nil {
		return VoiceConfig{}, control.ErrUnavailable
	}
	installed, err := s.voiceInstalled(ctx, t)
	if err != nil {
		return VoiceConfig{}, err
	}
	content, exists, err := s.voiceFile(ctx, t)
	if err != nil {
		return VoiceConfig{}, err
	}
	values, props, err := voiceValues(content)
	if err != nil {
		return VoiceConfig{}, err
	}
	network, err := s.voiceNetwork(ctx, t)
	if err != nil {
		return VoiceConfig{}, err
	}
	fields := []AddonField{
		{Key: "max_voice_distance", Label: "Voice distance", Help: "How far nearby players can hear each other, in blocks.", Kind: "number", Min: 1, Max: 256},
		{Key: "whisper_distance", Label: "Whisper distance", Help: "Must be no greater than the normal voice distance.", Kind: "number", Min: 1, Max: 256},
		{Key: "enable_groups", Label: "Group chats", Help: "Let players create private voice groups.", Kind: "boolean"},
		{Key: "force_voice_chat", Label: "Require the client mod", Help: "Players without Simple Voice Chat will be disconnected. This does not install it for them.", Kind: "boolean"},
		{Key: "allow_recording", Label: "Allow recording", Help: "Allow the mod's built-in recording feature.", Kind: "boolean"},
		{Key: "spectator_interaction", Label: "Spectators can talk", Help: "Allow spectators to talk with other players.", Kind: "boolean"},
	}
	configured := exists && network.Status == "READY" && props["port"] == strconv.Itoa(network.Port) && props["bind_address"] == "*" && props["voice_host"] == net.JoinHostPort(network.Hostname, strconv.Itoa(network.Port))
	return VoiceConfig{ID: VoiceAddon, Name: "Simple Voice Chat", Fields: fields, Values: values, Defaults: voiceDefaults(), Revision: voiceRevision(content, exists), Installed: installed, Exists: exists, Configured: configured, Network: network}, nil
}
func (s *Service) SaveVoiceChat(ctx context.Context, user, id, key uuid.UUID, in VoiceInput, rid string) (any, error) {
	if !in.VoiceValues.valid() || len(in.Revision) != 64 {
		return nil, server.ErrInvalid
	}
	return s.mutate(ctx, user, id, key, server.Settings, "addon.voicechat.save", in, rid, func(t Target) (any, error) {
		if err := s.offline(ctx, t); err != nil {
			return nil, err
		}
		installed, err := s.voiceInstalled(ctx, t)
		if err != nil {
			return nil, err
		}
		if !installed {
			return nil, server.ErrInvalid
		}
		content, exists, err := s.voiceFile(ctx, t)
		if err != nil {
			return nil, err
		}
		if voiceRevision(content, exists) != in.Revision {
			return nil, ErrChanged
		}
		network, err := s.voiceNetwork(ctx, t)
		if err != nil {
			return nil, err
		}
		if network.Status != "READY" {
			return nil, ErrAddonPort
		}
		reader, ok := s.Panel.(pterodactyl.NetworkReader)
		if !ok {
			return nil, control.ErrNotConfigured
		}
		allocations, err := reader.NetworkAllocations(ctx, t.Identifier)
		if err != nil {
			return nil, control.ErrUnavailable
		}
		if !slices.Contains(allocations, network.Allocation) {
			return nil, ErrAddonPort
		}
		lines, err := parseProperties(content)
		if err != nil {
			return nil, ErrAddonConfig
		}
		props := in.properties()
		props["port"] = strconv.Itoa(network.Port)
		props["bind_address"] = "*"
		props["voice_host"] = net.JoinHostPort(network.Hostname, strconv.Itoa(network.Port))
		var output strings.Builder
		for _, line := range lines {
			if _, managed := props[line.key]; !managed {
				output.WriteString(line.raw)
			}
		}
		if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n") {
			output.WriteByte('\n')
		}
		for _, key := range []string{"port", "bind_address", "voice_host", "max_voice_distance", "whisper_distance", "enable_groups", "force_voice_chat", "allow_recording", "spectator_interaction"} {
			output.WriteString(key + "=" + escapeProperty(props[key]) + "\n")
		}
		data := []byte(output.String())
		if len(data) > pterodactyl.MaxTextBytes {
			return nil, server.ErrInvalid
		}
		usage, err := s.Control.Panel.Resources(ctx, t.Identifier)
		if err != nil {
			return nil, control.ErrUnavailable
		}
		if usage.State != "offline" {
			return nil, ErrOffline
		}
		if usage.DiskBytes+int64(len(data)-len(content)) > int64(t.DiskMB)*1048576 {
			return nil, server.ErrInvalid
		}
		if _, err = s.Control.Access(ctx, t.q, user, id, server.Settings); err != nil {
			return nil, err
		}
		file, _, _ := voicePath(t.Type)
		parent := "/"
		for _, segment := range strings.Split(strings.TrimPrefix(path.Dir(file), "/"), "/") {
			dir := path.Join(parent, segment)
			entry, e := s.inspect(ctx, t, dir, true)
			if e != nil {
				return nil, e
			}
			if entry == nil {
				if _, e = s.Control.Access(ctx, t.q, user, id, server.Settings); e != nil {
					return nil, e
				}
				if e = s.Panel.CreateFolder(ctx, t.Identifier, parent, segment); e != nil {
					return nil, control.ErrUnknown
				}
			} else if entry.IsFile {
				return nil, server.ErrInvalid
			}
			parent = dir
		}
		// Recheck after network reads and directory creation, including changes
		// made outside Blockyard through SFTP. Panel does not offer atomic CAS.
		current, currentExists, err := s.voiceFile(ctx, t)
		if err != nil {
			return nil, err
		}
		if voiceRevision(current, currentExists) != in.Revision {
			return nil, ErrChanged
		}
		if _, err = s.Control.Access(ctx, t.q, user, id, server.Settings); err != nil {
			return nil, err
		}
		if err = s.Panel.WriteFile(ctx, t.Identifier, file, data); err != nil {
			return nil, control.ErrUnknown
		}
		return map[string]any{"status": "saved", "restart_required": true, "revision": voiceRevision(string(data), true)}, nil
	})
}

// VoiceNetworkStatus polls durable job state without repeatedly reading container files.
func (s *Service) VoiceNetworkStatus(ctx context.Context, user, id uuid.UUID) (VoiceNetwork, error) {
	target, err := s.Control.AccessAddonRead(ctx, s.DB, user, id)
	if err != nil {
		return VoiceNetwork{}, err
	}
	return s.voiceNetwork(ctx, Target{Target: target, q: s.DB})
}
