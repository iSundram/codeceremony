package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment     string
	HTTPAddr        string
	SessionSecret   string
	AllowedOrigin   string
	SeedDemoData    bool
	SessionTTLHours int
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	SMTPEncryption  string
}

func Load() (Config, error) {
	cfg := Config{
		Environment:     envOr("APP_ENV", "development"),
		HTTPAddr:        envOr("HTTP_ADDR", ":8080"),
		SessionSecret:   envOr("SESSION_SECRET", "codeceremony-local-development-secret-change-me"),
		AllowedOrigin:   envOr("ALLOWED_ORIGIN", "http://localhost:3000"),
		SessionTTLHours: 12,
		SMTPHost:        strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:        587,
		SMTPUsername:    strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:    strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		SMTPFrom:        strings.TrimSpace(os.Getenv("SMTP_FROM")),
		SMTPEncryption:  envOr("SMTP_ENCRYPTION", "starttls"),
	}

	if raw := strings.TrimSpace(os.Getenv("SEED_DEMO_DATA")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("SEED_DEMO_DATA: %w", err)
		}
		cfg.SeedDemoData = value
	} else {
		cfg.SeedDemoData = true
	}

	if raw := strings.TrimSpace(os.Getenv("SESSION_TTL_HOURS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be a positive integer")
		}
		cfg.SessionTTLHours = value
	}
	if raw := strings.TrimSpace(os.Getenv("SMTP_PORT")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 65535 {
			return Config{}, fmt.Errorf("SMTP_PORT must be a valid port")
		}
		cfg.SMTPPort = value
	}
	if cfg.SMTPEncryption != "none" && cfg.SMTPEncryption != "starttls" && cfg.SMTPEncryption != "tls" {
		return Config{}, fmt.Errorf("SMTP_ENCRYPTION must be none, starttls, or tls")
	}

	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.SessionSecret) == "" {
		return Config{}, fmt.Errorf("SESSION_SECRET must not be empty")
	}
	if cfg.Environment == "production" && cfg.SessionSecret == "codeceremony-local-development-secret-change-me" {
		return Config{}, fmt.Errorf("SESSION_SECRET must be changed in production")
	}
	if cfg.AllowedOrigin == "" {
		cfg.AllowedOrigin = "http://localhost:3000"
	}

	return cfg, nil
}

func (c Config) IsProduction() bool {
	return strings.EqualFold(c.Environment, "production")
}

func (c Config) SMTPConfigured() bool {
	return c.SMTPHost != "" && c.SMTPFrom != ""
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
