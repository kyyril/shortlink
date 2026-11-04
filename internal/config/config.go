package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration loaded from environment variables.
// The application fails to start if a required value is missing or invalid.
type Config struct {
	AppEnv          string        `env:"APP_ENV"           envDefault:"development"`
	HTTPAddr        string        `env:"HTTP_ADDR"         envDefault:":8080"`
	BaseURL         string        `env:"BASE_URL"          envDefault:"http://localhost:8080"`
	DatabaseURL     string        `env:"DATABASE_URL,required"`
	JWTSecret       string        `env:"JWT_SECRET,required"`
	JWTTTL          time.Duration `env:"JWT_TTL"           envDefault:"15m"`
	LogLevel        string        `env:"LOG_LEVEL"         envDefault:"info"`
	LogFormat       string        `env:"LOG_FORMAT"        envDefault:"json"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT"  envDefault:"15s"`
	RequestTimeout  time.Duration `env:"REQUEST_TIMEOUT"   envDefault:"5s"`
	RateLimitRPS    float64       `env:"RATE_LIMIT_RPS"    envDefault:"10"`
	RateLimitBurst  int           `env:"RATE_LIMIT_BURST"  envDefault:"20"`
	ClickBufferSize int           `env:"CLICK_BUFFER_SIZE" envDefault:"1024"`
}

// Load reads environment variables into Config and validates it.
// Returns an error if any required variable is missing or any value is invalid.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

// validate checks business rules that env tags cannot express.
func validate(cfg *Config) error {
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes, got %d", len(cfg.JWTSecret))
	}
	if cfg.AppEnv != "development" && cfg.AppEnv != "production" {
		return fmt.Errorf("APP_ENV must be development or production, got %q", cfg.AppEnv)
	}
	if cfg.LogLevel != "debug" && cfg.LogLevel != "info" && cfg.LogLevel != "warn" && cfg.LogLevel != "error" {
		return fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error, got %q", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return fmt.Errorf("LOG_FORMAT must be json or text, got %q", cfg.LogFormat)
	}
	if cfg.JWTTTL <= 0 {
		return fmt.Errorf("JWT_TTL must be positive, got %v", cfg.JWTTTL)
	}
	if cfg.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be positive, got %v", cfg.ShutdownTimeout)
	}
	if cfg.RequestTimeout <= 0 {
		return fmt.Errorf("REQUEST_TIMEOUT must be positive, got %v", cfg.RequestTimeout)
	}
	if cfg.RateLimitRPS <= 0 {
		return fmt.Errorf("RATE_LIMIT_RPS must be positive, got %v", cfg.RateLimitRPS)
	}
	if cfg.RateLimitBurst <= 0 {
		return fmt.Errorf("RATE_LIMIT_BURST must be positive, got %v", cfg.RateLimitBurst)
	}
	if cfg.ClickBufferSize <= 0 {
		return fmt.Errorf("CLICK_BUFFER_SIZE must be positive, got %v", cfg.ClickBufferSize)
	}
	return nil
}
