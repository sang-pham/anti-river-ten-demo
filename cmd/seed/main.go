package main

import (
	"context"
	"fmt"
	"os"

	"go-demo/internal/config"
	"go-demo/internal/db"
	"go-demo/internal/observability"
)

func main() {
	// Allow overriding DATABASE_URL via env (required). Other config are fine as defaults.
	cfg, err := config.FromEnv()
	if err != nil {
		panic(err)
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required (e.g., postgres://postgres:postgres@localhost:5434/go_demo?sslmode=disable)")
		os.Exit(2)
	}

	log := observability.NewLogger(cfg.LogLevel)
	log.Info("seeding roles", "dsn", cfg.DatabaseURL)

	dbx, err := db.New(cfg, log)
	if err != nil {
		log.Error("db connect/migrate failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := dbx.Close(); cerr != nil {
			log.Error("database close error", "err", cerr)
		}
	}()

	if err := dbx.SeedDefaultRoles(context.Background()); err != nil {
		log.Error("seed default roles failed", "err", err)
		os.Exit(1)
	}

	log.Info("seed default roles completed")
}