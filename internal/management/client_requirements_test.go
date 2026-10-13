package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/catalog"
	"minecraft-hosting/internal/server"
)

func TestClientRequirementsOwnershipAndReconciliation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.s.ClientRequirements(ctx, f.other, f.id); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("foreign access", err)
	}
	if f.panel.calls != 0 {
		t.Fatal("unauthorized Panel call")
	}
	f.panel.folders["/plugins"] = true
	f.panel.folders["/plugins/voicechat"] = true
	f.panel.files["/plugins/voicechat.jar"] = []byte("jar")
	f.panel.files["/plugins/manual.jar"] = []byte("manual")
	f.panel.files["/plugins/changed.jar"] = []byte("changed")
	f.panel.files["/plugins/linked.jar"] = []byte("link")
	f.panel.symlinks["/plugins/linked.jar"] = true
	f.panel.files["/plugins/voicechat/voicechat-server.properties"] = []byte("force_voice_chat=true\nsecret=do-not-return\n")
	receipt, _ := json.Marshal(map[string]any{"identifier": f.identifier, "collection": "plugins", "files": []catalog.Package{
		{Filename: "voicechat.jar", Size: 3, ProjectID: "9eGKb6K1", ClientSide: "optional"},
		{Filename: "removed.jar", Size: 3, ProjectID: "removed", ClientSide: "required"},
		{Filename: "changed.jar", Size: 1, ProjectID: "changed", ClientSide: "unsupported"},
	}})
	if _, err := f.db.Exec(ctx, `INSERT INTO management_operations(id,server_id,user_id,idempotency_key,action,fingerprint,status,result,request_id) VALUES($1,$2,$3,$4,'catalog.install','test','SUCCEEDED',$5,'test')`, uuid.New(), f.id, f.owner, uuid.New(), receipt); err != nil {
		t.Fatal(err)
	}
	for _, user := range []uuid.UUID{f.owner, f.admin, f.mod} {
		result, err := f.s.ClientRequirements(ctx, user, f.id)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Addons) != 3 {
			t.Fatal("removed or symlinked addon included", result)
		}
		voice := result.Addons[2]
		if voice.ProjectID != "9eGKb6K1" || voice.VoiceRequired == nil || !*voice.VoiceRequired {
			t.Fatal("voice join rule missing", voice)
		}
		if result.Addons[0].ClientSide != "unknown" || result.Addons[1].ClientSide != "unknown" {
			t.Fatal("uploaded or changed files treated as known", result)
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), "do-not-return") || strings.Contains(string(encoded), "secret") {
			t.Fatal("configuration leaked")
		}
	}
	f.panel.files["/plugins/voicechat/voicechat-server.properties"] = []byte("force_voice_chat=false\n")
	result, err := f.s.ClientRequirements(ctx, f.mod, f.id)
	if err != nil || result.Addons[2].VoiceRequired == nil || *result.Addons[2].VoiceRequired {
		t.Fatal("optional voice rule not updated", result, err)
	}
	delete(f.panel.files, "/plugins/voicechat/voicechat-server.properties")
	result, err = f.s.ClientRequirements(ctx, f.owner, f.id)
	if err != nil || result.Addons[2].VoiceRequired != nil {
		t.Fatal("missing config assumed optional", result, err)
	}
}
