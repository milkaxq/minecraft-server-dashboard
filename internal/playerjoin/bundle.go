package playerjoin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"minecraft-hosting/internal/catalog"
)

// Bundle downloads only the published client files, verifies their hashes, and
// returns a launcher-neutral ZIP. It never reads or bundles server directories.
func (s Service) Bundle(ctx context.Context, slug string) ([]byte, error) {
	snapshot, err := s.public(ctx, slug)
	if err != nil {
		return nil, err
	}
	if !snapshot.PackReady || len(snapshot.PackFiles) == 0 || len(snapshot.PackFiles) > 40 {
		return nil, ErrUnavailable
	}
	var total int64
	seen := map[string]bool{}
	for _, file := range snapshot.PackFiles {
		name := path.Base(file.Path)
		if file.Path != "mods/"+name || strings.HasPrefix(name, ".") || seen[file.Path] || len(file.Downloads) != 1 || file.Size <= 0 || file.Size > 64<<20 {
			return nil, ErrUnavailable
		}
		seen[file.Path] = true
		total += file.Size
		if total > 128<<20 {
			return nil, ErrUnavailable
		}
	}
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	for _, file := range snapshot.PackFiles {
		data, err := s.client().Download(ctx, catalog.Package{Filename: path.Base(file.Path), Size: file.Size, URL: file.Downloads[0], SHA512: file.Hashes["sha512"]})
		if err != nil {
			return nil, ErrUnavailable
		}
		hash := sha1.Sum(data)
		if !strings.EqualFold(hex.EncodeToString(hash[:]), file.Hashes["sha1"]) {
			return nil, ErrUnavailable
		}
		header := &zip.FileHeader{Name: file.Path, Method: zip.Store}
		header.SetMode(0644)
		entry, err := z.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err = entry.Write(data); err != nil {
			return nil, err
		}
	}
	readme := fmt.Sprintf("Blockyard player setup\n\nMinecraft: %s\nClient loader: %s %s\nServer address: %s\n\nCreate a separate installation using the Minecraft version and loader above.\nExtract this ZIP, then copy its mods folder into that installation's game directory.\nLaunch the installation and add the server address in Multiplayer.\nThis bundle does not install Minecraft or its loader automatically.\nDo not mix these JARs with files from a different loader or game version.\n", snapshot.Version, snapshot.Loader, snapshot.LoaderVersion, snapshot.Address)
	entry, err := z.Create("README.txt")
	if err != nil {
		return nil, err
	}
	if _, err = entry.Write([]byte(readme)); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(snapshot.PackFiles, "", "  ")
	if err != nil {
		return nil, err
	}
	entry, err = z.Create("client-files.json")
	if err != nil {
		return nil, err
	}
	if _, err = entry.Write(manifest); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	// Refresh authorization after downloads: expiry, revocation or changed setup
	// must not produce a new response from an obsolete invitation snapshot.
	current, err := s.public(ctx, slug)
	if err != nil {
		return nil, err
	}
	if !current.PackReady || !current.PublishedAt.Equal(snapshot.PublishedAt) {
		return nil, ErrUnavailable
	}
	return buffer.Bytes(), nil
}
