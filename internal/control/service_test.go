package control

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/minecraft"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
	"minecraft-hosting/internal/testutil"
)

type countedPanel struct {
	*pterodactyl.Mock
	power, commands, resources atomic.Int32
	losePower                  atomic.Bool
}

func (p *countedPanel) Power(ctx context.Context, id string, signal pterodactyl.PowerSignal) error {
	p.power.Add(1)
	err := p.Mock.Power(ctx, id, signal)
	if err == nil && p.losePower.Swap(false) {
		return errors.New("lost response containing a private credential")
	}
	return err
}
func (p *countedPanel) SendCommand(ctx context.Context, id, command string) error {
	p.commands.Add(1)
	return p.Mock.SendCommand(ctx, id, command)
}
func (p *countedPanel) Resources(ctx context.Context, id string) (pterodactyl.Usage, error) {
	p.resources.Add(1)
	return p.Mock.Resources(ctx, id)
}

type eulaCheck struct {
	accepted bool
	err      error
}

func (e *eulaCheck) Accepted(context.Context, string) (bool, error) { return e.accepted, e.err }

type probe struct{ calls atomic.Int32 }

func (p *probe) Players(context.Context, string, int) (minecraft.Players, error) {
	p.calls.Add(1)
	return minecraft.Players{Online: 7, Max: 10}, nil
}
func TestLiveAuthorizationAndIdempotency(t *testing.T) {
	db := testutil.Database(t)
	ctx := context.Background()
	owner, admin, mod, other, node, id := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{owner, admin, mod, other} {
		if _, err := db.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,verified_at) VALUES($1,$2,'unused','Player',now())", u, u.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, "INSERT INTO nodes(id,pterodactyl_node_id,name,cpu_capacity,ram_capacity_mb,disk_capacity_mb) VALUES($1,1,'Test',200,4096,20480)", node); err != nil {
		t.Fatal(err)
	}
	panel := &countedPanel{Mock: pterodactyl.NewMock()}
	u, err := panel.CreateUser(ctx, pterodactyl.UserRequest{ExternalID: owner, Email: "player@example.com", Name: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := panel.CreateServer(ctx, pterodactyl.ServerRequest{ExternalID: id, UserID: u.ID, NodeID: 1, AllocationID: 1, EggID: 1, Name: "World", Image: "java", Startup: "minecraft", Limits: pterodactyl.Limits{RAMMB: 2048, DiskMB: 10240, CPU: 100, Backups: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO servers(id,owner_id,plan_id,node_id,pterodactyl_server_id,pterodactyl_identifier,name,status,minecraft_version,server_type,hostname,port,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit)
 VALUES($1,$2,'10000000-0000-4000-8000-000000000001',$3,$4,$5,'World','RUNNING','1.21.1','PAPER','play.example.com',25565,2048,10240,100,2,0)`, id, owner, node, remote.ID, remote.Identifier); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,'ADMIN'),($1,$3,'MODERATOR')", id, admin, mod); err != nil {
		t.Fatal(err)
	}
	players := &probe{}
	agreement := &eulaCheck{}
	s := Service{DB: db, Panel: panel, Players: players, EULA: agreement}
	if err = s.Power(ctx, other, id, uuid.New(), pterodactyl.PowerStart, "", "test"); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("cross-customer power allowed", err)
	}
	if _, err = s.Usage(ctx, other, id); !errors.Is(err, server.ErrNotFound) {
		t.Fatal("cross-customer metrics allowed", err)
	}
	if panel.power.Load() != 0 || panel.resources.Load() != 0 {
		t.Fatal("unauthorized requests reached Panel")
	}
	for _, signal := range []pterodactyl.PowerSignal{pterodactyl.PowerStop, pterodactyl.PowerKill} {
		if err = s.Power(ctx, mod, id, uuid.New(), signal, id.String(), "test"); !errors.Is(err, server.ErrNotFound) {
			t.Fatal("moderator stop/kill allowed", err)
		}
	}
	for _, signal := range []pterodactyl.PowerSignal{pterodactyl.PowerStart, pterodactyl.PowerRestart} {
		if e := s.Power(ctx, mod, id, uuid.New(), signal, "", "eula-denied"); !errors.Is(e, ErrEULA) {
			t.Fatal("power did not require EULA", e)
		}
	}
	agreement.err = errors.New("remote read unavailable")
	if e := s.Power(ctx, owner, id, uuid.New(), pterodactyl.PowerStart, "", "unavailable"); !errors.Is(e, ErrUnavailable) {
		t.Fatal("EULA read failure did not fail closed", e)
	}
	agreement.err = nil
	s.EULA = nil
	if e := s.Power(ctx, owner, id, uuid.New(), pterodactyl.PowerStart, "", "unconfigured"); !errors.Is(e, ErrNotConfigured) {
		t.Fatal("missing EULA checker did not fail closed", e)
	}
	s.EULA = agreement
	if panel.power.Load() != 0 {
		t.Fatal("unaccepted EULA reached power API")
	}
	agreement.accepted = true
	key := uuid.New()
	if err = s.Power(ctx, mod, id, key, pterodactyl.PowerStart, "", "request"); err != nil {
		t.Fatal(err)
	}
	if err = s.Power(ctx, mod, id, key, pterodactyl.PowerStart, "", "replay"); err != nil {
		t.Fatal(err)
	}
	if panel.power.Load() != 1 {
		t.Fatal("replayed power sent twice")
	}
	if err = s.Power(ctx, mod, id, key, pterodactyl.PowerRestart, "", "conflict"); !errors.Is(err, ErrBusy) {
		t.Fatal("changed idempotent body accepted")
	}
	for _, command := range []string{"stop", "minecraft:stop", "/stop", "op Someone", "reload", "list\nstop", "\x00stop", "   "} {
		if err = s.Command(ctx, mod, id, uuid.New(), command, "test"); err == nil {
			t.Errorf("moderator sent %q", command)
		}
	}
	if panel.commands.Load() != 0 {
		t.Fatal("unauthorized commands reached Panel")
	}
	if err = s.Command(ctx, mod, id, uuid.New(), "say Hello friends", "test"); err != nil {
		t.Fatal(err)
	}
	usage, err := s.Usage(ctx, owner, id)
	if err != nil || usage.PlayersOnline == nil || *usage.PlayersOnline != 7 || players.calls.Load() != 1 {
		t.Fatal("players unavailable", err)
	}
	panel.losePower.Store(true)
	uncertain := uuid.New()
	if err = s.Power(ctx, owner, id, uncertain, pterodactyl.PowerRestart, "", "test"); !errors.Is(err, ErrUnknown) {
		t.Fatal("lost power response not marked uncertain")
	}
	before := panel.power.Load()
	if err = s.Power(ctx, owner, id, uncertain, pterodactyl.PowerRestart, "", "replay"); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	if panel.power.Load() != before {
		t.Fatal("uncertain restart repeated")
	}
	if err = s.Power(ctx, admin, id, uuid.New(), pterodactyl.PowerKill, "", "test"); !errors.Is(err, server.ErrInvalid) {
		t.Fatal("kill did not require confirmation")
	}
	if err = s.Power(ctx, admin, id, uuid.New(), pterodactyl.PowerKill, id.String(), "test"); err != nil {
		t.Fatal(err)
	}
	usage, err = s.Usage(ctx, owner, id)
	if err != nil || usage.PlayersOnline == nil || *usage.PlayersOnline != 0 || players.calls.Load() != 1 {
		t.Fatal("offline player query not skipped", err)
	}
	if _, err = db.Exec(ctx, "UPDATE servers SET status='SUSPENDED' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	before = panel.power.Load()
	if err = s.Power(ctx, owner, id, uuid.New(), pterodactyl.PowerStart, "", "test"); !errors.Is(err, ErrState) {
		t.Fatal("suspended server started", err)
	}
	if panel.power.Load() != before {
		t.Fatal("suspended request reached Panel")
	}
	var leaked bool
	if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM audit_logs WHERE metadata::text LIKE '%Hello friends%' OR metadata::text LIKE '%private credential%')").Scan(&leaked); err != nil || leaked {
		t.Fatal("command or error contents in audit", err)
	}
	var cpu, ram, disk int
	if err = db.QueryRow(ctx, "SELECT cpu_limit,ram_mb,disk_mb FROM servers WHERE id=$1", id).Scan(&cpu, &ram, &disk); err != nil || cpu != 100 || ram != 2048 || disk != 10240 {
		t.Fatal("control changed purchased limits", err)
	}
}
