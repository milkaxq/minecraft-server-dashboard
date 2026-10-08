package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/billing"
	"minecraft-hosting/internal/config"
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
	go (&billing.Service{DB: db}).RunReconciler(ctx, log)
	log.Info("provisioning worker started")
	return (&provisioning.Worker{DB: db, Panel: panel, Log: log}).Run(ctx)
}
