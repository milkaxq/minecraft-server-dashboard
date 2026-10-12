package defaultplugins

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedBundles(t *testing.T) {
	for _, version := range []string{"1.21.1", "26.3"} {
		plugins, err := Bundle(version)
		if err != nil {
			t.Fatal(err)
		}
		for _, plugin := range plugins {
			if len(plugin.SHA512) != 128 || plugin.Size <= 0 || !strings.HasPrefix(plugin.URL, "https://cdn.modrinth.com/data/") || filepath.Base(plugin.Filename) != plugin.Filename {
				t.Fatal("unsafe reviewed artifact", plugin.Name)
			}
		}
	}
	if _, err := Bundle("unknown"); err == nil {
		t.Fatal("unreviewed release accepted")
	}
}

type installFixture struct{ dir, assets, bin, source string }

func installerFixture(t *testing.T) installFixture {
	t.Helper()
	for _, tool := range []string{"bash", "jq", "sha512sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("installer tests require " + tool)
		}
	}
	root := t.TempDir()
	f := installFixture{dir: filepath.Join(root, "server"), assets: filepath.Join(root, "assets"), bin: filepath.Join(root, "bin"), source: filepath.Join(root, "download")}
	for _, dir := range []string{f.dir, f.assets, f.bin} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"install.sh", "authme.yml"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(f.assets, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	data := []byte("test-only verified plugin payload")
	sum := sha512.Sum512(data)
	plugins, _ := Bundle("26.3")
	for i := range plugins {
		plugins[i].SHA512 = hex.EncodeToString(sum[:])
		plugins[i].Size = int64(len(data))
	}
	manifest, _ := json.Marshal(map[string]any{"versions": map[string]any{"26.3": plugins}})
	if err := os.WriteFile(filepath.Join(f.assets, "manifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.source, data, 0600); err != nil {
		t.Fatal(err)
	}
	// Mock only network delivery and archive inspection. The real shell, JSON
	// selection, size/hash checks, file preservation and receipt writes run below.
	for name, script := range map[string]string{
		"curl": "#!/bin/bash\nwhile [ $# -gt 0 ]; do if [ \"$1\" = --output ]; then cp -- \"$BUNDLE_TEST_SOURCE\" \"$2\"; exit; fi; shift; done\nexit 1\n",
		"jar":  "#!/bin/bash\n[ \"${FAIL_ARCHIVE:-false}\" = false ]\n",
	} {
		if err := os.WriteFile(filepath.Join(f.bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f installFixture) run(version string, env ...string) error {
	command := exec.Command("bash", filepath.Join(f.assets, "install.sh"), version)
	command.Dir = f.dir
	command.Env = append(os.Environ(), "PATH="+f.bin+":"+os.Getenv("PATH"), "BUNDLE_TEST_SOURCE="+f.source)
	command.Env = append(command.Env, env...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

func TestInstallerSeedsSecureDefaultsAndPreservesReinstallation(t *testing.T) {
	f := installerFixture(t)
	if err := f.run("26.3"); err != nil {
		t.Fatal(err)
	}
	properties, err := os.ReadFile(filepath.Join(f.dir, "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"online-mode=false", "enforce-secure-profile=false", "white-list=true"} {
		if !strings.Contains(string(properties), value) {
			t.Fatal("missing secure property", value)
		}
	}
	if _, err := os.Stat(filepath.Join(f.dir, "eula.txt")); !os.IsNotExist(err) {
		t.Fatal("installer accepted EULA")
	}
	if _, err := os.Stat(filepath.Join(f.dir, ".blockyard-default-plugins.json")); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"server.properties", "plugins/AuthMe/config.yml", "world-progress.txt"} {
		if err := os.WriteFile(filepath.Join(f.dir, file), []byte("customer-owned configuration"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Existing matching jars are verified locally; a retry needs no downloads.
	if err := os.Remove(f.source); err != nil {
		t.Fatal(err)
	}
	if err := f.run("26.3"); err != nil {
		t.Fatal("retry downloaded or failed", err)
	}
	for _, file := range []string{"server.properties", "plugins/AuthMe/config.yml", "world-progress.txt"} {
		data, _ := os.ReadFile(filepath.Join(f.dir, file))
		if string(data) != "customer-owned configuration" {
			t.Fatal("overwrote customer file", file)
		}
	}
}

func TestInstallerFailsBeforeOfflineSetupOnBadDownload(t *testing.T) {
	for _, failure := range []string{"hash", "archive", "version"} {
		t.Run(failure, func(t *testing.T) {
			f := installerFixture(t)
			version := "26.3"
			env := []string{}
			switch failure {
			case "hash":
				if err := os.WriteFile(f.source, []byte("corrupted plugin"), 0600); err != nil {
					t.Fatal(err)
				}
			case "archive":
				env = append(env, "FAIL_ARCHIVE=true")
			case "version":
				version = "future"
			}
			if err := f.run(version, env...); err == nil {
				t.Fatal("invalid bundle installed")
			}
			for _, file := range []string{"server.properties", "plugins/AuthMe/config.yml", ".blockyard-default-plugins.json"} {
				if _, err := os.Stat(filepath.Join(f.dir, file)); !os.IsNotExist(err) {
					t.Fatal("unsafe partial setup", file)
				}
			}
		})
	}
}

func TestInstallerRejectsSymlinksConflictsAndModifiedJars(t *testing.T) {
	for _, failure := range []string{"symlink", "conflict", "modified"} {
		t.Run(failure, func(t *testing.T) {
			f := installerFixture(t)
			if failure == "symlink" {
				if err := os.Symlink(f.assets, filepath.Join(f.dir, "plugins")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(filepath.Join(f.dir, "plugins"), 0700); err != nil {
					t.Fatal(err)
				}
				name := "AuthMe-old.jar"
				if failure == "modified" {
					name = "AuthMe-6.0.1-Paper.jar"
				}
				if err := os.WriteFile(filepath.Join(f.dir, "plugins", name), []byte("keep me"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.run("26.3"); err == nil {
				t.Fatal("unsafe existing files accepted")
			}
			if _, err := os.Stat(filepath.Join(f.dir, "server.properties")); !os.IsNotExist(err) {
				t.Fatal("offline setup created after unsafe preflight")
			}
		})
	}
}
