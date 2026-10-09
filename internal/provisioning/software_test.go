package provisioning

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

func TestApprovedLoaderPinsAndProvisioningSnapshot(t *testing.T) {
	for _, kind := range []string{"SPIGOT", "PURPUR", "FABRIC", "FORGE", "NEOFORGE"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.worker.Verifier = acceptInstallation{}
			ctx := context.Background()
			template := SoftwareTemplate{ServerType: kind, EggID: 7, Image: "java21", Startup: "launcher", VersionVariable: "MC_VERSION", Environment: map[string]string{"LOADER_VERSION": "base"}, VersionEnvironment: map[string]map[string]string{"1.21.1": {"LOADER_VERSION": "approved"}}, Versions: []string{"1.21.1"}}
			if err := ConfigureSoftware(ctx, f.db, f.admin, template); err != nil {
				t.Fatal(err)
			}
			s, err := (server.Service{DB: f.db}).Create(ctx, f.owner, server.CreateInput{PlanID: uuid.MustParse("10000000-0000-4000-8000-000000000001"), Name: "Modded world", MinecraftVersion: "1.21.1", ServerType: kind}, "test")
			if err != nil {
				t.Fatal(err)
			}
			f.enqueue(t, s, jobs.Provision)
			f.once(t)
			remote, err := f.panel.FindServerByExternalID(ctx, s.ID)
			if err != nil {
				t.Fatal(err)
			}
			if remote.Request.Environment["MC_VERSION"] != "1.21.1" || remote.Request.Environment["LOADER_VERSION"] != "approved" || remote.Request.Limits.RAMMB != 2048 {
				t.Fatal("incorrect resolved request", remote.Request)
			}
			if server.SupportsMods(kind) {
				loaderKey := map[string]string{"FABRIC": "LOADER_VERSION", "FORGE": "FORGE_VERSION", "NEOFORGE": "NEOFORGE_VERSION"}[kind]
				template.VersionEnvironment["1.21.1"][loaderKey] = "updated"
				if err = ConfigureSoftware(ctx, f.db, f.admin, template); err != nil {
					t.Fatal(err)
				}
				options, err := (server.Service{DB: f.db}).SoftwareOptions(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, option := range options {
					if option.ServerType == kind && option.LoaderVersion != "updated" {
						t.Fatal("missing public loader version", option)
					}
				}
				stored, err := (server.Service{DB: f.db}).Get(ctx, f.owner, s.ID)
				if err != nil {
					t.Fatal(err)
				}
				want := ""
				if kind == "FABRIC" {
					want = "approved"
				}
				if stored.LoaderVersion != want {
					t.Fatal("existing loader changed with template", stored.LoaderVersion)
				}
			}
			template.VersionEnvironment["1.21.1"]["MC_VERSION"] = "1.20.1"
			if ConfigureSoftware(ctx, f.db, f.admin, template) == nil {
				t.Fatal("release override accepted")
			}
			delete(template.VersionEnvironment["1.21.1"], "MC_VERSION")
			template.VersionEnvironment["1.20.1"] = map[string]string{"LOADER_VERSION": "unapproved"}
			if ConfigureSoftware(ctx, f.db, f.admin, template) == nil {
				t.Fatal("unapproved version accepted")
			}
		})
	}
}

type acceptInstallation struct{}

func (acceptInstallation) VerifyInstallation(context.Context, string, string, string, map[string]string) error {
	return nil
}
