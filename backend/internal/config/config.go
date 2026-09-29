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
	// MailParallelism is how many sends one dispatch tick runs at once. It
	// exists because a single unreachable endpoint used to hold the whole
	// batch: with a fifteen second dial timeout, twenty queued messages took
	// five minutes to discover that the first one was dead.
	MailParallelism int
	DataDir         string
	FixturesPath    string
	PersistInterval time.Duration
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
		MailParallelism: 4,
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

	// APP_ENV is a set of switches, so an unrecognised value has to be refused
	// rather than treated as "not production". It used to be compared with
	// EqualFold against the single string "production", which meant APP_ENV=prod
	// — one character of the most consequential variable in the file — silently
	// disabled every guard rail below and logged environment=prod.
	switch strings.ToLower(strings.TrimSpace(cfg.Environment)) {
	case "development", "test", "production":
	default:
		return Config{}, fmt.Errorf("APP_ENV must be development, test or production, got %q", cfg.Environment)
	}
	cfg.Environment = strings.ToLower(strings.TrimSpace(cfg.Environment))

	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.SessionSecret) == "" {
		return Config{}, fmt.Errorf("SESSION_SECRET must not be empty")
	}
	if cfg.IsProduction() && cfg.SessionSecret == "codeceremony-local-development-secret-change-me" {
		return Config{}, fmt.Errorf("SESSION_SECRET must be changed in production")
	}
	// A one-character secret passed the equality check above and was then used
	// to derive the audit key, so the audit chain was forgeable by anyone who
	// guessed it. Length is the cheap check that catches the realistic mistake.
	if cfg.IsProduction() && len(cfg.SessionSecret) < 32 {
		return Config{}, fmt.Errorf("SESSION_SECRET must be at least 32 characters in production, got %d", len(cfg.SessionSecret))
	}
	// SESSION_TTL_HOURS was only checked for >= 1, and the value is multiplied
	// out into a time.Duration at boot. 9223372036854775807 hours overflowed to
	// -1h0m0s, so every session minted by that portal expired the instant it
	// was issued and only the seeded long-lived tokens still worked.
	if cfg.SessionTTLHours < 1 || cfg.SessionTTLHours > 24*365 {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be between 1 and %d hours", 24*365)
	}
	// The audit chain is signed with a key derived from AUDIT_SECRET. Falling
	// back to the session secret is convenient in development, where one
	// variable is nicer than two, and it is why the fallback is checked here
	// rather than left to fail quietly at verification time.
	//
	// It is not acceptable in production, and the previous handling was a
	// warning. A leaked session secret — the one secret most likely to leak,
	// because it is the one operators paste into compose files — then also
	// bought the ability to forge audit history, which is exactly the property
	// the chain exists to have. A warning is not a control, so production
	// refuses to boot without its own key.
	if strings.TrimSpace(cfg.AuditSecret) == "" {
		if cfg.IsProduction() {
			return Config{}, fmt.Errorf(
				"AUDIT_SECRET must be set in production: deriving the audit key from SESSION_SECRET means a leaked session secret can forge audit history")
		}
		cfg.AuditSecret = cfg.SessionSecret
		// No logger is available here, so the fallback is surfaced through the
		// effective value, which the boot log already prints.
		cfg.auditSecretFromSession = true
	}
	// Seeding mints fixed, publicly documented session tokens for accounts that
	// hold organizer and admin rights, and hashes one shared password. That is
	// correct for a self-hosted demo and indefensible in production.
	//
	// The password is the smaller half of the problem. The tokens are literal
	// constants in the repository and in .dogfood.toml, valid for a hundred
	// years, so requiring a private SEED_PASSWORD in production bought nothing:
	// a portal configured that way still handed every anonymous visitor an admin
	// session on the first request. Production therefore refuses to seed at all,
	// and the token table is not something a password can protect.
	if cfg.IsProduction() && cfg.SeedDemoData {
		return Config{}, fmt.Errorf(
			"SEED_DEMO_DATA cannot be enabled in production: it mints fixed, publicly documented session tokens for organizer and admin accounts")
	}
	if cfg.SeedPassword != "" && len(cfg.SeedPassword) < 12 && !cfg.SeedDemoData {
		return Config{}, fmt.Errorf("SEED_PASSWORD must be at least 12 characters")
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
