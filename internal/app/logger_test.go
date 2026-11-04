package app_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/kyyril/shortlink/internal/app"
)

func TestNewLogger_UnknownLevel_ReturnsError(t *testing.T) {
	_, err := app.NewLogger("verbose", "json", &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown level, got nil")
	}
}

func TestNewLogger_UnknownFormat_ReturnsError(t *testing.T) {
	_, err := app.NewLogger("info", "xml", &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

func TestNewLogger_LevelFiltering(t *testing.T) {
	// When level is "warn", debug and info messages must not appear.
	tests := []struct {
		name        string
		configLevel string
		logFunc     func(l *slog.Logger)
		wantLogged  bool
	}{
		{
			name:        "DebugSuppressedAtInfoLevel",
			configLevel: "info",
			logFunc:     func(l *slog.Logger) { l.Debug("secret debug") },
			wantLogged:  false,
		},
		{
			name:        "InfoPassesAtInfoLevel",
			configLevel: "info",
			logFunc:     func(l *slog.Logger) { l.Info("hello info") },
			wantLogged:  true,
		},
		{
			name:        "InfoSuppressedAtWarnLevel",
			configLevel: "warn",
			logFunc:     func(l *slog.Logger) { l.Info("suppressed info") },
			wantLogged:  false,
		},
		{
			name:        "WarnPassesAtWarnLevel",
			configLevel: "warn",
			logFunc:     func(l *slog.Logger) { l.Warn("visible warn") },
			wantLogged:  true,
		},
		{
			name:        "DebugPassesAtDebugLevel",
			configLevel: "debug",
			logFunc:     func(l *slog.Logger) { l.Debug("debug visible") },
			wantLogged:  true,
		},
		{
			name:        "InfoSuppressedAtErrorLevel",
			configLevel: "error",
			logFunc:     func(l *slog.Logger) { l.Info("suppressed") },
			wantLogged:  false,
		},
		{
			name:        "ErrorPassesAtErrorLevel",
			configLevel: "error",
			logFunc:     func(l *slog.Logger) { l.Error("real error") },
			wantLogged:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := app.NewLogger(tc.configLevel, "json", &buf)
			if err != nil {
				t.Fatalf("NewLogger() unexpected error: %v", err)
			}

			tc.logFunc(logger)

			logged := buf.Len() > 0
			if logged != tc.wantLogged {
				t.Errorf("wantLogged=%v but got output=%q", tc.wantLogged, buf.String())
			}
		})
	}
}

func TestNewLogger_JSONFormat_HasExpectedKeys(t *testing.T) {
	var buf bytes.Buffer
	logger, err := app.NewLogger("info", "json", &buf)
	if err != nil {
		t.Fatalf("NewLogger() unexpected error: %v", err)
	}

	logger.Info("test message", "request_id", "abc123", "status", 200)

	var out map[string]any
	if err := json.NewDecoder(&buf).Decode(&out); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	requiredKeys := []string{"time", "level", "msg"}
	for _, key := range requiredKeys {
		if _, ok := out[key]; !ok {
			t.Errorf("JSON output missing key %q; got: %v", key, out)
		}
	}
	if out["msg"] != "test message" {
		t.Errorf("msg = %v, want %q", out["msg"], "test message")
	}
	if out["request_id"] != "abc123" {
		t.Errorf("request_id = %v, want %q", out["request_id"], "abc123")
	}
	if v, ok := out["status"]; !ok || v != float64(200) {
		t.Errorf("status = %v, want 200", v)
	}
}

func TestNewLogger_TextFormat_IsReadable(t *testing.T) {
	var buf bytes.Buffer
	logger, err := app.NewLogger("info", "text", &buf)
	if err != nil {
		t.Fatalf("NewLogger() unexpected error: %v", err)
	}

	logger.Info("text format check", "key", "value")

	out := buf.String()
	if !strings.Contains(out, "text format check") {
		t.Errorf("text output missing message; got: %s", out)
	}
	if !strings.Contains(out, "key=value") {
		t.Errorf("text output missing key=value pair; got: %s", out)
	}
}
