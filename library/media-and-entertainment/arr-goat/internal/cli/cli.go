// Package cli implements the arr-goat command surface: a thin orchestrator
// over per-service "arr" engines (sonarr-goat, radarr-goat, ...).
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/client"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/config"
	"github.com/spf13/cobra"
)

// NewRoot builds the arr-goat root command.
func NewRoot(cfg *config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "arr-goat",
		Short: "Unified control plane for your self-hosted Servarr (arr) media stack",
		Long: `arr-goat is a thin control plane over the arr family: Sonarr, Radarr,
Prowlarr, Bazarr, SABnzbd, and Transmission. Each service has its own base URL
and credential; one config file + environment drives the whole fleet.

The status command probes every configured service live. Per-service health and
dispatch use the per-service engine binary (e.g. sonarr-goat-pp-cli) for deep
commands, or the built-in REST/RPC client where no engine exists.`,
	}
	root.AddCommand(newStatusCmd(cfg))
	root.AddCommand(newConfigCmd(cfg))
	root.AddCommand(newDoctorCmd(cfg))
	for _, service := range cfg.Known() {
		sc := newServiceCmd(cfg, service)
		addServiceSubcommands(sc, cfg, service)
		root.AddCommand(sc)
	}
	return root
}

// engineBinary maps a canonical service name to its per-service engine CLI.
func engineBinary(service string) string {
	return service + "-goat-pp-cli"
}

// newStatusCmd probes every configured service's health/status endpoint live.
func newStatusCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Live health/status probe of every configured service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			type svcStatus struct {
				Service string `json:"service"`
				Status  string `json:"status"`
				Detail  string `json:"detail,omitempty"`
			}
			var results []svcStatus
			w := cmd.OutOrStdout()
			for _, known := range cfg.Known() {
				s, err := cfg.Resolve(known)
				if err != nil {
					continue
				}
				key := cfg.Key(s)
				switch {
				case key == "":
					if jsonMode(cmd) {
						results = append(results, svcStatus{known, "MISSING_KEY", ""})
					} else {
						fmt.Fprintf(w, "%-12s MISSING_KEY (set ARR_GOAT_%s_KEY)\n", known, strings.ToUpper(known))
					}
					continue
				case s.BaseURL == "":
					if jsonMode(cmd) {
						results = append(results, svcStatus{known, "NO_BASE_URL", ""})
					} else {
						fmt.Fprintf(w, "%-12s NO_BASE_URL (set ARR_GOAT_%s_BASE_URL)\n", known, strings.ToUpper(known))
					}
					continue
				}
				c := client.New(s, key)
				msg, err := c.Health()
				if err != nil {
					if jsonMode(cmd) {
						results = append(results, svcStatus{known, "ERROR", err.Error()})
					} else {
						fmt.Fprintf(w, "%-12s ERROR %s\n", known, err)
					}
					continue
				}
				if jsonMode(cmd) {
					results = append(results, svcStatus{known, "OK", msg})
				} else {
					fmt.Fprintf(w, "%-12s OK    %s\n", known, msg)
				}
			}
			if jsonMode(cmd) {
				return writeJSON(cmd, results)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output as JSON")
	return cmd
}

// newServiceCmd exposes a single service command. `health` runs the in-process
// client; all other args dispatch to the per-service engine binary (when one
// exists). This keeps the whole fleet surface under one `arr-goat <service>`.
func newServiceCmd(cfg *config.Config, service string) *cobra.Command {
	return &cobra.Command{
		Use:   service + " health | " + service + " <engine-args...>",
		Short: "Live " + service + " status (health) or run a deep engine command",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := cfg.Resolve(service)
			if err != nil {
				return err
			}
			// `health` (or no args) -> in-process health probe.
			if len(args) == 0 || args[0] == "health" {
				key := cfg.Key(s)
				if key == "" {
					return fmt.Errorf("%s: no key set (ARR_GOAT_%s_KEY)", service, strings.ToUpper(service))
				}
				if s.BaseURL == "" {
					return fmt.Errorf("%s: no base url set", service)
				}
				msg, err := client.New(s, key).Health()
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), msg)
				return nil
			}

			// Anything else -> dispatch to engine binary if present.
			bin := engineBinary(service)
			enginePath, lookErr := exec.LookPath(bin)
			if lookErr != nil {
				return fmt.Errorf("%s: no engine on PATH; use 'arr-goat %s health' (full deep CLI not built for %s yet; only health/status is available)", service, service, service)
			}
			up := strings.ToUpper(service)
			env := append(os.Environ(),
				up+"_GOAT_X_API_KEY="+cfg.Key(s),
				up+"_GOAT_BASE_URL="+s.BaseURL,
				up+"_GOAT_PROTOCOL=https",
			)
			child := exec.Command(enginePath, args...)
			child.Env = env
			child.Stdin = cmd.InOrStdin()
			child.Stdout = cmd.OutOrStdout()
			child.Stderr = cmd.ErrOrStderr()
			if err := child.Run(); err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					return fmt.Errorf("%s engine exited %d", service, ee.ExitCode())
				}
				return fmt.Errorf("%s engine: %w", service, err)
			}
			return nil
		},
	}
}

// newConfigCmd prints the resolved fleet configuration and exposes `set`.
func newConfigCmd(cfg *config.Config) *cobra.Command {
	show := &cobra.Command{
		Use:   "config",
		Short: "Show the resolved arr fleet configuration; use `set` to update",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "config_path: %s\n", cfg.Path)
			fmt.Fprintf(w, "services:\n")
			for _, known := range cfg.Known() {
				s, err := cfg.Resolve(known)
				if err != nil {
					continue
				}
				key := cfg.Key(s)
				hasKey := key != ""
				var base string
				marker := ""
				envBase := os.Getenv("ARR_GOAT_" + strings.ToUpper(known) + "_BASE_URL")
				if s.BaseURL != "" {
					if envBase != "" && envBase == s.BaseURL {
						marker = " (env)"
					}
					base = s.BaseURL
				} else {
					base = "(not set)"
				}
				keyState := "MISSING"
				if hasKey {
					keyState = "configured"
				}
				fmt.Fprintf(w, "  - %-12s base_url=%-36s%s key=%s (%s)\n", known, base, marker, keyState, s.KeyEnv)
			}
			return nil
		},
	}

	set := &cobra.Command{
		Use:   "set <service>",
		Short: "Set a service's base URL and/or key env var name in the config file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.ToLower(strings.TrimSpace(args[0]))
			known := map[string]bool{}
			for _, k := range cfg.Known() {
				known[k] = true
			}
			if !known[name] {
				return fmt.Errorf("unknown service %q (known: %s)", name, strings.Join(cfg.Known(), ", "))
			}
			baseURL, _ := cmd.Flags().GetString("base-url")
			keyEnv, _ := cmd.Flags().GetString("key-env")
			if baseURL == "" && keyEnv == "" {
				return fmt.Errorf("pass at least one of --base-url <url> or --key-env <ENV_VAR>")
			}
			cfg.SetService(name, baseURL, keyEnv)
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %s in %s\n", name, cfg.Path)
			return nil
		},
	}
	set.Flags().String("base-url", "", "service base URL, e.g. https://hadm.net/tv")
	set.Flags().String("key-env", "", "env var holding the credential (e.g. SONARR_GOAT_X_API_KEY)")
	show.AddCommand(set)
	return show
}

func newDoctorCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Verify config, credentials, and engine availability for all services",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			for _, known := range cfg.Known() {
				s, err := cfg.Resolve(known)
				if err != nil {
					continue
				}
				bin := engineBinary(known)
				hasKey := cfg.Key(s) != ""
				binStatus := "not-found"
				if p, err := exec.LookPath(bin); err == nil {
					binStatus = p
				}
				fmt.Fprintf(w, "%-12s engine=%-22s key=%s\n", known, binStatus, map[bool]string{true: "configured", false: "MISSING"}[hasKey])
			}
			return nil
		},
	}
}
