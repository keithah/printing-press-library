package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/client"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/config"
	"github.com/spf13/cobra"
)

// serviceClient resolves a service client from the shared config.
func serviceClient(cfg *config.Config, name string) (*client.Client, error) {
	s, err := cfg.Resolve(name)
	if err != nil {
		return nil, err
	}
	if cfg.Key(s) == "" {
		return nil, fmt.Errorf("%s: no key set", name)
	}
	return client.New(s, cfg.Key(s)), nil
}

// addServiceSubcommands attaches the per-service deep command surfaces as
// children of the given service command (e.g. `arr-goat transmission torrents`).
func addServiceSubcommands(c *cobra.Command, cfg *config.Config, service string) {
	switch service {
	case "transmission":
		addTransmissionCommands(c, cfg)
	case "sabnzbd":
		addSABCommands(c, cfg)
	case "bazarr":
		addBazarrCommands(c, cfg)
	}
}

func addTransmissionCommands(c *cobra.Command, cfg *config.Config) {
	torrents := &cobra.Command{
		Use:   "torrents [search]",
		Short: "List torrents (optionally filtered by name)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := serviceClient(cfg, "transmission")
			if err != nil {
				return err
			}
			term := ""
			if len(args) > 0 {
				term = args[0]
			}
			return printTorrents(cmd, cc, term)
		},
	}
	torrents.Flags().Bool("json", false, "output as JSON")
	c.AddCommand(torrents)
	c.AddCommand(&cobra.Command{
		Use:   "add <magnet-or-url>",
		Short: "Add a torrent by magnet URI or URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, _ := serviceClient(cfg, "transmission")
			id, err := cc.AddTorrent(args[0], false)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added torrent id=%d\n", id)
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "start <ids...|all>",
		Short: "Start torrent(s) by id (or all)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, _ := serviceClient(cfg, "transmission")
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			return cc.SetTorrents("torrent-start", ids, false)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "stop <ids...|all>",
		Short: "Stop torrent(s) by id (or all)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, _ := serviceClient(cfg, "transmission")
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			return cc.SetTorrents("torrent-stop", ids, false)
		},
	})
	removeCmd := &cobra.Command{
		Use:   "remove <ids...|all>",
		Short: "Remove torrent(s) by id (or all); --delete also deletes local data",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, _ := serviceClient(cfg, "transmission")
			del, _ := cmd.Flags().GetBool("delete")
			yes, _ := cmd.Flags().GetBool("yes")
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			// "all" (nil ids) is destructive: require explicit --yes.
			if len(ids) == 0 && !yes {
				return fmt.Errorf("removing ALL torrents is destructive; re-run with --yes to confirm")
			}
			return cc.SetTorrents("torrent-remove", ids, del)
		},
	}
	removeCmd.Flags().Bool("delete", false, "also delete local data")
	removeCmd.Flags().Bool("yes", false, "confirm removing ALL torrents")
	c.AddCommand(removeCmd)
}

func printTorrents(cmd *cobra.Command, cc *client.Client, term string) error {
	list, err := cc.Torrents(term)
	if err != nil {
		return err
	}
	if jsonMode(cmd) {
		return writeJSON(cmd, list)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "torrents: %d\n", len(list))
	for _, t := range list {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-5d %-5.0f%% %-16s %s\n", t.ID, t.PercentDone*100, transStatus(t.Status), truncate(t.Name, 60))
	}
	return nil
}

// jsonMode reports whether the command was invoked with --json.
func jsonMode(cmd *cobra.Command) bool {
	if cmd.Flags().Lookup("json") != nil {
		v, _ := cmd.Flags().GetBool("json")
		return v
	}
	return false
}

// writeJSON marshals v to the command's stdout.
func writeJSON(cmd *cobra.Command, v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return nil
}

func transStatus(s int) string {
	switch s {
	case 0:
		return "stopped"
	case 1:
		return "check-wait"
	case 2:
		return "checking"
	case 3:
		return "download-wait"
	case 4:
		return "downloading"
	case 5:
		return "seed-wait"
	case 6:
		return "seeding"
	case 7:
		return "isolated"
	default:
		return "?"
	}
}

// parseIDs parses a torrent id list; "all" (or empty) means every torrent.
func parseIDs(args []string) ([]int, error) {
	ids := make([]int, 0, len(args))
	for _, a := range args {
		if strings.EqualFold(a, "all") {
			return nil, nil
		}
		n, err := strconv.Atoi(a)
		if err != nil {
			return nil, fmt.Errorf("bad torrent id %q", a)
		}
		ids = append(ids, n)
	}
	return ids, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func addSABCommands(c *cobra.Command, cfg *config.Config) {
	queue := &cobra.Command{
		Use:   "queue",
		Short: "Show the SABnzbd download queue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := serviceClient(cfg, "sabnzbd")
			if err != nil {
				return err
			}
			return printQueue(cmd, cc)
		},
	}
	queue.Flags().Bool("json", false, "output as JSON")
	c.AddCommand(queue)
	c.AddCommand(&cobra.Command{
		Use:   "pause",
		Short: "Pause the SABnzbd queue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := serviceClient(cfg, "sabnzbd")
			if err != nil {
				return err
			}
			return cc.SABPause(true)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "resume",
		Short: "Resume the SABnzbd queue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := serviceClient(cfg, "sabnzbd")
			if err != nil {
				return err
			}
			return cc.SABPause(false)
		},
	})
}

func printQueue(cmd *cobra.Command, cc *client.Client) error {
	q, err := cc.Queue()
	if err != nil {
		return err
	}
	if jsonMode(cmd) {
		return writeJSON(cmd, q)
	}
	state := "running"
	if q.Paused {
		state = "paused"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "sabnzbd: %s | speed %s | %d queued\n", state, q.Speed, len(q.Slots))
	for _, s := range q.Slots {
		fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s (%s)\n", s.Status, truncate(s.Filename, 55), s.Percent)
	}
	return nil
}

func addBazarrCommands(c *cobra.Command, cfg *config.Config) {
	status := &cobra.Command{
		Use:   "status",
		Short: "Show Bazarr + Sonarr/Radarr versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := serviceClient(cfg, "bazarr")
			if err != nil {
				return err
			}
			st, err := cc.BazarrStatus()
			if err != nil {
				return err
			}
			if jsonMode(cmd) {
				return writeJSON(cmd, st)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "bazarr v%s (sonarr %s / radarr %s)\n", st.BazarrVersion, st.SonarrVersion, st.RadarrVersion)
			return nil
		},
	}
	status.Flags().Bool("json", false, "output as JSON")
	c.AddCommand(status)

	wanted := &cobra.Command{
		Use:   "wanted [episodes|movies]",
		Short: "List wanted (missing subtitle) items",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := serviceClient(cfg, "bazarr")
			if err != nil {
				return err
			}
			kind := "episodes"
			if len(args) > 0 && strings.EqualFold(args[0], "movies") {
				kind = "movies"
			}
			rows, total, err := cc.Wanted(kind, "missing", 20)
			if err != nil {
				return err
			}
			if jsonMode(cmd) {
				return writeJSON(cmd, map[string]interface{}{"kind": kind, "total": total, "items": rows})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "bazarr %s wanted: %d total (showing %d)\n", kind, total, len(rows))
			for _, r := range rows {
				subl := ""
				if len(r.MissingSubtitles) > 0 {
					langs := make([]string, 0, len(r.MissingSubtitles))
					for _, m := range r.MissingSubtitles {
						langs = append(langs, m.Name)
					}
					subl = " [" + strings.Join(langs, ", ") + "]"
				}
				if r.SeriesTitle != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s %s - %s%s\n", r.SeriesTitle, r.EpisodeNumber, r.EpisodeTitle, subl)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s%s\n", r.Title, subl)
				}
			}
			return nil
		},
	}
	wanted.Flags().Bool("json", false, "output as JSON")
	c.AddCommand(wanted)
}
