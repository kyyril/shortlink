package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kyyril/shortlink/internal/app"
	"github.com/kyyril/shortlink/internal/config"
	"github.com/kyyril/shortlink/internal/httpx"
)

type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(ctx context.Context) error {
	return f.err
}

func testConfig() *config.Config {
	return &config.Config{
		AppEnv:          "development",
		HTTPAddr:        ":8080",
		BaseURL:         "http://localhost:8080",
		DatabaseURL:     "postgres://user:pass@localhost:5432/db",
		JWTSecret:       "this-secret-is-at-least-32-bytes-long",
		JWTTTL:          15 * time.Minute,
		LogLevel:        "info",
		LogFormat:       "json",
		ShutdownTimeout: 5 * time.Second,
		RequestTimeout:  2 * time.Second,
		RateLimitRPS:    10,
		RateLimitBurst:  20,
		ClickBufferSize: 1024,
	}
}

func TestHealthz_AlwaysReturns200OK(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	application := app.New(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	application.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestReadyz_DatabaseHealthy_Returns200OK(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	pinger := &fakePinger{err: nil}

	application := app.New(cfg, logger, app.WithPinger(pinger))
	application.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	application.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("status = %q, want %q", body["status"], "ready")
	}
}

func TestReadyz_DatabaseDown_Returns503(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	pinger := &fakePinger{err: errors.New("connection refused")}

	application := app.New(cfg, logger, app.WithPinger(pinger))
	application.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	application.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var prob httpx.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Code != "database_down" {
		t.Errorf("problem code = %q, want %q", prob.Code, "database_down")
	}
}

func TestReadyz_NotReady_Returns503(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	application := app.New(cfg, logger)
	application.SetReady(false)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	application.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var prob httpx.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Code != "not_ready" {
		t.Errorf("problem code = %q, want %q", prob.Code, "not_ready")
	}
}
