package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/backups"
	"minecraft-hosting/internal/billing"
	"minecraft-hosting/internal/config"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/provisioning"
	"minecraft-hosting/internal/pterodactyl"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("worker stopped", "category", "configuration_or_database")
		os.Exit(1)
	}
}
func run(log *slog.Logger) error {
	c, err := config.LoadWorker()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := pgxpool.New(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = db.Ping(pingCtx)
	cancel()
	if err != nil {
		return err
	}
	panel, err := pterodactyl.NewApplication(c.PanelURL, c.ApplicationKey, c.Environment == "development")
	if err != nil {
		return err
	}
	var verifier provisioning.InstallationVerifier
	if c.InstallationKey != "" {
		client, e := pterodactyl.NewServerClient(c.PanelURL, c.InstallationKey, c.Environment == "development")
		if e != nil {
			return e
		}
		verifier = provisioning.ReceiptVerifier{Files: client}
	}
	var background sync.WaitGroup
	defer func() { stop(); background.Wait() }()
	background.Add(1)
	go func() { defer background.Done(); (&billing.Service{DB: db}).RunReconciler(ctx, log) }()
	if c.BackupSchedulerEnabled {
		backupClient, e := pterodactyl.NewServerClient(c.PanelURL, c.BackupClientKey, c.Environment == "development")
		if e != nil {
			return e
		}
		scheduler := &backups.Worker{DB: db, Panel: backupClient, Control: &control.Service{DB: db, Panel: backupClient}, Log: log}
		background.Add(1)
		go func() { defer background.Done(); _ = scheduler.Run(ctx) }()
		log.Info("backup scheduler started")
	}
	log.Info("provisioning worker started")
	return (&provisioning.Worker{DB: db, Panel: panel, Log: log, Verifier: verifier}).Run(ctx)
}
