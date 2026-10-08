// control is an operator-only CLI with privileged database access, never an HTTP route.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/config"
	"minecraft-hosting/internal/node"
	"minecraft-hosting/internal/provisioning"
	"minecraft-hosting/internal/pterodactyl"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Operator command failed. Check configuration, privileges, and supplied IDs; secrets and remote response bodies are omitted.")
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("use control admin, software or node")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	databaseURL, err := config.OperatorDatabaseURL()
	if err != nil {
		return err
	}
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	f := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	switch os.Args[1] {
	case "admin":
		email := f.String("email", "", "verified account to promote")
		reason := f.String("reason", "", "audit reason")
		if err = f.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *email == "" || len(*reason) < 3 || len(*reason) > 500 {
			return errors.New("email and reason required")
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(context.Background())
		var id uuid.UUID
		if err = tx.QueryRow(ctx, "UPDATE users SET is_admin=true,updated_at=now() WHERE email=$1 AND verified_at IS NOT NULL RETURNING id", *email).Scan(&id); err != nil {
			return err
		}
		if err = audit.Record(ctx, tx, id, uuid.Nil, "operator.admin_promoted", uuid.NewString(), map[string]string{"reason": *reason, "source": "privileged_cli"}); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		fmt.Println("Administrator:", id)
		return nil
	case "software":
		actor := f.String("actor", "", "administrator UUID")
		file := f.String("file", "", "software JSON configuration")
		if err = f.Parse(os.Args[2:]); err != nil {
			return err
		}
		id, err := uuid.Parse(*actor)
		if err != nil {
			return err
		}
		source, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer source.Close()
		decoder := json.NewDecoder(io.LimitReader(source, 1<<20))
		decoder.DisallowUnknownFields()
		var t provisioning.SoftwareTemplate
		if err = decoder.Decode(&t); err != nil {
			return err
		}
		if err = decoder.Decode(&struct{}{}); err != io.EOF {
			return errors.New("trailing JSON")
		}
		if err = provisioning.ConfigureSoftware(ctx, db, id, t); err != nil {
			return err
		}
		fmt.Println("Software configured:", t.ServerType)
		return nil
	case "node":
		actor := f.String("actor", "", "administrator UUID")
		panelID := f.Int("panel-id", 0, "Panel node ID")
		ram := f.Int("ram-mb", 0, "sellable RAM budget after host reserve")
		disk := f.Int("disk-mb", 0, "sellable disk budget after host reserve")
		cpu := f.Int("cpu", 0, "sellable CPU percentage points")
		dedicated := f.Bool("dedicated", false, "confirm exclusive management by this application")
		if err = f.Parse(os.Args[2:]); err != nil {
			return err
		}
		id, err := uuid.Parse(*actor)
		if err != nil {
			return err
		}
		c, err := config.LoadWorker()
		if err != nil {
			return err
		}
		panel, err := pterodactyl.NewApplication(c.PanelURL, c.ApplicationKey, c.Environment == "development")
		if err != nil {
			return err
		}
		nodeID, err := node.Register(ctx, db, panel, id, *panelID, *ram, *disk, *cpu, *dedicated)
		if err != nil {
			return err
		}
		fmt.Println("Node registered:", nodeID)
		return nil
	default:
		return errors.New("unknown command")
	}
}
