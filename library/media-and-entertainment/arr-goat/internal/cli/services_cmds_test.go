package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/config"
	"github.com/spf13/cobra"
)

func buildTransmissionCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cfg, _ := config.Load("/nonexistent/config.toml")
	tr := &cobra.Command{Use: "transmission"}
	addTransmissionCommands(tr, cfg)
	return tr
}

func TestRemoveHasDeleteAndYesFlags(t *testing.T) {
	tr := buildTransmissionCmd(t)
	var got *cobra.Command
	for _, c := range tr.Commands() {
		if c.Name() == "remove" {
			got = c
		}
	}
	if got == nil {
		t.Fatal("remove command not registered under transmission")
	}
	for _, name := range []string{"delete", "yes"} {
		if got.Flags().Lookup(name) == nil {
			t.Fatalf("missing --%s flag on remove", name)
		}
	}
}

// TestRemoveAllGuardedWithoutYes: with a key configured, `remove all` must
// refuse before any network call because it is destructive.
func TestRemoveAllGuardedWithoutYes(t *testing.T) {
	t.Setenv("ARR_GOAT_TRANSMISSION_KEY", "user:pass")
	tr := buildTransmissionCmd(t)
	var out bytes.Buffer
	tr.SetOut(&out)
	tr.SetErr(&out)
	tr.SetArgs([]string{"remove", "all"})
	err := tr.Execute()
	if err == nil {
		t.Fatalf("expected destructive-guard error, got nil (out=%q)", out.String())
	}
	if !strings.Contains(err.Error(), "destructive") {
		t.Fatalf("expected destructive guard error, got: %v", err)
	}
}

// TestRemoveSpecificIDsNotAll: specific ids are not "all", so the guard
// must NOT fire (it proceeds toward the network call, which fails without a
// reachable host — we only assert the guard didn't trigger).
func TestRemoveSpecificIDsNotAll(t *testing.T) {
	t.Setenv("ARR_GOAT_TRANSMISSION_KEY", "user:pass")
	t.Setenv("ARR_GOAT_TRANSMISSION_BASE_URL", "http://127.0.0.1:1")
	tr := buildTransmissionCmd(t)
	var out bytes.Buffer
	tr.SetOut(&out)
	tr.SetErr(&out)
	tr.SetArgs([]string{"remove", "42"})
	err := tr.Execute()
	if err != nil && strings.Contains(err.Error(), "destructive") {
		t.Fatalf("specific id wrongly treated as all: %v", err)
	}
}

func TestTruncateUTF8(t *testing.T) {
	s := "Tëst with 日本語 characters"
	out := truncate(s, 6)
	if strings.ContainsRune(out, '\uFFFD') {
		t.Fatalf("truncate produced invalid UTF-8: %q", out)
	}
	if l := len([]rune(out)); l > 7 { // n-1 + the "…" rune
		t.Fatalf("truncate rune length %d > 7", l)
	}
}

func TestTruncateShortNoChange(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("short string should be unchanged, got %q", got)
	}
}
