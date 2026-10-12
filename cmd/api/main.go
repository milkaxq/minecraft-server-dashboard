package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/admin"
	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/billing"
	"minecraft-hosting/internal/config"
	"minecraft-hosting/internal/console"
	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/httpapi"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/mail"
	"minecraft-hosting/internal/management"
	"minecraft-hosting/internal/minecraft"
	"minecraft-hosting/internal/plan"
	"minecraft-hosting/internal/pterodactyl"
	"minecraft-hosting/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("API stopped", "category", "startup_or_runtime")
		os.Exit(1)
	}
}
func run() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		return err
	}
	var mailer auth.Mailer = mail.Spool{Directory: c.MailDirectory, FrontendOrigin: c.FrontendOrigin}
	if c.SMTPAddress != "" {
		mailer = mail.SMTP{Address: c.SMTPAddress, Username: c.SMTPUsername, Password: c.SMTPPassword, From: c.SMTPFrom, FrontendOrigin: c.FrontendOrigin}
	}
	authentication, err := auth.New(pool, mailer)
	if err != nil {
		return err
	}
	authentication.RecoveryKey = c.RecoveryKey
	authentication.InviteOnly = c.RegistrationMode == "invite_only"
	if len(c.RecoveryKey) == 32 {
		go authentication.RunRecoveryDelivery(ctx, log)
	}
	var panel pterodactyl.ServerClient
	var consoleSource pterodactyl.ConsoleClient
	var manager pterodactyl.ManagementClient
	panelOrigin := ""
	if c.ClientAPIKey != "" {
		client, e := pterodactyl.NewServerClient(c.PanelURL, c.ClientAPIKey, c.Environment == "development")
		if e != nil {
			return e
		}
		client.ConfigureTransfers(c.TransferOrigins)
		manager = client
		panel = client
		consoleSource = client
		u, _ := url.Parse(c.PanelURL)
		panelOrigin = u.Scheme + "://" + u.Host
	}
	controls := &control.Service{DB: pool, Panel: panel, Players: minecraft.Probe{}}
	liveConsole := &console.Service{DB: pool, Control: controls, Source: consoleSource, FrontendOrigin: c.FrontendOrigin, PanelOrigin: panelOrigin, AllowedOrigins: c.WebsocketOrigins, AllowPlaintext: c.Environment == "development", Lifetime: ctx}
	managementService := &management.Service{DB: pool, Control: controls, Panel: manager}
	controls.EULA = managementService
	var payment billing.PaymentProvider
	if c.PaymentProvider == "mock" {
		payment, err = billing.NewMock(c.MockPaymentSecret)
		if err != nil {
			return err
		}
	}
	billingService := &billing.Service{DB: pool, Provider: payment}
	api := &httpapi.API{DB: pool, Billing: billingService, Admin: &admin.Service{DB: pool}, Auth: authentication, Plans: plan.Service{DB: pool}, Servers: server.Service{DB: pool}, Jobs: jobs.Service{DB: pool}, Management: managementService, Control: controls, Console: liveConsole, Log: log, Origin: c.FrontendOrigin, CookieSecure: c.Environment == "production" || strings.HasPrefix(c.FrontendOrigin, "https://")}
	go billingService.RunReconciler(ctx, log)

	httpServer := &http.Server{Addr: c.Address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	metrics := &http.Server{Addr: ":9090", Handler: api.Metrics(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	errors := make(chan error, 2)
	go func() { errors <- httpServer.ListenAndServe() }()
	go func() { errors <- metrics.ListenAndServe() }()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				if err := authentication.Cleanup(jobCtx); err != nil {
					log.Error("auth cleanup failed", "category", "database")
				}
				cancel()
			}
		}
	}()
	log.Info("API listening", "address", c.Address, "environment", c.Environment)
	select {
	case <-ctx.Done():
	case err = <-errors:
		if err == http.ErrServerClosed {
			err = nil
		}
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e := httpServer.Shutdown(shutdown)
	_ = metrics.Shutdown(shutdown)
	if err != nil {
		return err
	}
	return e
}
