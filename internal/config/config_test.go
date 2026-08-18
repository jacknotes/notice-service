package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("INSTANCE_ID", "")
	cfg := Load()
	if cfg.DBHost != "127.0.0.1" {
		t.Errorf("DBHost default = %q, want 127.0.0.1", cfg.DBHost)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default = %q, want 8080", cfg.Port)
	}
	if cfg.EncryptKey == "" {
		t.Error("EncryptKey should default to a non-empty generated key")
	}
	if cfg.InstanceID == "" {
		t.Error("InstanceID should default to a generated uuid")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("PORT", "9090")
	cfg := Load()
	if cfg.DBHost != "db.example.com" {
		t.Errorf("DBHost = %q, want db.example.com", cfg.DBHost)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
}
