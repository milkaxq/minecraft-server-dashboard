// Package playerjoin publishes owner-controlled, read-only invitations for players.
package playerjoin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/server"
)

var ErrUnavailable = errors.New("player invitation unavailable")
var slugPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

type Service struct {
	DB         *pgxpool.Pool
	Management *management.Service
	Catalog    *catalog.Client
	HTTP       *http.Client
}
type Addon struct {
	Title    string `json:"title"`
	Project  string `json:"project,omitempty"`
	Required bool   `json:"required"`
}
type PackFile struct {
	Path        string            `json:"path"`
	Hashes      map[string]string `json:"hashes"`
	Downloads   []string          `json:"downloads"`
	Size        int64             `json:"fileSize"`
	Environment map[string]string `json:"env"`
}
type Snapshot struct {
	Name          string     `json:"name"`
	Address       string     `json:"address"`
	Version       string     `json:"minecraft_version"`
	Loader        string     `json:"loader,omitempty"`
	LoaderVersion string     `json:"loader_version,omitempty"`
	VerifiedLogin bool       `json:"verified_login"`
	Whitelist     bool       `json:"whitelist"`
	Addons        []Addon    `json:"addons"`
	PackReady     bool       `json:"pack_ready"`
	PackReason    string     `json:"pack_reason,omitempty"`
	Notes         []string   `json:"notes,omitempty"`
	Files         []PackFile `json:"-"`
	PublishedAt   time.Time  `json:"published_at"`
}
type storedSnapshot struct {
	Snapshot
	PackFiles []PackFile `json:"pack_files"`
}
type Link struct {
	Slug      string    `json:"slug"`
	ExpiresAt time.Time `json:"expires_at"`
	Snapshot  Snapshot  `json:"setup"`
}

func (s Service) client() *catalog.Client {
	if s.Catalog != nil {
		return s.Catalog
	}
	return catalog.New()
}

func (s Service) fabricLoader(ctx context.Context, game string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://meta.fabricmc.net/v2/versions/loader/"+url.PathEscape(game), nil)
	if err != nil {
		return "", err
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(req)
	if err != nil {
		return "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", ErrUnavailable
	}
	var choices []struct {
		Loader struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		} `json:"loader"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&choices) != nil {
		return "", ErrUnavailable
	}
	for _, choice := range choices {
		if choice.Loader.Stable && regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(choice.Loader.Version) {
			return choice.Loader.Version, nil
		}
	}
	return "", ErrUnavailable
}

func (s Service) snapshot(ctx context.Context, actor, id uuid.UUID) (Snapshot, error) {
	domain := server.Service{DB: s.DB}
	v, err := domain.Get(ctx, actor, id)
	if err != nil {
		return Snapshot{}, err
	}
	if v.Hostname == nil || v.Port == nil {
		return Snapshot{}, ErrUnavailable
	}
	requirements, err := s.Management.VerifiedClientRequirements(ctx, actor, id)
	if err != nil {
		return Snapshot{}, err
	}
	settings, err := s.Management.Settings(ctx, actor, id)
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{Name: v.Name, Address: net.JoinHostPort(*v.Hostname, strconv.Itoa(*v.Port)), Version: v.MinecraftVersion, VerifiedLogin: settings.OnlineMode, Whitelist: settings.Whitelist, Addons: []Addon{}, PublishedAt: time.Now().UTC(), Files: []PackFile{}}
	roots := []management.ClientAddon{}
	unverified := []string{}
	for _, addon := range requirements.Addons {
		if addon.ConfigurationID == management.VoiceAddon {
			if addon.VoiceRequired == nil {
				out.PackReason = "Simple Voice Chat settings could not be read. Refresh the setup after checking its configuration."
				return out, nil
			}
			out.Addons = append(out.Addons, Addon{Title: "Simple Voice Chat", Project: addon.ProjectID, Required: addon.VoiceRequired != nil && *addon.VoiceRequired})
			roots = append(roots, addon)
			continue
		}
		switch addon.ClientSide {
		case "unsupported":
			continue
		case "required", "optional":
			out.Addons = append(out.Addons, Addon{Title: addon.Title, Project: addon.ProjectID, Required: addon.ClientSide == "required"})
			roots = append(roots, addon)
		default:
			if addon.ProjectID == "" {
				unverified = append(unverified, addon.Title)
			} else {
				out.Notes = append(out.Notes, addon.Title+": Modrinth does not specify a client requirement. It stays installed on the server and is not added to the player download.")
			}
		}
	}
	if len(unverified) > 0 {
		out.PackReason = "These uploaded addons have no catalog record: " + strings.Join(unverified, ", ") + ". Ask your host to check their player requirements."
		return out, nil
	}
	if len(roots) == 0 {
		return out, nil
	}
	out.Loader = strings.ToLower(v.ServerType)
	out.LoaderVersion = v.LoaderVersion
	if server.SupportsPlugins(v.ServerType) {
		out.Loader = "fabric"
		out.LoaderVersion, err = s.fabricLoader(ctx, v.MinecraftVersion)
		if err != nil {
			out.PackReason = "A matching client loader could not be verified. Your host can retry publishing."
			return out, nil
		}
	}
	if out.LoaderVersion == "" || !server.SupportsMods(strings.ToUpper(out.Loader)) {
		out.PackReason = "Your host needs to verify the client loader version."
		return out, nil
	}
	seen := map[string]string{}
	filenames := map[string]bool{}
	var total int64
	for _, root := range roots {
		files, e := s.client().ResolveClient(ctx, root.ProjectID, root.VersionID, out.Loader, out.Version)
		if e != nil {
			required := root.ClientSide == "required" || root.VoiceRequired != nil && *root.VoiceRequired
			if !required {
				out.Notes = append(out.Notes, root.Title+": no matching client download was found. This optional addon stays on the server and is omitted from the player pack.")
				for i, addon := range out.Addons {
					if addon.Project == root.ProjectID {
						out.Addons = append(out.Addons[:i], out.Addons[i+1:]...)
						break
					}
				}
				continue
			}
			out.Files = nil
			out.PackReason = root.Title + ": a required matching client release or dependency could not be verified. Ask your host to review this addon."
			if errors.Is(e, catalog.ErrPreviewClient) {
				out.PackReason = root.Title + ": the matching client build for Minecraft " + out.Version + " is marked beta on Modrinth. Automatic player packs currently include stable builds only."
			}
			return out, nil
		}
		for _, file := range files {
			if previous, ok := seen[file.ProjectID]; ok {
				if previous != file.VersionID {
					out.Files = nil
					out.PackReason = "The client addons need conflicting dependency versions. Your host needs to resolve them."
					return out, nil
				}
				continue
			}
			hash, e := hex.DecodeString(file.SHA1)
			download, e2 := url.Parse(file.URL)
			if e != nil || len(hash) != 20 || e2 != nil || download.RawQuery != "" || download.Fragment != "" || path.Base(file.Filename) != file.Filename || strings.HasPrefix(file.Filename, ".") || filenames[file.Filename] {
				out.Files = nil
				out.PackReason = "Some client files could not be verified."
				return out, nil
			}
			total += file.Size
			if len(out.Files) >= 40 || total > 128<<20 {
				out.Files = nil
				out.PackReason = "This client pack exceeds the automatic setup limit."
				return out, nil
			}
			seen[file.ProjectID] = file.VersionID
			filenames[file.Filename] = true
			out.Files = append(out.Files, PackFile{Path: "mods/" + file.Filename, Hashes: map[string]string{"sha1": file.SHA1, "sha512": file.SHA512}, Downloads: []string{file.URL}, Size: file.Size, Environment: map[string]string{"client": "required", "server": "unsupported"}})
		}
	}
	out.PackReady = len(out.Files) > 0
	return out, nil
}

func (s Service) Publish(ctx context.Context, actor, id uuid.UUID, rid string) (Link, error) {
	domain := server.Service{DB: s.DB}
	if err := domain.Authorize(ctx, actor, id, server.Members); err != nil {
		return Link{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return Link{}, err
	}
	defer conn.Release()
	var locked bool
	lockKey := "server:" + id.String()
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", lockKey).Scan(&locked); err != nil {
		return Link{}, err
	}
	if !locked {
		return Link{}, control.ErrBusy
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		if e := conn.QueryRow(clean, "SELECT pg_advisory_unlock(hashtextextended($1,0))", lockKey).Scan(&unlocked); e != nil || !unlocked {
			_ = conn.Conn().Close(clean)
		}
	}()
	snapshot, err := s.snapshot(ctx, actor, id)
	if err != nil {
		return Link{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Link{}, err
	}
	defer tx.Rollback(context.Background())
	var owner uuid.UUID
	if err = tx.QueryRow(ctx, "SELECT owner_id FROM servers WHERE id=$1 AND status='RUNNING' FOR UPDATE", id).Scan(&owner); err != nil || owner != actor {
		return Link{}, server.ErrNotFound
	}
	var existing string
	err = tx.QueryRow(ctx, "SELECT slug FROM player_invites WHERE server_id=$1 AND expires_at>now()", id).Scan(&existing)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Link{}, err
	}
	if existing == "" {
		raw := make([]byte, 32)
		if _, err = rand.Read(raw); err != nil {
			return Link{}, err
		}
		existing = base64.RawURLEncoding.EncodeToString(raw)
	}
	data, err := json.Marshal(storedSnapshot{Snapshot: snapshot, PackFiles: snapshot.Files})
	if err != nil {
		return Link{}, err
	}
	out := Link{Slug: existing, ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour), Snapshot: snapshot}
	_, err = tx.Exec(ctx, `INSERT INTO player_invites(server_id,slug,created_by,snapshot,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(server_id) DO UPDATE SET slug=excluded.slug,snapshot=excluded.snapshot,expires_at=excluded.expires_at,created_by=excluded.created_by`, id, out.Slug, actor, data, out.ExpiresAt)
	if err != nil {
		return Link{}, err
	}
	if err = audit.Record(ctx, tx, actor, id, "players.invite.published", rid, map[string]any{"pack_ready": snapshot.PackReady, "addons": len(snapshot.Addons)}); err != nil {
		return Link{}, err
	}
	return out, tx.Commit(ctx)
}
func (s Service) Get(ctx context.Context, actor, id uuid.UUID) (*Link, error) {
	if err := (server.Service{DB: s.DB}).Authorize(ctx, actor, id, server.Members); err != nil {
		return nil, err
	}
	var out Link
	var data []byte
	err := s.DB.QueryRow(ctx, "SELECT slug,expires_at,snapshot FROM player_invites WHERE server_id=$1 AND expires_at>now()", id).Scan(&out.Slug, &out.ExpiresAt, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot storedSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	out.Snapshot = snapshot.Snapshot
	return &out, nil
}
func (s Service) Revoke(ctx context.Context, actor, id uuid.UUID, rid string) error {
	if err := (server.Service{DB: s.DB}).Authorize(ctx, actor, id, server.Members); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, "DELETE FROM player_invites WHERE server_id=$1 AND EXISTS(SELECT 1 FROM servers WHERE id=$1 AND owner_id=$2)", id, actor)
	if err != nil {
		return err
	}
	if err = audit.Record(ctx, tx, actor, id, "players.invite.revoked", rid, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s Service) public(ctx context.Context, slug string) (storedSnapshot, error) {
	if !slugPattern.MatchString(slug) {
		return storedSnapshot{}, server.ErrNotFound
	}
	var data []byte
	err := s.DB.QueryRow(ctx, `SELECT i.snapshot FROM player_invites i JOIN servers s ON s.id=i.server_id WHERE i.slug=$1 AND i.expires_at>now() AND s.status='RUNNING'`, slug).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedSnapshot{}, server.ErrNotFound
	}
	if err != nil {
		return storedSnapshot{}, err
	}
	var out storedSnapshot
	err = json.Unmarshal(data, &out)
	return out, err
}
func (s Service) Public(ctx context.Context, slug string) (Snapshot, error) {
	out, err := s.public(ctx, slug)
	return out.Snapshot, err
}
func (s Service) Pack(ctx context.Context, slug string) ([]byte, error) {
	out, err := s.public(ctx, slug)
	if err != nil {
		return nil, err
	}
	if !out.PackReady {
		return nil, ErrUnavailable
	}
	loaderKey := map[string]string{"fabric": "fabric-loader", "forge": "forge", "neoforge": "neoforge", "quilt": "quilt-loader"}[out.Loader]
	if loaderKey == "" || out.LoaderVersion == "" {
		return nil, ErrUnavailable
	}
	version := sha256.Sum256([]byte(out.PublishedAt.Format(time.RFC3339Nano)))
	manifest := map[string]any{"formatVersion": 1, "game": "minecraft", "versionId": hex.EncodeToString(version[:8]), "name": out.Name + " · Player setup", "summary": "Client mods verified against the published server setup. Ask the host about additional pack configuration.", "dependencies": map[string]string{"minecraft": out.Version, loaderKey: out.LoaderVersion}, "files": out.PackFiles}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	f, err := z.Create("modrinth.index.json")
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(data); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
