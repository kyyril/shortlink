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

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/config"
	"github.com/kyyril/shortlink/internal/httpx"
)

// Pinger is a health-check interface for readiness (e.g. database pool).
type Pinger interface {
	Ping(ctx context.Context) error
}

// AuthHandler specifies the HTTP handlers for user authentication.
type AuthHandler interface {
	Register(w http.ResponseWriter, r *http.Request)
	Login(w http.ResponseWriter, r *http.Request)
}

// LinkHandler specifies the HTTP handlers for link management.
type LinkHandler interface {
	Create(w http.ResponseWriter, r *http.Request)
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Redirect(w http.ResponseWriter, r *http.Request)
}

// StatsHandler specifies the HTTP handler for link statistics.
type StatsHandler interface {
	GetStats(w http.ResponseWriter, r *http.Request)
}

// RecorderCloser closes the background click recorder.
type RecorderCloser interface {
	Close(ctx context.Context) error
}

// tokenVerifier can validate a JWT token and return the user ID.
type tokenVerifier interface {
	VerifyToken(tokenStr string) (uuid.UUID, error)
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
	authH       AuthHandler
	linkH       LinkHandler
	statsH      StatsHandler
	recorder    RecorderCloser
	tokenVerif  tokenVerifier
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

// WithAuthHandler sets the authentication handler.
func WithAuthHandler(h AuthHandler) Option {
	return func(a *App) {
		a.authH = h
	}
}

// WithLinkHandler sets the link management handler.
// verifier is used to protect link routes with JWT authentication.
func WithLinkHandler(h LinkHandler, verifier tokenVerifier) Option {
	return func(a *App) {
		a.linkH = h
		a.tokenVerif = verifier
	}
}

// WithStatsHandler sets the click statistics handler.
func WithStatsHandler(h StatsHandler) Option {
	return func(a *App) {
		a.statsH = h
	}
}

// WithRecorder attaches the click recorder for lifecycle and graceful shutdown management.
func WithRecorder(r RecorderCloser) Option {
	return func(a *App) {
		a.recorder = r
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
		httpx.Metrics,
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
	mux.Handle("GET /metrics", promhttp.Handler())

	if a.authH != nil {
		mux.HandleFunc("POST /v1/auth/register", a.authH.Register)
		mux.HandleFunc("POST /v1/auth/login", a.authH.Login)
	}

	if a.linkH != nil && a.tokenVerif != nil {
		authn := auth.Authenticate(a.tokenVerif)

		// Public redirect — no auth required.
		mux.HandleFunc("GET /{code}", a.linkH.Redirect)

		// Protected link management routes.
		mux.Handle("POST /v1/links", authn(http.HandlerFunc(a.linkH.Create)))
		mux.Handle("GET /v1/links", authn(http.HandlerFunc(a.linkH.List)))
		mux.Handle("GET /v1/links/{code}", authn(http.HandlerFunc(a.linkH.Get)))
		mux.Handle("DELETE /v1/links/{code}", authn(http.HandlerFunc(a.linkH.Delete)))

		if a.statsH != nil {
			mux.Handle("GET /v1/links/{code}/stats", authn(http.HandlerFunc(a.statsH.GetStats)))
		}
	}

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

	// Graceful shutdown sequence:
	// 1. Mark not ready so load balancers divert traffic away.
	a.ready.Store(false)

	// 2. Server shutdown with deadline — wait for in-flight requests.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	// 3. Close click recorder — flush buffered clicks.
	if a.recorder != nil {
		if err := a.recorder.Close(shutdownCtx); err != nil {
			return fmt.Errorf("close recorder: %w", err)
		}
	}

	a.logger.Info("server shutdown complete")
	return nil
}
