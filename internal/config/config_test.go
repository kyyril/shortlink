package config_test

import (
	"testing"

	"github.com/kyyril/shortlink/internal/config"
)

// validEnv sets all required env vars for a valid config.
func validEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db?sslmode=disable")
	t.Setenv("JWT_SECRET", "this-secret-is-exactly-32-bytes!")
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("BASE_URL", "http://localhost:8080")
	t.Setenv("JWT_TTL", "15m")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("SHUTDOWN_TIMEOUT", "15s")
	t.Setenv("REQUEST_TIMEOUT", "5s")
	t.Setenv("RATE_LIMIT_RPS", "10")
	t.Setenv("RATE_LIMIT_BURST", "20")
	t.Setenv("CLICK_BUFFER_SIZE", "1024")
}

// TestLoad runs table-driven tests for config.Load.
// t.Setenv is incompatible with t.Parallel(), so subtests are sequential.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantErr bool
	}{
		{
			name: "ValidConfig_ReturnsConfig",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
			},
			wantErr: false,
		},
		{
			name: "MissingDatabaseURL_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				t.Setenv("JWT_SECRET", "this-secret-is-exactly-32-bytes!")
				// DATABASE_URL intentionally set to empty; caarlos0/env v11 treats
				// empty string as missing for required fields.
				t.Setenv("DATABASE_URL", "")
			},
			wantErr: true,
		},
		{
			name: "MissingJWTSecret_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db?sslmode=disable")
				// JWT_SECRET intentionally set to empty.
				t.Setenv("JWT_SECRET", "")
			},
			wantErr: true,
		},
		{
			name: "JWTSecretTooShort_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
				t.Setenv("JWT_SECRET", "tooshort")
			},
			wantErr: true,
		},
		{
			name: "InvalidAppEnv_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
				t.Setenv("APP_ENV", "staging")
			},
			wantErr: true,
		},
		{
			name: "InvalidLogLevel_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
				t.Setenv("LOG_LEVEL", "verbose")
			},
			wantErr: true,
		},
		{
			name: "InvalidLogFormat_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
				t.Setenv("LOG_FORMAT", "xml")
			},
			wantErr: true,
		},
		{
			name: "InvalidJWTTTL_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
				t.Setenv("JWT_TTL", "not-a-duration")
			},
			wantErr: true,
		},
		{
			name: "ZeroShutdownTimeout_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
				t.Setenv("SHUTDOWN_TIMEOUT", "0s")
			},
			wantErr: true,
		},
		{
			name: "ZeroRequestTimeout_ReturnsError",
			setup: func(t *testing.T) {
				t.Helper()
				validEnv(t)
				t.Setenv("REQUEST_TIMEOUT", "0s")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			_, err := config.Load()
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestLoad_ValidConfig_FieldsMatchEnv verifies that Load populates fields correctly.
func TestLoad_ValidConfig_FieldsMatchEnv(t *testing.T) {
	validEnv(t)
	t.Setenv("JWT_SECRET", "this-secret-is-exactly-32-bytes!")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.JWTSecret != "this-secret-is-exactly-32-bytes!" {
		t.Error("JWTSecret not set correctly")
	}
}
