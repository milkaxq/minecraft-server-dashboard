package management

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

type StartupIssue struct {
	Code  string `json:"code"`
	Title string `json:"title"`
	Help  string `json:"help"`
}
type StartupCheck struct {
	RequestReference string         `json:"request_reference,omitempty"`
	Checked          bool           `json:"checked"`
	CheckedAt        time.Time      `json:"checked_at"`
	PowerState       string         `json:"power_state"`
	Reason           string         `json:"reason,omitempty"`
	Issues           []StartupIssue `json:"issues"`
}

func startupIssues(text string) []StartupIssue {
	lower := strings.ToLower(text)
	// Inspect the latest start only. Errors after Minecraft finishes startup are
	// runtime messages and must not be labelled as startup failures.
	start := 0
	// Initialization errors can precede "Starting minecraft server version".
	// Trim only when an earlier completed session is present in this same file.
	if done := strings.Index(lower, "for help, type"); done >= 0 {
		for _, marker := range []string{"starting minecraft server version", "modlauncher running", "loading minecraft "} {
			if i := strings.LastIndex(lower, marker); i > done && i > start {
				start = i
			}
		}
	}
	lower = lower[start:]
	if end := strings.Index(lower, "for help, type"); end >= 0 {
		lower = lower[:end]
	}
	rules := []struct {
		code, title, help string
		patterns          []string
	}{
		{"EULA_REQUIRED", "Minecraft agreement needs acceptance", "The owner must review and accept Minecraft's EULA before starting. The dashboard never accepts it automatically.", []string{"agree to the eula", "eula=false"}},
		{"MISSING_DEPENDENCY", "An addon is missing a dependency", "Check Console for the required addon names and versions. Install the matching server dependencies, then restart.", []string{"unknowndependencyexception", "missing required dependenc", "missing dependenc", "mod resolution encountered an incompatible mod set"}},
		{"JAVA_VERSION", "Java version does not match", "This software needs a different Java runtime. Ask your host to check the configured runtime; changing your computer's Java will not fix the server.", []string{"unsupportedclassversionerror", "compiled by a more recent version", "unsupported java detected"}},
		{"INCOMPATIBLE_ADDON", "An addon may be incompatible", "Check its Minecraft version and server loader. Paper plugins, Fabric mods, and Forge/NeoForge mods use different formats. See Console for the exact file.", []string{"invalidpluginexception", "invaliddescriptionexception", "unsupported api version", "not a valid mod file", "incompatible mod set"}},
		{"WORLD_VERSION", "The world may require a newer Minecraft version", "Do not downgrade the original world. Keep a backup and open it with a compatible Minecraft version, or restore a known compatible backup.", []string{"world was saved with a newer", "was saved with a newer version", "incompatible world version"}},
		{"MEMORY", "The server reported a memory problem", "Check the purchased memory allocation and addon requirements. Reduce demanding addons or ask the host about a suitable plan; the dashboard cannot raise resource limits.", []string{"outofmemoryerror", "cannot allocate memory", "unable to create native thread"}},
		{"PORT_CONFLICT", "The game port could not be opened", "Ask your host to check the server's assigned port. Do not change host networking or try to use another customer's port.", []string{"failed to bind to port", "address already in use"}},
	}
	out := []StartupIssue{}
	for _, rule := range rules {
		for _, pattern := range rule.patterns {
			if strings.Contains(lower, pattern) {
				out = append(out, StartupIssue{rule.code, rule.title, rule.help})
				break
			}
		}
	}
	if len(out) == 0 && (strings.Contains(lower, "/error]") || strings.Contains(lower, " fatal") || strings.Contains(lower, " error]:") || strings.Contains(lower, "failed to start the minecraft server")) {
		out = append(out, StartupIssue{"UNKNOWN_ERROR", "An unrecognized startup error was logged", "Open Console for the exact error. Send your host a report with the support reference; private log contents are not attached automatically."})
	}
	return out
}

func (s *Service) Startup(ctx context.Context, user, id uuid.UUID) (StartupCheck, error) {
	t, err := s.access(ctx, user, id, server.View)
	if err != nil {
		return StartupCheck{}, err
	}
	out := StartupCheck{CheckedAt: time.Now().UTC(), Issues: []StartupIssue{}}
	usage, err := s.Control.Panel.Resources(ctx, t.Identifier)
	if err != nil {
		return out, control.ErrUnavailable
	}
	out.PowerState = usage.State
	file, err := s.inspect(ctx, t, "/logs/latest.log", true)
	if err != nil || file == nil || !file.IsFile {
		out.Reason = "Latest Minecraft log is not available yet. Open Console for live startup output."
		return out, nil
	}
	if file.Size > 1<<20 {
		out.Reason = "The log is too large for the guided check. Use Console or Files to inspect it."
		return out, nil
	}
	if usage.State == "running" && usage.UptimeMillis > 0 {
		modified, e := time.Parse(time.RFC3339, file.ModifiedAt)
		if e != nil || modified.Before(time.Now().Add(-time.Duration(usage.UptimeMillis)*time.Millisecond).Add(-5*time.Second)) {
			out.Reason = "The log could not be matched to the current server start. Open Console to check live output."
			return out, nil
		}
	}
	log, err := s.read(ctx, t, "/logs/latest.log")
	if err != nil {
		out.Reason = "The log could not be checked. Open Console for startup output."
		return out, nil
	}
	if _, err = s.Control.Access(ctx, t.q, user, id, server.View); err != nil {
		return StartupCheck{}, err
	}
	out.Checked = true
	out.Issues = startupIssues(log.Content)
	return out, nil
}
