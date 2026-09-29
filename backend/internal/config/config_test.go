package config

import "testing"

func TestLoadUsesLocalDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("SEED_DEMO_DATA", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if !cfg.SeedDemoData {
		t.Fatal("SeedDemoData = false, want true")
	}
	if cfg.AllowedOrigin != "http://localhost:3000" {
		t.Fatalf("AllowedOrigin = %q", cfg.AllowedOrigin)
	}
}

func TestLoadRejectsDevelopmentSecretInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "codeceremony-local-development-secret-change-me")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want production secret error")
	}
}

func TestLoadParsesSeedFlag(t *testing.T) {
	t.Setenv("SEED_DEMO_DATA", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SeedDemoData {
		t.Fatal("SeedDemoData = true, want false")
	}
}

func TestSMTPConfigurationIsOptionalAndParsed(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.org")
	t.Setenv("SMTP_FROM", "noreply@example.org")
	t.Setenv("SMTP_PORT", "2525")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.SMTPConfigured() {
		t.Fatal("SMTPConfigured() = false, want true")
	}
	if cfg.SMTPPort != 2525 {
		t.Fatalf("SMTPPort = %d, want 2525", cfg.SMTPPort)
	}
}

func TestLoadRejectsInvalidSMTPPort(t *testing.T) {
	t.Setenv("SMTP_PORT", "not-a-port")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want SMTP port error")
	}
}

func TestAuditSecretFallsBackToTheSessionSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "a-real-session-secret")
	t.Setenv("AUDIT_SECRET", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AuditSecret != "a-real-session-secret" {
		t.Errorf("AuditSecret = %q, want the session secret", cfg.AuditSecret)
	}
	if !cfg.AuditSecretIsDerived() {
		t.Error("a borrowed audit secret was not reported as derived")
	}

	// A dedicated key wins, and is the point of having one: rotating sessions
	// must not invalidate the audit.
	t.Setenv("AUDIT_SECRET", "a-real-audit-secret")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AuditSecret != "a-real-audit-secret" {
		t.Errorf("AuditSecret = %q, want the configured value", cfg.AuditSecret)
	}
	if cfg.AuditSecretIsDerived() {
		t.Error("a configured audit secret was reported as derived")
	}
}
