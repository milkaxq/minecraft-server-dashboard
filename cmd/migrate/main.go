package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/database"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err == nil {
		defer db.Close()
		err = database.Migrate(ctx, db)
	}
	if err != nil {
		slog.Error("migration failed", "category", "database")
		os.Exit(1)
	}
	slog.Info("migrations applied")
}
