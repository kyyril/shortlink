package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/kyyril/shortlink/internal/config"
	"github.com/kyyril/shortlink/internal/httpx"
)

// Pinger is a health-check interface for readiness (e.g. database pool).
type Pinger interface {
	Ping(ctx context.Context) error
}

// App represents the application composition root.
type App struct {
	cfg         *config.Config
	logger      *slog.Logger
	server      *http.Server
	ready       atomic.Bool
	pinger      Pinger
	ln          net.Listener // optional custom listener for tests
	extraRoutes map[string]http.HandlerFunc
}

// Option configures App.
type Option func(*App)

// WithListener sets a custom net.Listener (useful for random port tests).
func WithListener(ln net.Listener) Option {
	return func(a *App) {
		a.ln = ln
	}
}

// WithPinger sets a custom readiness pinger (e.g. database pool).
func WithPinger(p Pinger) Option {
	return func(a *App) {
		a.pinger = p
	}
}

// WithRoute registers an additional route handler (useful for testing middleware/shutdown).
func WithRoute(pattern string, handler http.HandlerFunc) Option {
	return func(a *App) {
		a.extraRoutes[pattern] = handler
	}
}

// New creates and initializes an App.
func New(cfg *config.Config, logger *slog.Logger, opts ...Option) *App {
	a := &App{
		cfg:         cfg,
		logger:      logger,
		extraRoutes: make(map[string]http.HandlerFunc),
	}

	for _, opt := range opts {
		opt(a)
	}

	mux := a.routes()

	handler := httpx.Chain(
		mux,
		httpx.RequestID,
		httpx.Recover(logger),
		httpx.AccessLog(logger),
		httpx.Timeout(cfg.RequestTimeout),
	)

	a.server = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return a
}

// routes builds the HTTP router for the application.
func (a *App) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /readyz", a.handleReadyz)

	for pattern, h := range a.extraRoutes {
		mux.HandleFunc(pattern, h)
	}

	return mux
}

// Routes returns the configured HTTP handler for testing.
func (a *App) Routes() http.Handler {
	return a.server.Handler
}

// SetReady sets the application readiness state (used during startup, shutdown, and tests).
func (a *App) SetReady(ready bool) {
	a.ready.Store(ready)
}
func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	_ = httpx.EncodeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz handles readiness probes. It returns 200 only if the server is marked ready
// and the pinger (database) is reachable. Otherwise it returns 503.
func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !a.ready.Load() {
		httpx.WriteProblem(w, r, http.StatusServiceUnavailable, "not_ready", "server is shutting down or not ready")
		return
	}

	if a.pinger != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := a.pinger.Ping(ctx); err != nil {
			a.logger.Warn("readiness probe database ping failed", "error", err)
			httpx.WriteProblem(w, r, http.StatusServiceUnavailable, "database_down", "database is not reachable")
			return
		}
	}

	_ = httpx.EncodeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Run starts the server and blocks until ctx is cancelled, then executes graceful shutdown.
func (a *App) Run(ctx context.Context) error {
	// Mark ready to accept traffic
	a.ready.Store(true)

	serverErr := make(chan error, 1)
	go func() {
		if a.ln != nil {
			serverErr <- a.server.Serve(a.ln)
		} else {
			serverErr <- a.server.ListenAndServe()
		}
	}()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	case <-ctx.Done():
		a.logger.Info("shutdown signal received, starting graceful shutdown")
	}

	// 1. Mark not ready so load balancers divert traffic away
	a.ready.Store(false)

	// 2. Server shutdown with deadline
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	a.logger.Info("server shutdown complete")
	return nil
}
