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
