package server

import (
	"testing"

	"github.com/google/uuid"
)

func TestRBAC(t *testing.T) {
	permissions := []Permission{View, Console, Start, Restart, Stop, Kill, Files, Settings, Plugins, Worlds, Backups, Billing, Members, Delete, AcceptEULA, "unknown"}
	for _, role := range []Role{Owner, Admin, Moderator, "", "ROOT"} {
		for _, p := range permissions {
			expected := false
			if p != "unknown" {
				switch role {
				case Owner:
					expected = true
				case Admin:
					expected = p != Billing && p != Members && p != Delete && p != AcceptEULA
				case Moderator:
					expected = p == View || p == Console || p == Start || p == Restart
				}
			}
			if Allowed(role, p) != expected {
				t.Errorf("%s %s permission mismatch", role, p)
			}
		}
	}
}
func TestTransitions(t *testing.T) {
	valid := map[State][]State{PendingPayment: {PaymentConfirmed, Cancelled}, PaymentConfirmed: {Provisioning, Cancelled}, Provisioning: {Running, Failed}, Running: {Suspended, Cancelled}, Suspended: {Running, Cancelled}, Failed: {Provisioning, Cancelled}}
	states := []State{PendingPayment, PaymentConfirmed, Provisioning, Running, Suspended, Failed, Cancelled, "UNKNOWN"}
	for _, from := range states {
		for _, to := range states {
			expected := false
			for _, v := range valid[from] {
				if to == v {
					expected = true
				}
			}
			if CanTransition(from, to) != expected {
				t.Errorf("transition %s -> %s", from, to)
			}
		}
	}
}
func TestInput(t *testing.T) {
	input := CreateInput{PlanID: uuid.New(), Name: " My world ", MinecraftVersion: "1.21.1", ServerType: "PAPER"}
	if err := input.Validate(); err != nil || input.Name != "My world" {
		t.Fatal("valid input rejected")
	}
	for _, version := range []string{"latest", "../1.21", "1.21;sh", "1.21\n", ""} {
		v := input
		v.MinecraftVersion = version
		if v.Validate() == nil {
			t.Errorf("accepted version %q", version)
		}
	}
	for _, kind := range []string{"BUKKIT", "paper", ""} {
		v := input
		v.ServerType = kind
		if v.Validate() == nil {
			t.Errorf("accepted type %q", kind)
		}
	}
}

func TestSoftwareCapabilities(t *testing.T) {
	for _, kind := range []string{"VANILLA", "PAPER", "SPIGOT", "PURPUR", "FABRIC", "FORGE", "NEOFORGE"} {
		input := CreateInput{PlanID: uuid.New(), Name: "World", MinecraftVersion: "1.21.1", ServerType: kind}
		if err := input.Validate(); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if SupportsPlugins(kind) && SupportsMods(kind) {
			t.Fatal("mixed ecosystem", kind)
		}
	}
}
