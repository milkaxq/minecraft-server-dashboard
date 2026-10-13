package playerjoin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
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

	p.files["/plugins/unknown.jar"] = []byte("catalog bytes")
	p.files["/plugins/optional.jar"] = []byte("catalog bytes")
	extras, _ := json.Marshal(map[string]any{"collection": "plugins", "identifier": remote.Identifier, "files": []catalog.Package{
		{Filename: "unknown.jar", Title: "VeinMiner", Size: 13, ProjectID: "known-unknown", VersionID: "unknown-version", SHA512: func() string { h := sha512.Sum512([]byte("catalog bytes")); return hex.EncodeToString(h[:]) }(), ClientSide: "unknown"},
		{Filename: "optional.jar", Title: "Optional plugin", Size: 13, ProjectID: "optional-project", VersionID: "optional-version", SHA512: func() string { h := sha512.Sum512([]byte("catalog bytes")); return hex.EncodeToString(h[:]) }(), ClientSide: "optional"},
	}})
	_, err = db.Exec(ctx, `INSERT INTO management_operations(id,server_id,user_id,idempotency_key,action,fingerprint,status,result,request_id) VALUES($1,$2,$3,$4,'catalog.install','test','SUCCEEDED',$5,'test')`, uuid.New(), id, owner, uuid.New(), extras)
	if err != nil {
		t.Fatal(err)
	}
	clientBytes := []byte("client jar")
	clientHash := sha512.Sum512(clientBytes)
	clientSHA1 := sha1.Sum(clientBytes)
	optionalLookups := 0
	expireDuringDownload := false
	clientChannel := "release"
	c := catalog.New()
	c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		var result any
		v := catalog.Version{ID: "client-release", ProjectID: "9eGKb6K1", Number: "fabric-2.6.24+1.21.1", Type: clientChannel, Loaders: []string{"fabric"}, Games: []string{"1.21.1"}, Files: []catalog.File{{Filename: "client-voice.jar", URL: "https://cdn.modrinth.com/client.jar", Size: 10, Primary: true, Hashes: map[string]string{"sha512": hex.EncodeToString(clientHash[:]), "sha1": hex.EncodeToString(clientSHA1[:])}}}}
		switch r.URL.Path {
		case "/client.jar":
			if expireDuringDownload {
				if _, err := db.Exec(ctx, "UPDATE player_invites SET expires_at=now()-interval '1 second' WHERE server_id=$1", id); err != nil {
					t.Fatal(err)
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(clientBytes))}, nil
		case "/v2/version/optional-version":
			optionalLookups++
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}"))}, nil
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

	if len(invite.Snapshot.Notes) != 2 || !strings.Contains(strings.Join(invite.Snapshot.Notes, " "), "VeinMiner") || len(invite.Snapshot.Addons) != 1 {
		t.Fatal("unknown catalog metadata or unavailable optional plugin blocked or polluted pack", invite.Snapshot)
	}
	if optionalLookups != 0 {
		t.Fatal("default pack resolved optional plugins")
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

	downloads, err := s.ClientDownloads(ctx, invite.Slug)
	if err != nil || len(downloads) != 1 || downloads[0].Path != "mods/client-voice.jar" || downloads[0].Downloads[0] != "https://cdn.modrinth.com/client.jar" {
		t.Fatal("manual downloads differ from verified pack", downloads, err)
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

	optionalInvite, err := s.PublishWithOptions(ctx, owner, id, "test", PublishOptions{IncludeOptional: true})
	if err != nil || !optionalInvite.Snapshot.IncludeOptional || optionalLookups != 1 {
		t.Fatal("optional feature choice not honored", optionalInvite, err)
	}
	clientChannel = "beta"
	blockedBeta, err := s.Publish(ctx, owner, id, "test")
	if err != nil || blockedBeta.Snapshot.PackReady || !strings.Contains(blockedBeta.Snapshot.PackReason, "marked beta") {
		t.Fatal("default allowed beta", blockedBeta, err)
	}
	enabledBeta, err := s.PublishWithOptions(ctx, owner, id, "test", PublishOptions{IncludeBeta: true})
	if err != nil || !enabledBeta.Snapshot.PackReady || !enabledBeta.Snapshot.IncludeBeta || !strings.Contains(strings.Join(enabledBeta.Snapshot.Notes, " "), "beta client build") {
		t.Fatal("beta option was not published", enabledBeta, err)
	}
	clientChannel = "release"

	bundle, err := s.Bundle(ctx, invite.Slug)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil || len(archive.File) != 3 {
		t.Fatal("bad client bundle", err)
	}
	for _, f := range archive.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == "mods/client-voice.jar" && !bytes.Equal(b, clientBytes) {
			t.Fatal("wrong bundled jar")
		}
		if f.Name == "README.txt" && !strings.Contains(string(b), "play.example.com:25565") {
			t.Fatal("missing join instructions")
		}
	}
	expireDuringDownload = true
	if _, err = s.Bundle(ctx, invite.Slug); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("expired invite served during bundle construction", err)
	}
	expireDuringDownload = false
	if _, err = db.Exec(ctx, "UPDATE player_invites SET expires_at=now()+interval '30 days' WHERE server_id=$1", id); err != nil {
		t.Fatal(err)
	}
	originalBytes := bytes.Clone(clientBytes)
	clientBytes = []byte("wrong file")
	if _, err = s.Bundle(ctx, invite.Slug); !errors.Is(err, ErrUnavailable) {
		t.Fatal("bad download checksum trusted", err)
	}
	clientBytes = originalBytes
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
	if _, err = s.ClientDownloads(ctx, invite.Slug); !errors.Is(err, ErrUnavailable) {
		t.Fatal("invalidated manual downloads remain available", err)
	}
	if _, err = s.Bundle(ctx, invite.Slug); !errors.Is(err, ErrUnavailable) {
		t.Fatal("invalidated bundle remains downloadable", err)
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

	if _, err = db.Exec(ctx, `UPDATE management_operations SET result=jsonb_set(result,'{files,1,client_side}','"required"') WHERE server_id=$1 AND result->'files'->1->>'project_id'='optional-project'`, id); err != nil {
		t.Fatal(err)
	}
	blocked, err := s.Publish(ctx, owner, id, "test")
	if err != nil || blocked.Snapshot.PackReady || !strings.Contains(blocked.Snapshot.PackReason, "Optional plugin") {
		t.Fatal("missing required client silently skipped", blocked, err)
	}
	p.files["/plugins/uploaded.jar"] = []byte("manual upload")
	blocked, err = s.Publish(ctx, owner, id, "test")
	if err != nil || blocked.Snapshot.PackReady || !strings.Contains(blocked.Snapshot.PackReason, "uploaded.jar") {
		t.Fatal("manual upload incorrectly treated as catalog addon", blocked, err)
	}
	delete(p.files, "/plugins/uploaded.jar")
	if _, err = s.Bundle(ctx, invite.Slug); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("revoked bundle remains downloadable", err)
	}
	if _, err = s.ClientDownloads(ctx, invite.Slug); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("revoked manual downloads remain available", err)
	}
	p.files["/plugins/voice.jar"] = []byte("modified bytes")
	if _, err = s.Publish(ctx, owner, id, "test"); !errors.Is(err, management.ErrChanged) {
		t.Fatal("modified JAR trusted", err)
	}
}
