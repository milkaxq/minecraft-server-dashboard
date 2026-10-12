// Package catalog uses Modrinth's official API to resolve server-compatible releases.
package catalog

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("catalog unavailable or package cannot be installed safely")
var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type Client struct{ HTTP *http.Client }

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

type Project struct {
	ID          string `json:"project_id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ServerSide  string `json:"server_side"`
}
type File struct {
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Size     int64             `json:"size"`
	Primary  bool              `json:"primary"`
	Hashes   map[string]string `json:"hashes"`
}
type Dependency struct {
	VersionID string `json:"version_id"`
	ProjectID string `json:"project_id"`
	Type      string `json:"dependency_type"`
}
type Version struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	Name         string       `json:"name"`
	Number       string       `json:"version_number"`
	Type         string       `json:"version_type"`
	Loaders      []string     `json:"loaders"`
	Games        []string     `json:"game_versions"`
	Files        []File       `json:"files"`
	Dependencies []Dependency `json:"dependencies"`
}
type Package struct {
	VersionID string `json:"version_id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	SHA512    string `json:"sha512"`
	URL       string `json:"-"`
	Data      []byte `json:"-"`
}

func (c *Client) get(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.modrinth.com/v2/"+endpoint, nil)
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("User-Agent", "Blockyard/1.0 (https://playblockyard.com)")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 || json.Unmarshal(data, out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (c *Client) Search(ctx context.Context, loader, game, query string) ([]Project, error) {
	if len(query) > 100 {
		return nil, ErrUnavailable
	}
	facets, _ := json.Marshal([][]string{{"categories:" + loader}, {"versions:" + game}, {"server_side:required", "server_side:optional"}})
	q := url.Values{"query": {query}, "facets": {string(facets)}, "limit": {"12"}, "index": {"downloads"}}
	var response struct {
		Hits []Project `json:"hits"`
	}
	err := c.get(ctx, "search?"+q.Encode(), &response)
	if response.Hits == nil {
		response.Hits = []Project{}
	}
	return response.Hits, err
}
func Compatible(v Version, loader, game string) bool {
	return v.Type == "release" && slices.Contains(v.Loaders, loader) && slices.Contains(v.Games, game)
}
func (c *Client) latest(ctx context.Context, project, loader, game string) (Version, error) {
	if !identifier.MatchString(project) {
		return Version{}, ErrUnavailable
	}
	l, _ := json.Marshal([]string{loader})
	g, _ := json.Marshal([]string{game})
	q := url.Values{"loaders": {string(l)}, "game_versions": {string(g)}}
	var versions []Version
	if err := c.get(ctx, "project/"+project+"/version?"+q.Encode(), &versions); err != nil {
		return Version{}, err
	}
	for _, v := range versions {
		if Compatible(v, loader, game) {
			return v, nil
		}
	}
	return Version{}, ErrUnavailable
}

// Resolve includes required dependencies only. Bounds prevent dependency cycles and unbounded installs.
func (c *Client) Resolve(ctx context.Context, project, loader, game string) ([]Package, error) {
	out := []Package{}
	seen := map[string]bool{}
	projects := map[string]string{}
	var total int64
	var visit func(string, string, int) error
	visit = func(project, pin string, depth int) error {
		if depth > 8 || len(seen) >= 20 {
			return ErrUnavailable
		}
		var v Version
		var err error
		if pin != "" {
			if !identifier.MatchString(pin) {
				return ErrUnavailable
			}
			err = c.get(ctx, "version/"+pin, &v)
		} else {
			v, err = c.latest(ctx, project, loader, game)
		}
		if err != nil || !identifier.MatchString(v.ID) || !identifier.MatchString(v.ProjectID) || !Compatible(v, loader, game) {
			return ErrUnavailable
		}
		if seen[v.ID] {
			return nil
		}
		if previous := projects[v.ProjectID]; previous != "" && previous != v.ID {
			return ErrUnavailable
		}
		seen[v.ID] = true
		projects[v.ProjectID] = v.ID
		var meta struct {
			ServerSide string `json:"server_side"`
		}
		if err = c.get(ctx, "project/"+v.ProjectID, &meta); err != nil || (meta.ServerSide != "required" && meta.ServerSide != "optional") {
			return ErrUnavailable
		}
		for _, d := range v.Dependencies {
			if d.Type == "required" {
				if d.ProjectID == "" && d.VersionID == "" {
					return ErrUnavailable
				}
				if err = visit(d.ProjectID, d.VersionID, depth+1); err != nil {
					return err
				}
			}
			if d.Type == "incompatible" {
				return ErrUnavailable
			}
		}
		var selected *File
		for i := range v.Files {
			f := &v.Files[i]
			if f.Primary {
				selected = f
				break
			}
		}
		if selected == nil && len(v.Files) == 1 {
			selected = &v.Files[0]
		}
		if selected == nil {
			return ErrUnavailable
		}
		f := *selected
		if !SafeFile(f) {
			return ErrUnavailable
		}
		total += f.Size
		if total > 128<<20 {
			return ErrUnavailable
		}
		for _, p := range out {
			if p.Filename == f.Filename {
				return ErrUnavailable
			}
		}
		out = append(out, Package{VersionID: v.ID, ProjectID: v.ProjectID, Name: v.Name, Filename: f.Filename, Size: f.Size, SHA512: f.Hashes["sha512"], URL: f.URL})
		return nil
	}
	if err := visit(project, "", 0); err != nil {
		return nil, err
	}
	return out, nil
}
func SafeFile(f File) bool {
	u, e := url.Parse(f.URL)
	hash, e2 := hex.DecodeString(f.Hashes["sha512"])
	return e == nil && u.Scheme == "https" && u.Host == "cdn.modrinth.com" && u.User == nil && e2 == nil && len(hash) == 64 && f.Size > 0 && f.Size <= 64<<20 && len(f.Filename) <= 200 && !strings.ContainsAny(f.Filename, "/\\\x00") && strings.HasSuffix(strings.ToLower(f.Filename), ".jar")
}
func (c *Client) Download(ctx context.Context, p Package) ([]byte, error) {
	if !SafeFile(File{URL: p.URL, Filename: p.Filename, Size: p.Size, Hashes: map[string]string{"sha512": p.SHA512}}) {
		return nil, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, p.Size+1))
	hash := sha512.Sum512(data)
	if err != nil || int64(len(data)) != p.Size || !strings.EqualFold(hex.EncodeToString(hash[:]), p.SHA512) {
		return nil, ErrUnavailable
	}
	return data, nil
}
