package management

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func voiceFixture(t *testing.T) fixture {
	t.Helper()
	f := setup(t)
	ctx := context.Background()
	f.panel.folders["/plugins"] = true
	f.panel.files["/plugins/voicechat-bukkit-2.6.jar"] = []byte("fixture jar already validated at upload")
	allocation := uuid.New()
	_, err := f.db.Exec(ctx, `INSERT INTO node_allocations(id,node_id,pterodactyl_allocation_id,hostname,port,voice_enabled) SELECT $1,node_id,2,'play.example.com',24454,true FROM servers WHERE id=$2`, allocation, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, `INSERT INTO server_addon_ports(server_id,allocation_id,addon,status) VALUES($1,$2,'simple-voice-chat','READY')`, f.id, allocation); err != nil {
		t.Fatal(err)
	}
	remote, err := f.panel.FindServerByExternalID(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.panel.AssignAllocation(ctx, pterodactyl.AdditionalAllocationRequest{ServerID: remote.ID, PrimaryAllocationID: remote.Request.AllocationID, AdditionalAllocationID: 2, Limits: remote.Request.Limits}); err != nil {
		t.Fatal(err)
	}
	return f
}
func TestVoiceSettingsDefaultsPreservationRevisionOwnershipAndLimits(t *testing.T) {
	f := voiceFixture(t)
	ctx := context.Background()
	for _, user := range []uuid.UUID{f.mod, f.other} {
		if _, err := f.s.VoiceChat(ctx, user, f.id); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("foreign voice settings", err)
		}
	}
	config, err := f.s.VoiceChat(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Installed || config.Exists || config.Values.Distance != 48 || !config.Values.Groups || config.Values.Required {
		t.Fatal("wrong defaults", config)
	}
	f.panel.folders["/plugins/voicechat"] = true
	f.panel.files["/plugins/voicechat/voicechat-server.properties"] = []byte("# Keep this comment\r\ncodec=AUDIO\r\nmax_voice_distance=72\nmax_voice_distance=96\nenable_groups=false\ncustom\\ key=unchanged\\ value\n")
	config, err = f.s.VoiceChat(ctx, f.admin, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if config.Values.Distance != 96 || config.Values.Groups || config.Values.Whisper != 24 {
		t.Fatal("existing or missing fields overwritten", config.Values)
	}
	in := VoiceInput{VoiceValues: config.Values, Revision: config.Revision}
	in.Distance = 80
	for _, user := range []uuid.UUID{f.mod, f.other} {
		if _, err = f.s.SaveVoiceChat(ctx, user, f.id, uuid.New(), in, "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("unauthorized save", err)
		}
	}
	key := uuid.New()
	if _, err = f.s.SaveVoiceChat(ctx, f.admin, f.id, key, in, "test"); err != nil {
		t.Fatal(err)
	}
	text := string(f.panel.files["/plugins/voicechat/voicechat-server.properties"])
	for _, wanted := range []string{"# Keep this comment\r\n", "codec=AUDIO\r\n", "custom\\ key=unchanged\\ value\n", "max_voice_distance=80\n", "port=24454\n", "bind_address=*\n", "voice_host=play.example.com\\:24454\n"} {
		if !strings.Contains(text, wanted) {
			t.Fatal("lost or unsafe configuration", wanted, text)
		}
	}
	if strings.Count(text, "max_voice_distance=") != 1 {
		t.Fatal("duplicate managed keys retained")
	}
	if _, err = f.s.SaveVoiceChat(ctx, f.admin, f.id, key, in, "test"); err != nil {
		t.Fatal("save not idempotent", err)
	}
	if _, err = f.s.SaveVoiceChat(ctx, f.admin, f.id, uuid.New(), in, "test"); !errors.Is(err, ErrChanged) {
		t.Fatal("stale overwrite", err)
	}
	config, err = f.s.VoiceChat(ctx, f.owner, f.id)
	if err != nil || !config.Configured {
		t.Fatal("saved configuration not recognized", config, err)
	}
	in = VoiceInput{VoiceValues: config.Values, Revision: config.Revision}
	in.Whisper = 81
	if _, err = f.s.SaveVoiceChat(ctx, f.owner, f.id, uuid.New(), in, "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("invalid distance accepted", err)
	}
	var ram, disk, cpu int
	if err = f.db.QueryRow(ctx, "SELECT ram_mb,disk_mb,cpu_limit FROM servers WHERE id=$1", f.id).Scan(&ram, &disk, &cpu); err != nil || ram != 2048 || disk != 10240 || cpu != 100 {
		t.Fatal("plan changed", err)
	}
}
func TestVoiceSettingsFirstSaveCreatesDirectoriesAndRejectsSymlinksAndOnlineWrites(t *testing.T) {
	f := voiceFixture(t)
	ctx := context.Background()
	config, err := f.s.VoiceChat(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	in := VoiceInput{VoiceValues: config.Values, Revision: config.Revision}
	if err = f.panel.Power(ctx, f.identifier, "start"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveVoiceChat(ctx, f.owner, f.id, uuid.New(), in, "test"); !errors.Is(err, ErrOffline) {
		t.Fatal("online settings changed", err)
	}
	if err = f.panel.Power(ctx, f.identifier, "stop"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveVoiceChat(ctx, f.owner, f.id, uuid.New(), in, "test"); err != nil {
		t.Fatal(err)
	}
	if !f.panel.folders["/plugins/voicechat"] {
		t.Fatal("configuration folder not created")
	}
	f.panel.symlinks["/plugins/voicechat"] = true
	if _, err = f.s.VoiceChat(ctx, f.owner, f.id); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("symlink traversed", err)
	}
}
func TestVoiceConfigCannotUseUnassignedPortsOrOverwriteUnsupportedValues(t *testing.T) {
	f := voiceFixture(t)
	ctx := context.Background()
	f.panel.folders["/plugins/voicechat"] = true
	file := "/plugins/voicechat/voicechat-server.properties"
	f.panel.files[file] = []byte("max_voice_distance=unknown\n")
	if _, err := f.s.VoiceChat(ctx, f.owner, f.id); !errors.Is(err, ErrAddonConfig) {
		t.Fatal("unsupported custom value silently defaulted", err)
	}
	f.panel.files[file] = []byte("max_voice_distance=48\n")
	config, err := f.s.VoiceChat(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.db.Exec(ctx, "UPDATE node_allocations SET pterodactyl_allocation_id=99 WHERE id=(SELECT allocation_id FROM server_addon_ports WHERE server_id=$1)", f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveVoiceChat(ctx, f.owner, f.id, uuid.New(), VoiceInput{VoiceValues: config.Values, Revision: config.Revision}, "test"); !errors.Is(err, ErrAddonPort) {
		t.Fatal("foreign allocation accepted", err)
	}
	if string(f.panel.files[file]) != "max_voice_distance=48\n" {
		t.Fatal("failed validation mutated file")
	}
	_, err = f.db.Exec(ctx, "INSERT INTO subscriptions(id,user_id,server_id,plan_id,price_cents,currency,billing_period,status,current_period_end) VALUES($1,$2,$3,'10000000-0000-4000-8000-000000000001',975,'USD','MONTHLY','ACTIVE',now()-interval '1 day')", uuid.New(), f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.VoiceChat(ctx, f.owner, f.id); !errors.Is(err, control.ErrState) {
		t.Fatal("expired subscriber read config", err)
	}
}

func TestVoiceModConfigurationUsesLoaderConfigDirectory(t *testing.T) {
	f := voiceFixture(t)
	ctx := context.Background()
	if _, err := f.db.Exec(ctx, "UPDATE servers SET server_type='FABRIC' WHERE id=$1", f.id); err != nil {
		t.Fatal(err)
	}
	f.panel.folders["/mods"] = true
	f.panel.files["/mods/voicechat-fabric-2.6.jar"] = []byte("validated upload fixture")
	config, err := f.s.VoiceChat(ctx, f.owner, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveVoiceChat(ctx, f.owner, f.id, uuid.New(), VoiceInput{VoiceValues: config.Values, Revision: config.Revision}, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(f.panel.files["/config/voicechat/voicechat-server.properties"]), "port=24454") {
		t.Fatal("mod configuration not written to its own location")
	}
	if _, exists := f.panel.files["/plugins/voicechat/voicechat-server.properties"]; exists {
		t.Fatal("mod wrote Paper configuration")
	}
}

func TestAddonReadExceptionOnlyAllowsVoiceSetupJobs(t *testing.T) {
	f := voiceFixture(t)
	ctx := context.Background()
	queue := jobs.Service{DB: f.db}
	j, err := queue.Enqueue(ctx, f.owner, f.id, uuid.New(), jobs.VoicePort, jobs.Input{Reason: "voice setup"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	config, err := f.s.VoiceChat(ctx, f.owner, f.id)
	if err != nil || config.Network.Status != "PREPARING" {
		t.Fatal("voice setup not inspectable", config, err)
	}
	if _, err = f.s.Control.Access(ctx, f.db, f.owner, f.id, server.Settings); !errors.Is(err, control.ErrState) {
		t.Fatal("setup job allowed a write", err)
	}
	if _, err = f.s.VoiceNetworkStatus(ctx, f.other, f.id); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("foreign network status", err)
	}
	if _, err = f.db.Exec(ctx, "UPDATE jobs SET status='FAILED' WHERE id=$1", j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, "UPDATE users SET is_admin=true WHERE id=$1", f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err = queue.Enqueue(ctx, f.owner, f.id, uuid.New(), jobs.Suspend, jobs.Input{Reason: "suspend test"}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.VoiceChat(ctx, f.owner, f.id); !errors.Is(err, control.ErrState) {
		t.Fatal("lifecycle job bypassed", err)
	}
}

type voiceWriteRace struct {
	*memoryPanel
	afterNetwork, afterFolder func()
}

func (p *voiceWriteRace) NetworkAllocations(ctx context.Context, id string) ([]int, error) {
	ids, err := p.memoryPanel.NetworkAllocations(ctx, id)
	if p.afterNetwork != nil {
		callback := p.afterNetwork
		p.afterNetwork = nil
		callback()
	}
	return ids, err
}
func (p *voiceWriteRace) CreateFolder(ctx context.Context, id, root, name string) error {
	err := p.memoryPanel.CreateFolder(ctx, id, root, name)
	if p.afterFolder != nil {
		callback := p.afterFolder
		p.afterFolder = nil
		callback()
	}
	return err
}
func TestVoiceSaveRevalidatesAfterRemoteReadsAndDirectoryCreation(t *testing.T) {
	t.Run("external edit", func(t *testing.T) {
		f := voiceFixture(t)
		ctx := context.Background()
		file := "/plugins/voicechat/voicechat-server.properties"
		f.panel.folders["/plugins/voicechat"] = true
		f.panel.files[file] = []byte("max_voice_distance=48\ncodec=AUDIO\n")
		config, err := f.s.VoiceChat(ctx, f.owner, f.id)
		if err != nil {
			t.Fatal(err)
		}
		changed := []byte("max_voice_distance=96\ncodec=VOIP\n")
		f.s.Panel = &voiceWriteRace{memoryPanel: f.panel, afterNetwork: func() { f.panel.files[file] = changed }}
		if _, err = f.s.SaveVoiceChat(ctx, f.owner, f.id, uuid.New(), VoiceInput{VoiceValues: config.Values, Revision: config.Revision}, "race"); !errors.Is(err, ErrChanged) {
			t.Fatal("late edit overwritten", err)
		}
		if string(f.panel.files[file]) != string(changed) {
			t.Fatal("external edit lost")
		}
	})
	t.Run("late membership revocation", func(t *testing.T) {
		f := voiceFixture(t)
		ctx := context.Background()
		config, err := f.s.VoiceChat(ctx, f.admin, f.id)
		if err != nil {
			t.Fatal(err)
		}
		f.s.Panel = &voiceWriteRace{memoryPanel: f.panel, afterFolder: func() {
			if _, err := f.db.Exec(ctx, "DELETE FROM server_members WHERE server_id=$1 AND user_id=$2", f.id, f.admin); err != nil {
				t.Fatal(err)
			}
		}}
		if _, err = f.s.SaveVoiceChat(ctx, f.admin, f.id, uuid.New(), VoiceInput{VoiceValues: config.Values, Revision: config.Revision}, "race"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("revoked member wrote configuration", err)
		}
		if _, exists := f.panel.files["/plugins/voicechat/voicechat-server.properties"]; exists {
			t.Fatal("configuration written after revocation")
		}
	})
}
