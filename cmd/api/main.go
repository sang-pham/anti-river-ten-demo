// @title go-demo API
// @version 1.0
// @description REST API demo with auth
// @BasePath /
// @schemes http
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	_ "go-demo/docs"

	"go-demo/internal/auth"
	"go-demo/internal/config"
	"go-demo/internal/db"
	apihttp "go-demo/internal/http"
	"go-demo/internal/observability"
	"go-demo/internal/sqllog"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		panic(err)
	}

	log := observability.NewLogger(cfg.LogLevel)

	// Initialize database and auth service
	dbx, err := db.New(cfg, log)
	if err != nil {
		log.Error("database initialization failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := dbx.Close(); cerr != nil {
			log.Error("database close error", "err", cerr)
		}
	}()

	// Seed default roles (DEMO.ROLE): USER and ADMIN
	if err := dbx.SeedDefaultRoles(context.Background()); err != nil {
		log.Error("seed default roles failed", "err", err)
		os.Exit(1)
	}

	authSvc := auth.NewService(dbx, cfg, log)

	// Initialize sql log repository and migrate table
	sqlRepo := sqllog.NewRepository(dbx.Gorm)
	if err := sqlRepo.Migrate(context.Background()); err != nil {
		log.Error("sql log migration failed", "err", err)
		os.Exit(1)
	}

	// Router and server
	router := apihttp.NewRouter(cfg, log, authSvc, sqlRepo)
	server := apihttp.NewServer(cfg, router, log)

	// Run with signal cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", "err", err)
		os.Exit(1)
	}

	log.Info("server exited cleanly")
}
