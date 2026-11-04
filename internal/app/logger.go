package app

import (
	"fmt"
	"io"
	"log/slog"
)

// NewLogger builds a *slog.Logger from level and format strings.
// w is where log output is written (use os.Stderr in production, a bytes.Buffer in tests).
// level must be one of: debug, info, warn, error.
// format must be one of: json, text.
func NewLogger(level, format string, w io.Writer) (*slog.Logger, error) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown log level %q: must be debug, info, warn, or error", level)
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("unknown log format %q: must be json or text", format)
	}

	return slog.New(handler), nil
}
