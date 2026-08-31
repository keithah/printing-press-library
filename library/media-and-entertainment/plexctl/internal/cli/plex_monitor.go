package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/plexruntime/api"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/plexruntime/authstore"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/plexruntime/config"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/plexruntime/monitor"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/plexruntime/plexauth"
	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/plexctl/internal/plexruntime/pms"
	"github.com/spf13/cobra"
)

// pp:data-source live

func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) { addNovelCommandIfAbsent(root, newPlexServeCmd()) })
}

func newPlexServeCmd() *cobra.Command {
	var listen string
	var timeout time.Duration
	cmd := &cobra.Command{Use: "serve", Short: "Serve the Plex health adapter for Uptime Kuma", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h := monitor.Handler{Timeout: timeout, Resolve: resolvePlexTarget}
			s := &http.Server{Addr: listen, Handler: h, ReadHeaderTimeout: 5 * time.Second}
			fmt.Fprintf(cmd.ErrOrStderr(), "plexctl monitoring adapter listening on %s\n", listen)
			return s.ListenAndServe()
		}}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:3003", "HTTP listen address")
	cmd.Flags().DurationVar(&timeout, "timeout", 180*time.Second, "per-request health timeout")
	return cmd
}

func resolvePlexTarget(accountName, requested string) (*pms.Client, error) {
	cfg, err := config.Load(config.Path())
	if err != nil {
		return nil, err
	}
	profile, err := findPlexProfile(cfg, accountName, requested)
	if err != nil {
		return nil, err
	}
	account, ok := cfg.Accounts[accountName]
	if !ok {
		return nil, fmt.Errorf("account %q is not configured", accountName)
	}
	accountToken, err := authstore.Get(account.TokenKey)
	if err != nil {
		return nil, err
	}
	serverToken := accountToken
	if profile.TokenKey != "" {
		if t, tokenErr := authstore.Get(profile.TokenKey); tokenErr == nil && t != "" {
			serverToken = t
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resources, err := plexauth.New("https://plex.tv", "plexctl", nil).Resources(ctx, accountToken)
	if err != nil {
		return nil, fmt.Errorf("refresh Plex connections: %w", err)
	}
	var matches []plexauth.Resource
	for _, r := range resources {
		if profile.MachineIdentifier != "" && r.ClientIdentifier == profile.MachineIdentifier {
			matches = append(matches, r)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("server %q is not currently advertised by Plex.tv", requested)
	}
	conn, err := validatePlexConnection(ctx, matches[0], serverToken)
	if err != nil {
		return nil, err
	}
	n := normalizePlexConnection(conn)
	token := matches[0].AccessToken
	if token == "" {
		token = serverToken
	}
	a, err := api.New(n.URL, token, nil)
	if err != nil {
		return nil, err
	}
	a.SetInsecureTLS(n.InsecureTLS)
	return pms.New(a), nil
}

func findPlexProfile(cfg config.Config, account, requested string) (config.ServerProfile, error) {
	if p, ok := cfg.ServersV2[requested]; ok {
		if p.Account != account {
			return config.ServerProfile{}, fmt.Errorf("server %q belongs to account %q", requested, p.Account)
		}
		return p, nil
	}
	var ids []string
	for id, p := range cfg.ServersV2 {
		if p.Account == account && strings.EqualFold(p.Name, requested) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		for id, p := range cfg.ServersV2 {
			if p.Account == account && strings.EqualFold(id, requested) {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return config.ServerProfile{}, fmt.Errorf("server %q is not configured", requested)
	}
	sort.Strings(ids)
	return cfg.ServersV2[ids[0]], nil
}

type normalizedPlexConnection struct {
	URL         string
	InsecureTLS bool
}

func normalizePlexConnection(c plexauth.Connection) normalizedPlexConnection {
	u := c.URI
	insecure := false
	if !c.Local && !c.Relay {
		if strings.HasPrefix(u, "http://") {
			u = "https://" + strings.TrimPrefix(u, "http://")
		}
		if parsed, err := url.Parse(u); err == nil && parsed.Scheme == "https" {
			insecure = net.ParseIP(parsed.Hostname()) != nil
		}
	}
	return normalizedPlexConnection{URL: u, InsecureTLS: insecure}
}
func validatePlexConnection(ctx context.Context, r plexauth.Resource, accountToken string) (plexauth.Connection, error) {
	token := r.AccessToken
	if token == "" {
		token = accountToken
	}
	for _, c := range orderedPlexConnections(r.Connections) {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		n := normalizePlexConnection(c)
		a, err := api.New(n.URL, token, nil)
		if err == nil {
			a.SetInsecureTLS(n.InsecureTLS)
			identity, probeErr := pms.New(a).Identity(probeCtx)
			cancel()
			if probeErr == nil && identity.MediaContainer.MachineIdentifier == r.ClientIdentifier {
				return c, nil
			}
		} else {
			cancel()
		}
	}
	return plexauth.Connection{}, errors.New("no reachable Plex connection matched machine identifier")
}
func orderedPlexConnections(in []plexauth.Connection) []plexauth.Connection {
	var tiers [3][]plexauth.Connection
	for _, c := range in {
		if c.URI == "" {
			continue
		}
		switch {
		case c.Relay:
			tiers[2] = append(tiers[2], c)
		case c.Local:
			tiers[0] = append(tiers[0], c)
		default:
			tiers[1] = append(tiers[1], c)
		}
	}
	out := make([]plexauth.Connection, 0, len(in))
	for _, tier := range tiers {
		out = append(out, tier...)
	}
	return out
}
