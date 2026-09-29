package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment   string
	HTTPAddr      string
	SessionSecret string
	// AuditSecret keys the hash chain on the action audit. It defaults to the
	// session secret so a working portal does not need a second variable, and is
	// separate so an operator can rotate sessions without invalidating the audit.
	AuditSecret string
	// auditSecretFromSession records that the audit key is derived rather than
	// configured, so boot can say so instead of leaving an operator to assume
	// they set it.
	auditSecretFromSession bool
	AllowedOrigin          string
	SeedDemoData           bool
	SeedPassword           string
	SessionTTLHours        int
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFrom               string
	SMTPFromName           string
	SMTPEncryption         string
	AppBaseURL             string
	MailInterval           int
	MailBatchSize          int
	MailMaxAttempts        int
	DataDir                string
	FixturesPath           string
	PersistInterval        time.Duration
}

const defaultSeedPassword = "codeceremony-dev"

func Load() (Config, error) {
	cfg := Config{
		Environment:     envOr("APP_ENV", "development"),
		HTTPAddr:        envOr("HTTP_ADDR", ":8080"),
		SessionSecret:   envOr("SESSION_SECRET", "codeceremony-local-development-secret-change-me"),
		AuditSecret:     envOr("AUDIT_SECRET", ""),
		AllowedOrigin:   envOr("ALLOWED_ORIGIN", "http://localhost:3000"),
		SessionTTLHours: 12,
		SeedPassword:    envOr("SEED_PASSWORD", defaultSeedPassword),
		DataDir:         envOr("DATA_DIR", "./data"),
		FixturesPath:    strings.TrimSpace(os.Getenv("FIXTURES_PATH")),
		PersistInterval: 2 * time.Second,
		SMTPHost:        strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:        587,
		SMTPUsername:    strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:    strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		SMTPFrom:        strings.TrimSpace(os.Getenv("SMTP_FROM")),
		SMTPFromName:    envOr("SMTP_FROM_NAME", "CodeCeremony"),
		SMTPEncryption:  envOr("SMTP_ENCRYPTION", "starttls"),
		AppBaseURL:      strings.TrimRight(envOr("APP_BASE_URL", "http://localhost:3000"), "/"),
		MailInterval:    10,
		MailBatchSize:   20,
		MailMaxAttempts: 4,
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
	if raw := strings.TrimSpace(os.Getenv("PERSIST_INTERVAL_SECONDS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return Config{}, fmt.Errorf("PERSIST_INTERVAL_SECONDS must be a positive integer")
		}
		cfg.PersistInterval = time.Duration(value) * time.Second
	}
	if raw := strings.TrimSpace(os.Getenv("MAIL_INTERVAL_SECONDS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return Config{}, fmt.Errorf("MAIL_INTERVAL_SECONDS must be a positive integer")
		}
		cfg.MailInterval = value
	}
	if raw := strings.TrimSpace(os.Getenv("MAIL_BATCH_SIZE")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			return Config{}, fmt.Errorf("MAIL_BATCH_SIZE must be between 1 and 500")
		}
		cfg.MailBatchSize = value
	}
	if raw := strings.TrimSpace(os.Getenv("MAIL_MAX_ATTEMPTS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 10 {
			return Config{}, fmt.Errorf("MAIL_MAX_ATTEMPTS must be between 1 and 10")
		}
		cfg.MailMaxAttempts = value
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
	// An audit chain signed with a predictable key is not tamper evident, it is
	// only tamper resistant against a careless operator. Falling back to the
	// session secret is deliberate, and is why the fallback is checked here
	// rather than left to fail quietly at verification time.
	if strings.TrimSpace(cfg.AuditSecret) == "" {
		cfg.AuditSecret = cfg.SessionSecret
		// No logger is available here, so the fallback is surfaced through the
		// effective value, which the boot log already prints.
		cfg.auditSecretFromSession = true
	}
	// Seeding mints fixed, publicly documented session tokens for accounts that
	// hold organizer and admin rights, and hashes one shared password. That is
	// correct for a self-hosted demo and indefensible in production, so opting
	// in there requires replacing the shared password as well.
	if cfg.IsProduction() && cfg.SeedDemoData {
		if cfg.SeedPassword == defaultSeedPassword {
			return Config{}, fmt.Errorf("SEED_PASSWORD must be set to a private value before SEED_DEMO_DATA can be enabled in production")
		}
		if len(cfg.SeedPassword) < 12 {
			return Config{}, fmt.Errorf("SEED_PASSWORD must be at least 12 characters")
		}
	}
	if cfg.AllowedOrigin == "" {
		cfg.AllowedOrigin = "http://localhost:3000"
	}

	return cfg, nil
}

// AuditSecretIsDerived reports whether the audit chain key is borrowed from the
// session secret rather than set on its own. Boot uses it to say so out loud,
// because an operator who later rotates SESSION_SECRET would otherwise discover
// the consequence by watching old audit entries stop verifying.
func (c Config) AuditSecretIsDerived() bool {
	return c.auditSecretFromSession
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
