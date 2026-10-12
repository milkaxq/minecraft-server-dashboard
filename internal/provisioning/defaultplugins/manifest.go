// Package defaultplugins contains the reviewed, pinned starter plugin bundle.
package defaultplugins

import (
	"crypto/sha512"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

//go:embed manifest.json
var manifest []byte

type Plugin struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	SHA512   string `json:"sha512"`
	Size     int64  `json:"size"`
}

type Receipt struct {
	Revision         string   `json:"bundle_revision"`
	MinecraftVersion string   `json:"minecraft_version"`
	Plugins          []Plugin `json:"plugins"`
}

func Revision() string {
	sum := sha512.Sum512(manifest)
	return hex.EncodeToString(sum[:])
}

func Bundle(version string) ([]Plugin, error) {
	var data struct {
		Versions map[string][]Plugin `json:"versions"`
	}
	if err := json.Unmarshal(manifest, &data); err != nil {
		return nil, err
	}
	plugins := data.Versions[version]
	if len(plugins) != 3 {
		return nil, fmt.Errorf("default plugin bundle is not reviewed for this release")
	}
	return plugins, nil
}
