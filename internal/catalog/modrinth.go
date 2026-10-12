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
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("catalog unavailable or package cannot be installed safely")
var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type Client struct{ HTTP *http.Client }

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

type Project struct {
	ID             string `json:"project_id"`
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	ServerSide     string `json:"server_side"`
	IconURL        string `json:"icon_url,omitempty"`
	Downloads      int64  `json:"downloads"`
	ReleaseVersion string `json:"release_version,omitempty"`
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

type SearchOptions struct {
	Query    string
	Sort     string
	Category string
	Offset   int
}

var ErrInvalidSearch = errors.New("invalid catalog search options")
var errNoCompatibleRelease = errors.New("no compatible stable release")

func (o SearchOptions) Validate() error {
	if len(o.Query) > 100 || o.Offset < 0 || o.Offset > 10000 {
		return ErrInvalidSearch
	}
	switch o.Sort {
	case "", "downloads", "relevance", "newest", "updated":
	default:
		return ErrInvalidSearch
	}
	switch o.Category {
	case "", "adventure", "cursed", "decoration", "economy", "equipment", "food", "game-mechanics", "library", "magic", "management", "minigame", "mobs", "optimization", "social", "storage", "technology", "transportation", "utility", "worldgen":
	default:
		return ErrInvalidSearch
	}
	return nil
}

type SearchPage struct {
	Projects         []Project `json:"projects"`
	HasMore          bool      `json:"has_more"`
	NextOffset       int       `json:"next_offset"`
	Software         string    `json:"software"`
	MinecraftVersion string    `json:"minecraft_version"`
}

func (c *Client) Search(ctx context.Context, loader, game, query string) ([]Project, error) {
	page, err := c.SearchPage(ctx, loader, game, SearchOptions{Query: query})
	return page.Projects, err
}

func (c *Client) SearchPage(ctx context.Context, loader, game string, options SearchOptions) (SearchPage, error) {
	out := SearchPage{Projects: []Project{}, MinecraftVersion: game, Software: strings.ToUpper(loader)}
	if err := options.Validate(); err != nil {
		return out, err
	}
	if options.Sort == "" {
		options.Sort = "downloads"
	}
	filters := [][]string{{"categories:" + loader}, {"versions:" + game}, {"server_side:required", "server_side:optional"}, {"project_type:mod", "project_type:plugin"}}
	if options.Category != "" {
		filters = append(filters, []string{"categories:" + options.Category})
	}
	facets, _ := json.Marshal(filters)
	q := url.Values{"query": {options.Query}, "facets": {string(facets)}, "limit": {"12"}, "index": {options.Sort}, "offset": {strconv.Itoa(options.Offset)}}
	var response struct {
		Hits  []Project `json:"hits"`
		Total int       `json:"total_hits"`
	}
	if err := c.get(ctx, "search?"+q.Encode(), &response); err != nil {
		return out, err
	}
	if len(response.Hits) > 12 {
		return out, ErrUnavailable
	}
	// Search facets describe a whole project, not one release. Check both
	// constraints on the same stable release, preserving upstream sort order.
	versions := make([]Version, len(response.Hits))
	failures := make([]error, len(response.Hits))
	slots := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for i, project := range response.Hits {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if project.ServerSide != "required" && project.ServerSide != "optional" {
				failures[i] = errNoCompatibleRelease
				return
			}
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				failures[i] = ctx.Err()
				return
			}
			defer func() { <-slots }()
			versions[i], failures[i] = c.latest(ctx, project.ID, loader, game)
			if failures[i] == nil && (!identifier.MatchString(versions[i].ID) || versions[i].ProjectID != project.ID) {
				failures[i] = ErrUnavailable
			}
		}()
	}
	wait.Wait()
	for i, project := range response.Hits {
		if errors.Is(failures[i], errNoCompatibleRelease) {
			continue
		}
		if failures[i] != nil {
			return out, ErrUnavailable
		}
		if !SafeIconURL(project.IconURL) {
			project.IconURL = ""
		}
		project.ReleaseVersion = versions[i].Number
		out.Projects = append(out.Projects, project)
	}
	out.NextOffset = options.Offset + len(response.Hits)
	out.HasMore = len(response.Hits) > 0 && out.NextOffset < response.Total && out.NextOffset <= 10000
	return out, nil
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
	return Version{}, errNoCompatibleRelease
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

// SafeIconURL limits browser image requests to Modrinth's image CDN.
func SafeIconURL(raw string) bool {
	if len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "cdn.modrinth.com" || u.User != nil || u.Fragment != "" {
		return false
	}
	path := strings.ToLower(u.Path)
	return strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".webp") || strings.HasSuffix(path, ".gif")
}
