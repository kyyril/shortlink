//go:build integration

package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/kyyril/shortlink/internal/app"
	"github.com/kyyril/shortlink/internal/store"
)

func TestReadiness_WithDatabase_AC16(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://shortlink:shortlink@localhost:5433/shortlink?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	cfg := testConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	application := app.New(cfg, logger, app.WithPinger(pool))
	application.SetReady(true)

	// 1. Database is up: /readyz returns 200, /healthz returns 200
	reqReady := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recReady := httptest.NewRecorder()
	application.Routes().ServeHTTP(recReady, reqReady)

	if recReady.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want %d", recReady.Code, http.StatusOK)
	}

	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recHealth := httptest.NewRecorder()
	application.Routes().ServeHTTP(recHealth, reqHealth)

	if recHealth.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want %d", recHealth.Code, http.StatusOK)
	}

	// 2. Close pool (simulating database connection loss / down)
	pool.Close()

	recReadyDown := httptest.NewRecorder()
	application.Routes().ServeHTTP(recReadyDown, reqReady)

	if recReadyDown.Code != http.StatusServiceUnavailable {
		t.Errorf("when database is down: /readyz status = %d, want %d", recReadyDown.Code, http.StatusServiceUnavailable)
	}

	// /healthz must still return 200 (process liveness is unaffected)
	recHealthAfter := httptest.NewRecorder()
	application.Routes().ServeHTTP(recHealthAfter, reqHealth)

	if recHealthAfter.Code != http.StatusOK {
		t.Errorf("when database is down: /healthz status = %d, want %d", recHealthAfter.Code, http.StatusOK)
	}
}
