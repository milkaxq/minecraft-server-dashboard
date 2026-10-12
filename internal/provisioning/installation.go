package provisioning

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"minecraft-hosting/internal/provisioning/defaultplugins"
	"minecraft-hosting/internal/pterodactyl"
)

// InstallationVerifier separates Client API file checks from Application API
// creation. The worker never reads a host directory or executes game processes.
type InstallationVerifier interface {
	VerifyInstallation(context.Context, string, string, string, map[string]string) error
}

type InstallationFiles interface {
	ListFiles(context.Context, string, string) ([]pterodactyl.FileEntry, error)
	ReadFile(context.Context, string, string) ([]byte, error)
}

type ReceiptVerifier struct{ Files InstallationFiles }

func requiresReceipt(kind string) bool {
	switch kind {
	case "PAPER", "SPIGOT", "PURPUR", "FABRIC", "FORGE", "NEOFORGE":
		return true
	}
	return false
}

func (v ReceiptVerifier) VerifyInstallation(ctx context.Context, identifier, kind, version string, environment map[string]string) error {
	if v.Files == nil {
		return ErrSoftware
	}
	files, err := v.Files.ListFiles(ctx, identifier, "/")
	if err != nil {
		return err
	}
	required := []string{".blockyard-install.json", "server.jar"}
	if kind == "FORGE" || kind == "NEOFORGE" {
		required[1] = "unix_args.txt"
	}
	if kind == "FABRIC" {
		required = append(required, "minecraft-server.jar")
	}
	for _, name := range required {
		found := false
		for _, file := range files {
			if file.Name == name && file.IsFile && !file.IsSymlink && file.Size > 0 {
				found = true
				break
			}
		}
		if !found {
			return ErrInstallation
		}
	}
	data, err := v.Files.ReadFile(ctx, identifier, "/.blockyard-install.json")
	if errors.Is(err, pterodactyl.ErrNotFound) {
		return ErrInstallation
	}
	if err != nil {
		return err
	}
	var proof struct {
		Software string `json:"software"`
		Version  string `json:"minecraft_version"`
		Loader   string `json:"loader_version"`
	}
	if len(data) > 4096 || json.Unmarshal(data, &proof) != nil || proof.Software != kind || proof.Version != version {
		return ErrInstallation
	}
	variable := map[string]string{"FABRIC": "LOADER_VERSION", "FORGE": "FORGE_VERSION", "NEOFORGE": "NEOFORGE_VERSION", "PURPUR": "BUILD_NUMBER"}[kind]
	if variable != "" && (environment[variable] == "" || proof.Loader != environment[variable]) {
		return ErrInstallation
	}
	if kind == "PAPER" || kind == "PURPUR" {
		return v.verifyDefaultPlugins(ctx, identifier, version, files)
	}
	return nil
}

func safeEntry(files []pterodactyl.FileEntry, name string, regular bool, size int64) bool {
	for _, file := range files {
		if file.Name == name && !file.IsSymlink && file.IsFile == regular && (!regular || file.Size > 0) && (size == 0 || file.Size == size) {
			return true
		}
	}
	return false
}

func (v ReceiptVerifier) verifyDefaultPlugins(ctx context.Context, identifier, version string, root []pterodactyl.FileEntry) error {
	expected, err := defaultplugins.Bundle(version)
	if err != nil || !safeEntry(root, ".blockyard-default-plugins.json", true, 0) || !safeEntry(root, "plugins", false, 0) || !safeEntry(root, "server.properties", true, 0) {
		return ErrInstallation
	}
	data, err := v.Files.ReadFile(ctx, identifier, "/.blockyard-default-plugins.json")
	if err != nil {
		return err
	}
	var receipt defaultplugins.Receipt
	if len(data) > 8192 || json.Unmarshal(data, &receipt) != nil || receipt.MinecraftVersion != version || receipt.Revision != defaultplugins.Revision() || !slices.Equal(receipt.Plugins, expected) {
		return ErrInstallation
	}
	plugins, err := v.Files.ListFiles(ctx, identifier, "/plugins")
	if err != nil {
		return err
	}
	for _, plugin := range expected {
		if !safeEntry(plugins, plugin.Filename, true, plugin.Size) {
			return ErrInstallation
		}
	}
	if !safeEntry(plugins, "AuthMe", false, 0) {
		return ErrInstallation
	}
	config, err := v.Files.ListFiles(ctx, identifier, "/plugins/AuthMe")
	if err != nil {
		return err
	}
	if !safeEntry(config, "config.yml", true, 0) {
		return ErrInstallation
	}
	return nil
}
