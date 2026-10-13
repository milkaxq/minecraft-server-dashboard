package playerjoin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type panel struct {
	pterodactyl.ManagementClient
	*pterodactyl.Mock
	files map[string][]byte
	calls int
}

func (p *panel) Ready(context.Context, string) error { p.calls++; return nil }
func (p *panel) WriteFile(_ context.Context, _ string, file string, data []byte) error {
	p.files[file] = bytes.Clone(data)
	return nil
}
func (p *panel) ReadFile(_ context.Context, _ string, file string) ([]byte, error) {
	p.calls++
	if data, ok := p.files[file]; ok {
		return data, nil
	}
	return nil, pterodactyl.ErrNotFound
}
func (p *panel) DownloadFile(ctx context.Context, id, file string) (io.ReadCloser, error) {
	data, err := p.ReadFile(ctx, id, file)
	return io.NopCloser(bytes.NewReader(data)), err
}
func (p *panel) ListFiles(_ context.Context, _ string, dir string) ([]pterodactyl.FileEntry, error) {
	p.calls++
	out := []pterodactyl.FileEntry{}
	seen := map[string]bool{}
	for file, data := range p.files {
		if path.Dir(file) == dir {
			out = append(out, pterodactyl.FileEntry{Name: path.Base(file), IsFile: true, Size: int64(len(data))})
			continue
		}
		if strings.HasPrefix(file, dir+"/") || dir == "/" {
			relative := strings.TrimPrefix(file, strings.TrimSuffix(dir, "/")+"/")
			name := strings.Split(relative, "/")[0]
			if strings.Contains(relative, "/") && !seen[name] {
				seen[name] = true
				out = append(out, pterodactyl.FileEntry{Name: name})
			}
		}
	}
	return out, nil
}

func TestInvitationOwnershipPrivacyPackAndRevocation(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, other, id := uuid.New(), uuid.New(), uuid.New()
	for _, user := range []uuid.UUID{owner, other} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'test','Player',now())", user, user.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	p := &panel{Mock: pterodactyl.NewMock(), files: map[string][]byte{"/server.properties": []byte("online-mode=false\nwhite-list=false\n"), "/plugins/voice.jar": []byte("verified bytes"), "/plugins/voicechat/voicechat-server.properties": []byte("force_voice_chat=true\n")}}
	user, err := p.CreateUser(ctx, pterodactyl.UserRequest{ExternalID: owner, Email: "owner@example.com", Name: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := p.CreateServer(ctx, pterodactyl.ServerRequest{ExternalID: id, Name: "Friends", UserID: user.ID, NodeID: 1, AllocationID: 1, EggID: 1, Image: "java", Startup: "minecraft", Limits: pterodactyl.Limits{RAMMB: 2048, DiskMB: 10240, CPU: 100, Backups: 2}})
	if err != nil {
		t.Fatal(err)
	}
	node := uuid.New()
	if _, err = db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Node',400,8192,40960)", node); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,pterodactyl_server_id,pterodactyl_identifier,node_id,name,status,minecraft_version,server_type,hostname,port,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit) VALUES($1,$2,'10000000-0000-4000-8000-000000000001',$3,$4,$5,'Friends','RUNNING','1.21.1','PAPER','play.example.com',25565,2048,10240,100,2,0)`, id, owner, remote.ID, remote.Identifier, node)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512(p.files["/plugins/voice.jar"])
	receipt, _ := json.Marshal(map[string]any{"collection": "plugins", "identifier": remote.Identifier, "files": []catalog.Package{{Filename: "voice.jar", Size: 14, ProjectID: "9eGKb6K1", VersionID: "server-release", SHA512: hex.EncodeToString(hash[:]), ClientSide: "optional"}}})
	_, err = db.Exec(ctx, `INSERT INTO management_operations(id,server_id,user_id,idempotency_key,action,fingerprint,status,result,request_id) VALUES($1,$2,$3,$4,'catalog.install','test','SUCCEEDED',$5,'test')`, uuid.New(), id, owner, uuid.New(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	c := catalog.New()
	c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		var result any
		v := catalog.Version{ID: "client-release", ProjectID: "9eGKb6K1", Number: "fabric-2.6.24+1.21.1", Type: "release", Loaders: []string{"fabric"}, Games: []string{"1.21.1"}, Files: []catalog.File{{Filename: "client-voice.jar", URL: "https://cdn.modrinth.com/client.jar", Size: 10, Primary: true, Hashes: map[string]string{"sha512": strings.Repeat("a", 128), "sha1": strings.Repeat("b", 40)}}}}
		switch r.URL.Path {
		case "/v2/version/server-release":
			v.ID = "server-release"
			v.Number = "bukkit-2.6.24"
			v.Loaders = []string{"paper"}
			result = v
		case "/v2/project/9eGKb6K1/version":
			result = []catalog.Version{v}
		case "/v2/version/client-release":
			result = v
		case "/v2/project/9eGKb6K1":
			result = map[string]string{"client_side": "required", "server_side": "required"}
		default:
			t.Fatal(r.URL)
		}
		data, _ := json.Marshal(result)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	m := &management.Service{DB: db, Panel: p, Control: &control.Service{DB: db, Panel: p}, Catalog: c}
	s := Service{DB: db, Management: m, Catalog: c, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "meta.fabricmc.net" {
			t.Fatal(r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"loader":{"version":"0.19.5","stable":true}}]`))}, nil
	})}}
	if _, err = s.Publish(ctx, other, id, "test"); !errors.Is(err, server.ErrNotFound) || p.calls != 0 {
		t.Fatal("foreign publish reached Panel", err)
	}
	invite, err := s.Publish(ctx, owner, id, "test")
	if err != nil || !invite.Snapshot.PackReady {
		t.Fatal(invite, err)
	}
	public, err := s.Public(ctx, invite.Slug)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(public)
	for _, private := range []string{owner.String(), id.String(), remote.Identifier, "server.properties", "pack_files", "sha512", "password"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private metadata leaked", private)
		}
	}
	data, err := s.Pack(ctx, invite.Slug)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) != 1 {
		t.Fatal(err)
	}
	r, _ := z.File[0].Open()
	manifest, _ := io.ReadAll(r)
	r.Close()
	var pack map[string]any
	if json.Unmarshal(manifest, &pack) != nil {
		t.Fatal("invalid pack")
	}
	deps := pack["dependencies"].(map[string]any)
	if deps["minecraft"] != "1.21.1" || deps["fabric-loader"] != "0.19.5" {
		t.Fatal(deps)
	}
	files := pack["files"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["path"] != "mods/client-voice.jar" {
		t.Fatal(files)
	}
	settings, err := m.Settings(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	settings.MOTD = "New rules"
	if _, err = m.SaveSettings(ctx, owner, id, uuid.New(), settings, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pack(ctx, invite.Slug); !errors.Is(err, ErrUnavailable) {
		t.Fatal("obsolete setup still downloadable", err)
	}
	if err = s.Revoke(ctx, other, id, "test"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal(err)
	}
	refreshed, err := s.Publish(ctx, owner, id, "test")
	if err != nil || refreshed.Slug != invite.Slug {
		t.Fatal("refresh broke shared address", err)
	}
	if _, err = db.Exec(ctx, "UPDATE player_invites SET expires_at=now()-interval '1 second' WHERE server_id=$1", id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Public(ctx, invite.Slug); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("expired invite remains public", err)
	}
	renewed, err := s.Publish(ctx, owner, id, "test")
	if err != nil || renewed.Slug == invite.Slug {
		t.Fatal("expired capability reused", err)
	}
	invite = renewed
	if err = s.Revoke(ctx, owner, id, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Public(ctx, invite.Slug); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("revoked invite still public", err)
	}
	p.files["/plugins/voice.jar"] = []byte("modified bytes")
	if _, err = s.Publish(ctx, owner, id, "test"); !errors.Is(err, management.ErrChanged) {
		t.Fatal("modified JAR trusted", err)
	}
}
