package provisioning

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

type SoftwareTemplate struct {
	ServerType         string                       `json:"server_type"`
	EggID              int                          `json:"egg_id"`
	Image              string                       `json:"docker_image"`
	Startup            string                       `json:"startup"`
	VersionVariable    string                       `json:"version_variable"`
	Environment        map[string]string            `json:"environment"`
	VersionEnvironment map[string]map[string]string `json:"version_environment,omitempty"`
	VersionImages      map[string]string            `json:"version_images,omitempty"`
	Versions           []string                     `json:"versions"`
}

var variablePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
var releasePattern = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}(\.[0-9]{1,2})?$`)

func ConfigureSoftware(ctx context.Context, db *pgxpool.Pool, actor uuid.UUID, t SoftwareTemplate) error {
	if err := (jobs.Service{DB: db}).RequireAdmin(ctx, actor); err != nil {
		return err
	}
	if !server.ValidSoftware(t.ServerType) || t.EggID <= 0 || t.Image == "" || t.Startup == "" || !variablePattern.MatchString(t.VersionVariable) || len(t.Versions) == 0 {
		return fmt.Errorf("invalid software template")
	}
	for k, v := range t.Environment {
		if !variablePattern.MatchString(k) || len(v) > 4096 {
			return fmt.Errorf("invalid egg environment")
		}
	}
	for _, v := range t.Versions {
		if !releasePattern.MatchString(v) {
			return fmt.Errorf("invalid supported release")
		}
	}
	for version, environment := range t.VersionEnvironment {
		approved := false
		for _, v := range t.Versions {
			if version == v {
				approved = true
			}
		}
		if !approved {
			return fmt.Errorf("environment for unapproved release")
		}
		for k, v := range environment {
			if !variablePattern.MatchString(k) || k == t.VersionVariable || len(v) > 4096 {
				return fmt.Errorf("invalid release environment")
			}
		}
	}
	for version, image := range t.VersionImages {
		approved := false
		for _, v := range t.Versions {
			if version == v {
				approved = true
			}
		}
		if !approved || image == "" || len(image) > 255 {
			return fmt.Errorf("invalid release image")
		}
	}
	if t.Environment == nil {
		t.Environment = map[string]string{}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `INSERT INTO software_templates(server_type,egg_id,docker_image,startup,version_variable,environment) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(server_type) DO UPDATE SET egg_id=EXCLUDED.egg_id,docker_image=EXCLUDED.docker_image,startup=EXCLUDED.startup,version_variable=EXCLUDED.version_variable,environment=EXCLUDED.environment,enabled=true`, t.ServerType, t.EggID, t.Image, t.Startup, t.VersionVariable, t.Environment); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE minecraft_versions SET enabled=false WHERE server_type=$1", t.ServerType); err != nil {
		return err
	}
	for _, v := range t.Versions {
		var image *string
		if selected, ok := t.VersionImages[v]; ok {
			image = &selected
		}
		if _, err = tx.Exec(ctx, "INSERT INTO minecraft_versions(server_type,version,environment,docker_image) VALUES($1,$2,$3,$4) ON CONFLICT(server_type,version) DO UPDATE SET enabled=true,environment=EXCLUDED.environment,docker_image=EXCLUDED.docker_image", t.ServerType, v, versionEnvironment(t, v), image); err != nil {
			return err
		}
	}
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "software.configured", uuid.NewString(), map[string]any{"server_type": t.ServerType, "egg_id": t.EggID, "versions": t.Versions}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func versionEnvironment(t SoftwareTemplate, version string) map[string]string {
	if env := t.VersionEnvironment[version]; env != nil {
		return env
	}
	return map[string]string{}
}
