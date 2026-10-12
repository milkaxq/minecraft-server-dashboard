package catalog

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(data any) *http.Response {
	b, _ := json.Marshal(data)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}
}
func TestFileEgressAndHashes(t *testing.T) {
	data := []byte("jar bytes")
	h := sha512.Sum512(data)
	good := File{URL: "https://cdn.modrinth.com/data/p/file.jar", Filename: "file.jar", Size: int64(len(data)), Hashes: map[string]string{"sha512": hex.EncodeToString(h[:])}}
	if !SafeFile(good) {
		t.Fatal("valid CDN file rejected")
	}
	for _, u := range []string{"http://cdn.modrinth.com/file.jar", "https://cdn.modrinth.com.evil.test/file.jar", "https://127.0.0.1/file.jar", "https://cdn.modrinth.com:443/file.jar", "https://user@cdn.modrinth.com/file.jar"} {
		bad := good
		bad.URL = u
		if SafeFile(bad) {
			t.Fatal("unsafe URL accepted", u)
		}
	}
	for _, n := range []string{"../file.jar", "folder/file.jar", "file.zip", "..\\file.jar"} {
		bad := good
		bad.Filename = n
		if SafeFile(bad) {
			t.Fatal("unsafe filename accepted", n)
		}
	}
	c := New()
	c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("wrong jar"))}, nil
	})
	if _, err := c.Download(context.Background(), Package{URL: good.URL, Filename: good.Filename, Size: good.Size, SHA512: good.Hashes["sha512"]}); err == nil {
		t.Fatal("corrupt download accepted")
	}
}
func TestResolveStableDependenciesAndCompatibility(t *testing.T) {
	h := sha512.Sum512([]byte("x"))
	file := func(n string) []File {
		return []File{{URL: "https://cdn.modrinth.com/" + n + ".jar", Filename: n + ".jar", Size: 1, Primary: true, Hashes: map[string]string{"sha512": hex.EncodeToString(h[:])}}}
	}
	root := Version{ID: "stable", ProjectID: "root", Type: "release", Loaders: []string{"forge"}, Games: []string{"1.21.1"}, Files: file("root"), Dependencies: []Dependency{{ProjectID: "dep", VersionID: "dependency", Type: "required"}, {ProjectID: "optional", Type: "optional"}}}
	dep := Version{ID: "dependency", ProjectID: "dep", Type: "release", Loaders: []string{"forge"}, Games: []string{"1.21.1"}, Files: file("dep")}
	c := New()
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v2/project/root/version":
			alpha := root
			alpha.Type = "alpha"
			return response([]Version{alpha, root}), nil
		case "/v2/version/dependency":
			return response(dep), nil
		case "/v2/project/root", "/v2/project/dep":
			return response(map[string]string{"server_side": "required"}), nil
		default:
			t.Fatal("unexpected request", r.URL)
			return nil, nil
		}
	})
	packages, err := c.Resolve(context.Background(), "root", "forge", "1.21.1")
	if err != nil || len(packages) != 2 || packages[0].Filename != "dep.jar" {
		t.Fatal(packages, err)
	}
	dep.Loaders = []string{"fabric"}
	if _, err = c.Resolve(context.Background(), "root", "forge", "1.21.1"); err == nil {
		t.Fatal("incompatible dependency accepted")
	}
	if Compatible(root, "paper", "1.21.1") || Compatible(root, "forge", "26.3") {
		t.Fatal("compatibility mismatch accepted")
	}
}

func TestSearchExcludesPacksAndClientOnlyProjects(t *testing.T) {
	c := New()
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		var facets [][]string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("facets")), &facets); err != nil {
			t.Fatal(err)
		}
		expected := [][]string{{"categories:forge"}, {"versions:1.21.1"}, {"server_side:required", "server_side:optional"}, {"project_type:mod", "project_type:plugin"}}
		got, _ := json.Marshal(facets)
		want, _ := json.Marshal(expected)
		if string(got) != string(want) {
			t.Fatal("incorrect catalog filters", string(got))
		}
		return response(map[string]any{"hits": []Project{}}), nil
	})
	if _, err := c.Search(context.Background(), "forge", "1.21.1", "test"); err != nil {
		t.Fatal(err)
	}
}

func TestSearchSanitizesProjectIcons(t *testing.T) {
	urls := []string{"https://cdn.modrinth.com/data/icon.png", "https://cdn.modrinth.com/icon.webp", "http://cdn.modrinth.com/icon.png", "https://cdn.modrinth.com.evil.test/icon.png", "https://127.0.0.1/icon.png", "https://user@cdn.modrinth.com/icon.png", "https://cdn.modrinth.com:443/icon.png", "https://cdn.modrinth.com/icon.svg", "data:image/png;base64,test", "", "https://cdn.modrinth.com/icon.png#fragment"}
	hits := make([]Project, len(urls))
	for i, raw := range urls {
		hits[i] = Project{ID: "project", Title: "Example", IconURL: raw, ServerSide: "required"}
	}
	c := New()
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/search" {
			return response(map[string]any{"hits": hits}), nil
		}
		return response([]Version{{ID: "stable", ProjectID: "project", Type: "release", Loaders: []string{"paper"}, Games: []string{"1.21.1"}}}), nil
	})
	projects, err := c.Search(context.Background(), "paper", "1.21.1", "")
	if err != nil || len(projects) != len(urls) {
		t.Fatalf("search failed: %v", err)
	}
	for i, p := range projects {
		expected := ""
		if i < 2 {
			expected = urls[i]
		}
		if p.IconURL != expected || p.Title != "Example" {
			t.Fatalf("unsafe or missing icon at %d: %q", i, p.IconURL)
		}
	}
}

// A project's software and game tags can refer to different releases.
func TestSearchRequiresOneStableReleaseMatchingSoftwareAndGame(t *testing.T) {
	c := New()
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v2/search":
			return response(map[string]any{"hits": []Project{{ID: "wrongGame", ServerSide: "required"}, {ID: "wrongLoader", ServerSide: "required"}, {ID: "alpha", ServerSide: "required"}, {ID: "valid", ServerSide: "required"}}, "total_hits": 4}), nil
		case "/v2/project/wrongGame/version":
			return response([]Version{{ID: "other", ProjectID: "wrongGame", Type: "release", Loaders: []string{"paper"}, Games: []string{"1.20.1"}}}), nil
		case "/v2/project/wrongLoader/version":
			return response([]Version{{ID: "other", ProjectID: "wrongLoader", Type: "release", Loaders: []string{"fabric"}, Games: []string{"1.21.1"}}}), nil
		case "/v2/project/alpha/version":
			return response([]Version{{ID: "alpha", ProjectID: "alpha", Type: "alpha", Loaders: []string{"paper"}, Games: []string{"1.21.1"}}}), nil
		case "/v2/project/valid/version":
			if r.URL.Query().Get("loaders") != `["paper"]` || r.URL.Query().Get("game_versions") != `["1.21.1"]` {
				t.Errorf("lost compatibility filters: %s", r.URL)
			}
			return response([]Version{{ID: "stable", ProjectID: "valid", Type: "release", Loaders: []string{"paper"}, Games: []string{"1.21.1"}}}), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
	})
	projects, err := c.Search(context.Background(), "paper", "1.21.1", "")
	if err != nil || len(projects) != 1 || projects[0].ID != "valid" {
		t.Fatalf("incompatible projects shown: %+v, %v", projects, err)
	}
}

func TestSearchOptionsKeepCompatibilityAndUseUpstreamOrdering(t *testing.T) {
	for _, sort := range []string{"", "downloads", "relevance", "newest", "updated"} {
		t.Run(sort, func(t *testing.T) {
			c := New()
			c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/search" {
					expectedSort := sort
					if expectedSort == "" {
						expectedSort = "downloads"
					}
					if r.URL.Query().Get("index") != expectedSort || r.URL.Query().Get("offset") != "12" || r.URL.Query().Get("query") != "permissions" {
						t.Errorf("incorrect ordering/pagination: %s", r.URL)
					}
					want := `[["categories:paper"],["versions:1.21.1"],["server_side:required","server_side:optional"],["project_type:mod","project_type:plugin"],["categories:utility"]]`
					if r.URL.Query().Get("facets") != want {
						t.Errorf("compatibility filters lost: %s", r.URL.Query().Get("facets"))
					}
					return response(map[string]any{"hits": []Project{{ID: "first", ServerSide: "required"}, {ID: "second", ServerSide: "optional"}}, "total_hits": 30}), nil
				}
				id := strings.Split(r.URL.Path, "/")[3]
				return response([]Version{{ID: "stable", ProjectID: id, Type: "release", Number: "1.0", Loaders: []string{"paper"}, Games: []string{"1.21.1"}}}), nil
			})
			page, err := c.SearchPage(context.Background(), "paper", "1.21.1", SearchOptions{Query: "permissions", Sort: sort, Category: "utility", Offset: 12})
			if err != nil || len(page.Projects) != 2 || page.Projects[0].ID != "first" || page.Projects[1].ID != "second" || !page.HasMore || page.NextOffset != 14 {
				t.Fatalf("wrong page: %+v %v", page, err)
			}
		})
	}
}
func TestInvalidSearchDoesNotContactProvider(t *testing.T) {
	for _, options := range []SearchOptions{{Sort: "unknown"}, {Category: "paper"}, {Category: "utility OR versions:*"}, {Offset: -1}, {Offset: 10001}, {Query: strings.Repeat("x", 101)}} {
		c := New()
		c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
			t.Error("invalid search reached provider")
			return nil, fmt.Errorf("unexpected request")
		})
		if _, err := c.SearchPage(context.Background(), "paper", "1.21.1", options); !errors.Is(err, ErrInvalidSearch) {
			t.Fatalf("invalid options accepted: %+v %v", options, err)
		}
	}
}
func TestCompatibilityCheckFailureIsNotReportedAsEmptyCatalog(t *testing.T) {
	c := New()
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/search" {
			return response(map[string]any{"hits": []Project{{ID: "project", ServerSide: "required"}}, "total_hits": 1}), nil
		}
		return nil, fmt.Errorf("provider unavailable")
	})
	if _, err := c.Search(context.Background(), "paper", "1.21.1", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal("upstream failure hidden", err)
	}
}
func TestResolveNoMatchingReleaseReturnsPublicUnavailableError(t *testing.T) {
	c := New()
	c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response([]Version{}), nil })
	if _, err := c.Resolve(context.Background(), "project", "paper", "1.21.1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("incompatible release error changed: %v", err)
	}
}
