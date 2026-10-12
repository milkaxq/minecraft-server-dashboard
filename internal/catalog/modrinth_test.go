package catalog

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
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
		hits[i] = Project{ID: "project", Title: "Example", IconURL: raw}
	}
	c := New()
	c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response(map[string]any{"hits": hits}), nil })
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
