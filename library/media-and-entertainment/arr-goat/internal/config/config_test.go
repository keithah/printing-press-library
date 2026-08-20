package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndResolve(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(`
[[services]]
name = "sonarr"
base_url = "https://hadm.net/tv"
key_env = "SONARR_GOAT_X_API_KEY"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("want 1 configured service, got %d", len(cfg.Services))
	}
	s, err := cfg.Resolve("sonarr")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if s.BaseURL != "https://hadm.net/tv" {
		t.Fatalf("base = %q", s.BaseURL)
	}
	if _, err := cfg.Resolve("notaservice"); err == nil {
		t.Fatal("expected error for unknown service")
	}
}

func TestEnvServiceInjection(t *testing.T) {
	t.Setenv("ARR_GOAT_SERVICES", "radarr,prowlarr")
	t.Setenv("ARR_GOAT_RADARR_BASE_URL", "https://hadm.net/movies")
	cfg, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s, err := cfg.Resolve("radarr")
	if err != nil {
		t.Fatalf("Resolve radarr: %v", err)
	}
	if s.BaseURL != "https://hadm.net/movies" {
		t.Fatalf("radarr base = %q", s.BaseURL)
	}
}

func TestKnownServices(t *testing.T) {
	cfg, _ := Load(filepath.Join(t.TempDir(), "x.toml"))
	names := strings.Join(cfg.Known(), ",")
	for _, want := range []string{"sonarr", "radarr", "prowlarr", "bazarr", "sabnzbd", "transmission"} {
		if !strings.Contains(names, want) {
			t.Fatalf("known services missing %q: %s", want, names)
		}
	}
}

func TestSetServiceAddAndSave(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	// Add a brand-new service with both fields + default key env.
	cfg.SetService("transmission", "https://hadm.net/transmission/", "arr_goat_transmission_key")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// Reload and confirm persisted.
	cfg2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s, err := cfg2.Resolve("transmission")
	if err != nil {
		t.Fatal(err)
	}
	if s.BaseURL != "https://hadm.net/transmission" { // trailing slash trimmed
		t.Fatalf("base = %q", s.BaseURL)
	}
	if s.KeyEnv != "ARR_GOAT_TRANSMISSION_KEY" {
		t.Fatalf("keyenv = %q", s.KeyEnv)
	}
	// Update an existing service: change only the base URL.
	cfg2.SetService("transmission", "https://new.example/transmission", "")
	s, _ = cfg2.Resolve("transmission")
	if s.BaseURL != "https://new.example/transmission" {
		t.Fatalf("updated base = %q", s.BaseURL)
	}
	if s.KeyEnv != "ARR_GOAT_TRANSMISSION_KEY" { // key env preserved
		t.Fatalf("keyenv should be preserved, got %q", s.KeyEnv)
	}
}
