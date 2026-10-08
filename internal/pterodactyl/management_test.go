package pterodactyl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestManagementProtocolAndSignedTransferCredentialSeparation(t *testing.T) {
	ctx := context.Background()
	backup := uuid.NewString()
	unlocks := 0
	posts := 0
	restores := 0
	wings := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Panel credential leaked to transfer host")
		}
		if r.URL.Query().Get("token") != "signed-private-token" {
			t.Error("missing signed token")
		}
		if r.Method == "POST" {
			if r.URL.Path != "/upload/file" || r.URL.Query().Get("directory") != "/plugins" {
				t.Error("upload path", r.URL.String())
			}
			file, header, err := r.FormFile("files")
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer file.Close()
			data, _ := io.ReadAll(file)
			if header.Filename != "Test.jar" || string(data) != "binary\x00payload" {
				t.Error("multipart upload changed")
			}
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, "download-payload")
	}))
	defer wings.Close()
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-client-key" {
			t.Error("wrong Client credential")
		}
		respond := func(v any) { json.NewEncoder(w).Encode(v) }
		attr := func(v any) { respond(map[string]any{"attributes": v}) }
		switch {
		case r.URL.Path == "/api/client/servers/abcdef12":
			attr(map[string]any{"identifier": "abcdef12", "status": nil, "feature_limits": map[string]int{"backups": 2}})
		case strings.HasSuffix(r.URL.Path, "/files/contents"):
			if r.URL.Query().Get("file") != "/server.properties" {
				t.Error("wrong file query")
			}
			io.WriteString(w, "motd=Hi\n")
		case strings.HasSuffix(r.URL.Path, "/files/write"):
			data, _ := io.ReadAll(r.Body)
			if r.URL.Query().Get("file") != "/server.properties" || string(data) != "motd=Changed\n" {
				t.Error("raw write changed")
			}
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/files/rename"):
			if r.Method != "PUT" {
				t.Error("rename method")
			}
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			if in["root"] != "/plugins" {
				t.Error("rename root")
			}
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/files/list"):
			respond(map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"name": "server.properties", "is_file": true, "size": 8, "is_symlink": false}}}})
		case strings.HasSuffix(r.URL.Path, "/files/upload"):
			attr(map[string]string{"url": wings.URL + "/upload/file?token=signed-private-token"})
		case strings.HasSuffix(r.URL.Path, "/files/download"):
			attr(map[string]string{"url": wings.URL + "/download/file?token=signed-private-token"})
		case strings.HasSuffix(r.URL.Path, "/backups") && r.Method == "GET":
			respond(map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"uuid": backup, "is_locked": false, "is_successful": true, "completed_at": "2026-10-07T10:00:00Z"}}}, "meta": map[string]any{"pagination": map[string]int{"total_pages": 1}}})
		case strings.HasSuffix(r.URL.Path, "/backups") && r.Method == "POST":
			posts++
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			if in["is_locked"] != true {
				t.Error("new backup not locked")
			}
			attr(map[string]any{"uuid": uuid.NewString(), "name": "New"})
		case strings.HasSuffix(r.URL.Path, "/restore"):
			restores++
			var in map[string]bool
			json.NewDecoder(r.Body).Decode(&in)
			if !in["truncate"] {
				t.Error("restore does not replace files")
			}
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/lock"):
			unlocks++
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/backups/"+backup) && r.Method == "DELETE":
			w.WriteHeader(204)
		default:
			t.Error("unexpected route", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer panel.Close()
	client, err := NewServerClient(panel.URL, "private-client-key", true)
	if err != nil {
		t.Fatal(err)
	}
	client.ConfigureTransfers([]string{wings.URL})
	if err = client.Ready(ctx, "abcdef12"); err != nil {
		t.Fatal(err)
	}
	list, err := client.ListFiles(ctx, "abcdef12", "/")
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	data, err := client.ReadFile(ctx, "abcdef12", "/server.properties")
	if err != nil || string(data) != "motd=Hi\n" {
		t.Fatal(string(data), err)
	}
	if err = client.WriteFile(ctx, "abcdef12", "/server.properties", []byte("motd=Changed\n")); err != nil {
		t.Fatal(err)
	}
	if err = client.RenameFile(ctx, "abcdef12", "/plugins", "Old.jar", "New.jar"); err != nil {
		t.Fatal(err)
	}
	if err = client.UploadFile(ctx, "abcdef12", "/plugins", "Test.jar", []byte("binary\x00payload")); err != nil {
		t.Fatal(err)
	}
	reader, err := client.DownloadFile(ctx, "abcdef12", "/server.properties")
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(reader)
	reader.Close()
	if string(data) != "download-payload" {
		t.Fatal(string(data))
	}
	if _, err = client.CreateBackup(ctx, "abcdef12", "New"); err != nil {
		t.Fatal(err)
	}
	if err = client.RestoreBackup(ctx, "abcdef12", backup); err != nil {
		t.Fatal(err)
	}
	if err = client.DeleteBackup(ctx, "abcdef12", backup); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || restores != 1 || unlocks != 0 {
		t.Fatal("backup protocol", posts, restores, unlocks)
	}
}
func TestTransfersDenyRedirectsAndUnlistedOrigins(t *testing.T) {
	ctx := context.Background()
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	client, _ := NewServerClient(redirect.URL, "key", true)
	client.ConfigureTransfers([]string{redirect.URL})
	if _, err := client.download(ctx, redirect.URL+"/download"); err == nil {
		t.Fatal("redirect followed")
	}
	if targetCalls != 0 {
		t.Fatal("redirect contacted foreign host")
	}
	for _, url := range []string{target.URL, "http://user:pass@localhost/file", "file:///etc/passwd", redirect.URL + "/#fragment"} {
		if _, err := client.transferURL(url); err == nil {
			t.Error("accepted", url)
		}
	}
	if err := client.RestoreBackup(ctx, "../bad", "../bad"); err == nil {
		t.Fatal("unsafe ID accepted")
	}
}
