package config

import (
	"os"
	"testing"
)

func TestLoadUsesRenderPortWhenAppPortIsUnset(t *testing.T) {
	oldAppPort, hadAppPort := os.LookupEnv("APP_PORT")
	t.Cleanup(func() {
		if hadAppPort {
			_ = os.Setenv("APP_PORT", oldAppPort)
			return
		}
		_ = os.Unsetenv("APP_PORT")
	})

	_ = os.Unsetenv("APP_PORT")
	t.Setenv("PORT", "10000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.HTTP.Port != "10000" {
		t.Fatalf("HTTP.Port = %q, want %q", cfg.HTTP.Port, "10000")
	}
}

func TestLoadRejectsMissingDatabaseURLWhenDatabaseIsRequired(t *testing.T) {
	unsetEnv(t, "DATABASE_URL")
	unsetEnv(t, "SUPABASE_DB_PASSWORD")
	t.Setenv("DB_REQUIRED", "true")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error, want database configuration error")
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	oldValue, hadValue := os.LookupEnv(key)
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv(key, oldValue)
			return
		}
		_ = os.Unsetenv(key)
	})

	_ = os.Unsetenv(key)
}
