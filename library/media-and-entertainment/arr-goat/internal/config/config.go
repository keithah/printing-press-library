// Package config holds the per-service fleet configuration for arr-goat.
//
// arr-goat manages a set of self-hosted "arr" / Servarr-family services, each
// with its own base URL and X-Api-Key credential. Services are configured in a
// TOML file (default ~/.config/arr-goat/config.toml) and/or environment
// variables. A single binary dispatches to each service's command surface.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Service is one configured arr-family instance.
type Service struct {
	Name    string `toml:"name"`     // canonical lowercased name (sonarr, radarr, ...)
	BaseURL string `toml:"base_url"` // e.g. https://hadm.net/tv
	KeyEnv  string `toml:"key_env"`  // env var holding the X-Api-Key (never inline)
}

// Config is the full arr-goat fleet configuration.
type Config struct {
	Path     string    `toml:"-"`
	Services []Service `toml:"services"`
}

// DefaultConfigPath returns the user config location.
func DefaultConfigPath() string {
	if v := os.Getenv("ARR_GOAT_CONFIG"); v != "" {
		return v
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "config.toml"
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "arr-goat", "config.toml")
}

// Load reads the config from path if present and merges any services declared
// via an ARR_GOAT_SERVICES env list (comma-separated canonical names). Missing
// file is not an error; the CLI still works from env or via built-in discovery.
func Load(path string) (*Config, error) {
	cfg := &Config{Path: path}
	if b, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	// Env-declared services take precedence / are added.
	envList := os.Getenv("ARR_GOAT_SERVICES")
	if envList != "" {
		configured := map[string]bool{}
		for _, s := range cfg.Services {
			configured[strings.ToLower(s.Name)] = true
		}
		for _, name := range strings.Split(envList, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			if configured[name] {
				continue
			}
			cfg.Services = append(cfg.Services, Service{
				Name:   name,
				KeyEnv: "ARR_GOAT_" + strings.ToUpper(strings.NewReplacer("-", "_").Replace(name)) + "_KEY",
			})
		}
	}
	sort.Slice(cfg.Services, func(i, j int) bool { return cfg.Services[i].Name < cfg.Services[j].Name })
	return cfg, nil
}

// Known returns the canonical list of arr-family services arr-goat understands.
// BaseURL is filled from the ARR_GOAT_<NAME>_BASE_URL env when available.
func (c *Config) Known() []string {
	// Order matters for display; sonarr/radarr first as the primary surfaces.
	return []string{"sonarr", "radarr", "prowlarr", "bazarr", "sabnzbd", "transmission"}
}

// Resolve returns the service config for name (by canonical name), applying env
// overrides for its key and base URL.
func (c *Config) Resolve(name string) (Service, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range c.Services {
		if s.Name == name {
			return c.applyEnv(s), nil
		}
	}
	// Allow env-only service not in file.
	for _, known := range c.Known() {
		if known == name {
			s := Service{Name: name, KeyEnv: "ARR_GOAT_" + strings.ToUpper(name) + "_KEY"}
			return c.applyEnv(s), nil
		}
	}
	return Service{}, fmt.Errorf("unknown service %q (known: %s)", name, strings.Join(c.Known(), ", "))
}

func (c *Config) applyEnv(s Service) Service {
	up := strings.ToUpper(s.Name)
	if v := os.Getenv("ARR_GOAT_" + up + "_BASE_URL"); v != "" {
		s.BaseURL = v
	}
	if s.KeyEnv == "" {
		s.KeyEnv = "ARR_GOAT_" + up + "_KEY"
	}
	return s
}

// Save writes the config (600 perms). Credentials are only ever referenced by
// env var name — never stored inline.
func (c *Config) Save() error {
	if c.Path == "" {
		c.Path = DefaultConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return err
	}
	b, err := toml.Marshal(struct {
		Services []Service `toml:"services"`
	}{Services: c.Services})
	if err != nil {
		return err
	}
	tmp := c.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.Path)
}

// Key returns the resolved API key for a service from its env var.
func (c *Config) Key(s Service) string {
	if s.KeyEnv == "" {
		s.KeyEnv = "ARR_GOAT_" + strings.ToUpper(s.Name) + "_KEY"
	}
	return strings.TrimSpace(os.Getenv(s.KeyEnv))
}

// AuthHeader returns the header name used for this service's credential:
// X-Api-Key for Servarr/REST services, empty for transmission (which uses
// HTTP Basic paired with an underlying user credential).
func (s Service) AuthHeader() string {
	if s.Name == "transmission" {
		return "" // basic auth instead
	}
	if s.Name == "sabnzbd" {
		return "" // key sent as query param
	}
	return "X-Api-Key"
}

// SetService updates (or adds) a service in the config with the given base URL
// and key env var name. An empty baseURL / keyEnv leaves that field unchanged
// (or, for a new service, defaulted). Returns the resulting service.
func (c *Config) SetService(name, baseURL, keyEnv string) Service {
	name = strings.ToLower(strings.TrimSpace(name))
	for i := range c.Services {
		if c.Services[i].Name == name {
			if baseURL != "" {
				c.Services[i].BaseURL = strings.TrimRight(baseURL, "/")
			}
			if keyEnv != "" {
				c.Services[i].KeyEnv = strings.ToUpper(strings.TrimSpace(keyEnv))
			}
			return c.Services[i]
		}
	}
	s := Service{Name: name}
	s.BaseURL = strings.TrimRight(baseURL, "/")
	s.KeyEnv = strings.ToUpper(strings.TrimSpace(keyEnv))
	if s.KeyEnv == "" {
		s.KeyEnv = "ARR_GOAT_" + strings.ToUpper(name) + "_KEY"
	}
	c.Services = append(c.Services, s)
	return s
}
