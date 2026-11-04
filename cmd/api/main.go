package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kyyril/shortlink/internal/app"
	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/click"
	"github.com/kyyril/shortlink/internal/config"
	"github.com/kyyril/shortlink/internal/link"
	"github.com/kyyril/shortlink/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := app.NewLogger(cfg.LogLevel, cfg.LogFormat, os.Stderr)
	if err != nil {
		return fmt.Errorf("new logger: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	poolCtx, cancelPool := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPool()

	pool, err := store.NewPool(poolCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	// Auth layer
	userRepo := store.NewUserRepo(pool)
	authSvc := auth.NewService(userRepo, cfg.JWTSecret, cfg.JWTTTL, time.Now)
	authHandler := auth.NewHandler(authSvc)

	// Click recorder
	clickRepo := store.NewClickRepo(pool)
	recorder := click.NewRecorder(clickRepo, cfg.ClickBufferSize, 100, 500*time.Millisecond, logger)
	recorder.Start()

	// Link layer
	linkRepo := store.NewLinkRepo(pool)
	idemRepo := store.NewIdempotencyRepo(pool)
	linkSvc := link.NewService(linkRepo, link.NewRandCodeGenerator(), time.Now)
	linkHandler := link.NewHandler(linkSvc, cfg.BaseURL, link.WithClickRecorder(recorder), link.WithIdempotencyStore(idemRepo))

	// Stats layer
	statsSvc := click.NewService(clickRepo, linkSvc, time.Now)
	statsHandler := click.NewHandler(statsSvc)

	application := app.New(
		cfg,
		logger,
		app.WithPinger(pool),
		app.WithAuthHandler(authHandler),
		app.WithLinkHandler(linkHandler, authSvc),
		app.WithStatsHandler(statsHandler),
		app.WithRecorder(recorder),
	)

	logger.Info("starting server", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)
	if err := application.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("run app: %w", err)
	}

	logger.Info("server stopped gracefully")
	return nil
}
