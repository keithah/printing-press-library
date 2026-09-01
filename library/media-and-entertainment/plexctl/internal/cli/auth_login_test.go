package cli

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/config"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/plexruntime/plexauth"
)

type fakePlexLoginClient struct {
	result plexauth.LoginResult
	user   plexauth.User
	err    error
	seen   string
}

func (c *fakePlexLoginClient) Login(context.Context) (plexauth.LoginResult, error) {
	return c.result, c.err
}

func (c *fakePlexLoginClient) User(_ context.Context, token string) (plexauth.User, error) {
	c.seen = token
	if c.err != nil {
		return plexauth.User{}, c.err
	}
	return c.user, nil
}

func TestCompletePlexLoginVerifiesBeforePersisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	cfgPath := filepath.Join(home, "config", "plexctl-pp-cli", "config.toml")
	client := &fakePlexLoginClient{
		result: plexauth.LoginResult{Token: "minted-token"},
		user:   plexauth.User{ID: 42, Username: "keith"},
	}
	result, err := completePlexLogin(context.Background(), cfgPath, client)
	if err != nil {
		t.Fatal(err)
	}
	if client.seen != "minted-token" {
		t.Fatalf("verification token = %q, want minted token", client.seen)
	}
	if result.Username != "keith" || result.PlexID != 42 {
		t.Fatalf("result = %#v, want verified account", result)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "minted-token" {
		t.Fatalf("saved token = %q, want minted token", got)
	}
}

func TestCompletePlexLoginDoesNotPersistUnverifiedToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	cfgPath := filepath.Join(home, "config", "plexctl-pp-cli", "config.toml")
	client := &fakePlexLoginClient{result: plexauth.LoginResult{Token: "minted-token"}, err: errors.New("unauthorized")}

	if _, err := completePlexLogin(context.Background(), cfgPath, client); err == nil {
		t.Fatal("completePlexLogin() succeeded with an unverified token")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "" {
		t.Fatalf("saved token = %q, want empty after verification failure", got)
	}
}
