package console

import "testing"

func TestPluginListConfirmsOnlyColouredCompleteCommandOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		lines     []string
		confirmed bool
		enabled   bool
	}{
		{"legacy", []string{"[12:00:00 INFO]: Plugins (2): \x1b[32mVoiceChat\x1b[0m, \x1b[31mBrokenPlugin\x1b[0m"}, true, true},
		{"modern", []string{"[12:00:00 INFO]: ℹ Server Plugins (2):", "[12:00:00 INFO]: Bukkit Plugins:", "[12:00:00 INFO]: - \x1b[38;2;85;255;85mVoiceChat\x1b[0m, \x1b[38;2;255;85;85mBrokenPlugin\x1b[0m"}, true, true},
		{"chat spoof", []string{"[12:00:00 INFO]: <Player> Plugins (1): \x1b[32mVoiceChat"}, false, false},
		{"plain", []string{"[12:00:00 INFO]: Plugins (2): VoiceChat, BrokenPlugin"}, false, false},
		{"startup", []string{"[12:00:00 INFO]: [PluginInitializerManager] Server Plugins (1): \x1b[32mVoiceChat"}, false, false},
		{"partial", []string{"Server Plugins (2):", "- \x1b[32mVoiceChat\x1b[0m"}, false, false},
		{"bad counts", []string{"Plugins (1): \x1b[32mVoiceChat\x1b[0m, \x1b[31mBrokenPlugin\x1b[0m"}, false, false},
		{"reset overrides green", []string{"Plugins (1): \x1b[32;0mVoiceChat"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := pluginListParser{expected: -1, states: map[string]bool{}}
			for _, line := range tc.lines {
				p.add(line)
			}
			if p.complete() != tc.confirmed {
				t.Fatal(p)
			}
			if tc.confirmed && p.states["VoiceChat"] != tc.enabled {
				t.Fatal(p.states)
			}
			if tc.confirmed && p.states["BrokenPlugin"] {
				t.Fatal("disabled plugin marked enabled")
			}
		})
	}
}
func TestModernPluginContinuationAndLegacyDecoration(t *testing.T) {
	p := pluginListParser{expected: -1, states: map[string]bool{}}
	for _, line := range []string{"Server Plugins (3):", "Bukkit Plugins:", "- \x1b[33m*\x1b[32mOne\x1b[0m, \x1b[31mTwo\x1b[0m,", "  \x1b[32mThree\x1b[0m"} {
		p.add(line)
	}
	if !p.complete() || !p.states["One"] || p.states["Two"] || !p.states["Three"] {
		t.Fatal(p)
	}
}
