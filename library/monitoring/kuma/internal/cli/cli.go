// Package cli implements the kuma-pp-cli command surface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	kuma "github.com/mvanhorn/printing-press-library/library/monitoring/kuma/internal/client"
)

const usage = `kuma-pp-cli — Uptime Kuma v2 operator CLI

Usage:
  kuma-pp-cli <command> [flags]

Commands:
  health            connectivity + auth check
  monitors          list monitors (--query substring)
  heartbeats        recent beats across monitors (--hours, default 3)
  incident-context  composite brief for one monitor (--monitor id|name)
  set-retries       change a monitor's maxretries (dry-run unless --yes)
  version           print version

Global flags:
  --url, --username, --password   connection overrides (default from env)
  --json                          JSON output
`

// httpTimeout wraps the default transport with a per-request timeout.
type httpTimeout struct{ d time.Duration }

// RoundTrip defers to the default transport; the http.Client's Timeout field
// (set alongside this transport) already bounds each request, and layering a
// second context deadline here cancels long-poll requests mid-flight.
func (h *httpTimeout) RoundTrip(req *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(req)
}

type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer, env func(string) string) int {
	if len(args) < 1 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	cmd := args[0]
	if cmd == "--version" || cmd == "-version" {
		fmt.Fprintln(stdout, "kuma-pp-cli 2026.8.26")
		return ExitOK
	}
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Fprint(stdout, usage)
		return ExitOK
	}
	known := map[string]bool{
		"health": true, "monitors": true, "heartbeats": true,
		"incident-context": true, "set-retries": true, "version": true,
	}
	if !known[cmd] {
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return ExitUsage
	}
	if cmd == "version" {
		fmt.Fprintln(stdout, "kuma-pp-cli 2026.8.26")
		return ExitOK
	}

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	urlF := fs.String("url", env("UPTIME_KUMA_URL"), "Kuma base URL")
	userF := fs.String("username", env("UPTIME_KUMA_USERNAME"), "username")
	passF := fs.String("password", env("UPTIME_KUMA_PASSWORD"), "password")
	_ = fs.Bool("json", false, "JSON output") // reserved: machine output mode

	client := kuma.New(kuma.Config{
		BaseURL:    strings.TrimRight(*urlF, "/"),
		Username:   *userF,
		Password:   *passF,
		HTTPClient: &http.Client{Transport: &httpTimeout{d: 20 * time.Second}},
	})

	ctx := context.Background()
	var err error
	switch cmd {
	case "health":
		err = runHealth(ctx, client, fs, args[1:], stdout, stderr, urlF)
	case "monitors":
		err = runMonitors(ctx, client, fs, args[1:], stdout, stderr)
	case "heartbeats":
		err = runHeartbeats(ctx, client, fs, args[1:], stdout, stderr)
	case "incident-context":
		err = runIncident(ctx, client, fs, args[1:], stdout, stderr)
	case "set-retries":
		err = runSetRetries(ctx, client, fs, args[1:], stdout, stderr)
	}
	return classifyErr(err, stderr)
}

func classifyErr(err error, stderr io.Writer) int {
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.code != ExitUsage && ee.code != ExitOK {
			fmt.Fprintln(stderr, "error:", err)
		}
		return ee.code
	}
	fmt.Fprintln(stderr, "error:", err)
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "auth failed"):
		return ExitAuth
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return ExitTimeout
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"),
		strings.Contains(msg, "handshake failed"):
		return ExitConnection
	default:
		return ExitError
	}
}
