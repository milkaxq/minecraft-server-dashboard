package provisioning

import (
	"context"
	"encoding/json"
	"errors"

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
	case "SPIGOT", "PURPUR", "FABRIC", "FORGE", "NEOFORGE":
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
	return nil
}
