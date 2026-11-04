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
	"github.com/kyyril/shortlink/internal/config"
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

	application := app.New(cfg, logger, app.WithPinger(pool))

	logger.Info("starting server", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)
	if err := application.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("run app: %w", err)
	}

	logger.Info("server stopped gracefully")
	return nil
}
