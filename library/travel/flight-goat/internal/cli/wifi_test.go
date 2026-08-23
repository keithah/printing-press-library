// Copyright 2026 Matt Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the `wifi` (SeatWifi) command group.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runWifiCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	flags := &rootFlags{}
	cmd := newWifiCmd(flags)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errb.String(), err
}

func TestWifiCmd_HelpListsSubcommands(t *testing.T) {
	stdout, _, err := runWifiCmd(t, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"flight", "airline", "airlines", "rollouts", "speed", "airline-speed", "search"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestWifiFlight_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"flight", "UA1234"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "/api/v1/flights/UA1234") {
		t.Errorf("dry-run should contain flight path, got: %s", s)
	}
	if !strings.Contains(s, "dry run") {
		t.Errorf("dry-run marker missing: %s", s)
	}
}

func TestWifiAirline_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"airline", "UA"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "/api/airlines/UA") {
		t.Errorf("got %q", out.String())
	}
}

func TestWifiAirlines_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"airlines"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "/api/airlines") {
		t.Errorf("got %q", out.String())
	}
}

func TestWifiRollouts_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"rollouts", "UA"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "/api/rollouts/UA") {
		t.Errorf("got %q", out.String())
	}
}

func TestWifiRollouts_All_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"rollouts"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "/api/rollouts") {
		t.Errorf("got %q", out.String())
	}
}

func TestWifiSpeed_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"speed", "UA1234"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "/api/speed-reports/stats/UA1234") {
		t.Errorf("got %q", out.String())
	}
}

func TestWifiSearch_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"search", "united"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "/api/search") {
		t.Errorf("got %q", out.String())
	}
}

func TestWifiAirlineSpeed_DryRun(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	cmd := newWifiCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"airline-speed", "UA"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "/api/speed-reports/airline/UA") {
		t.Errorf("got %q", out.String())
	}
}

func TestWifiFlight_RequiresArg(t *testing.T) {
	_, _, err := runWifiCmd(t, "flight")
	if err == nil {
		t.Fatal("expected error for missing flight arg")
	}
}

func TestWifiSearch_RequiresArg(t *testing.T) {
	_, _, err := runWifiCmd(t, "search")
	if err == nil {
		t.Fatal("expected error for missing search arg")
	}
}
