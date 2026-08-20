// Package client implements in-process HTTP/RPC clients for each arr service.
// arr-goat uses these directly (no separate engine binaries) so a single
// binary carries the whole fleet's functionality.
package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/config"
)

// Client talks to one arr service.
type Client struct {
	Service config.Service
	Key     string
	HTTP    *http.Client
}

// New builds a client for the resolved service + key.
func New(s config.Service, key string) *Client {
	return &Client{
		Service: s,
		Key:     key,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// baseURL returns the service root, trimming trailing slash.
func (c *Client) baseURL() string {
	return strings.TrimRight(c.Service.BaseURL, "/")
}

func (c *Client) do(req *http.Request) ([]byte, int, error) {
	body, code, _, err := c.doFull(req)
	return body, code, err
}

// doFull performs the request and returns body, status code, and the response
// (needed to read headers, e.g. transmission's CSRF session id).
func (c *Client) doFull(req *http.Request) ([]byte, int, *http.Response, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return body, resp.StatusCode, resp, nil
}

func (c *Client) get(path string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", c.baseURL()+path, nil)
	if err != nil {
		return nil, 0, err
	}
	if c.Service.AuthHeader() != "" && c.Key != "" {
		req.Header.Set(c.Service.AuthHeader(), c.Key)
	}
	return c.do(req)
}

// Health hits the service health/status endpoint and returns a short summary.
func (c *Client) Health() (string, error) {
	// RPC services have no REST probe and are handled specially.
	if c.Service.Name == "transmission" {
		return c.transmissionHealth()
	}
	probe, ok := probes[c.Service.Name]
	if !ok {
		return "", fmt.Errorf("no health probe for %q", c.Service.Name)
	}
	path := probe.path
	keyParam := c.Service.Name == "sabnzbd"
	if keyParam {
		path = strings.ReplaceAll(path, "{key}", url.QueryEscape(c.Key))
	}
	body, code, err := c.get(path)
	if err != nil {
		return "", err
	}
	if code >= 300 {
		return "", fmt.Errorf("%s: HTTP %d %s", c.Service.Name, code, strings.TrimSpace(string(body)))
	}
	// sabnzbd returns {"status":false,"error":"..."} on errors despite HTTP 200.
	if c.Service.Name == "sabnzbd" {
		var sabErr struct {
			Status *bool  `json:"status"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(body, &sabErr); err == nil && sabErr.Status != nil && !*sabErr.Status {
			return "", fmt.Errorf("%s: sabnzbd error: %s", c.Service.Name, sabErr.Error)
		}
	}
	return fmt.Sprintf("%s: %s", c.Service.Name, summarizeServiceHealth(c.Service.Name, body)), nil
}

// summarizeServiceHealth renders a compact one-line human summary per service.
func summarizeServiceHealth(service string, body []byte) string {
	switch service {
	case "bazarr":
		var d struct {
			Data struct {
				Version string `json:"bazarr_version"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &d) == nil && d.Data.Version != "" {
			return fmt.Sprintf("bazarr v%s OK", d.Data.Version)
		}
	case "sabnzbd":
		var d struct {
			Queue struct {
				Version    string      `json:"version"`
				Paused     bool        `json:"paused"`
				Diskspace1 interface{} `json:"diskspace1"`
				Speed      string      `json:"speed"`
			} `json:"queue"`
		}
		if json.Unmarshal(body, &d) == nil && d.Queue.Version != "" {
			state := "running"
			if d.Queue.Paused {
				state = "paused"
			}
			disk := genericToFloat(d.Queue.Diskspace1)
			return fmt.Sprintf("sabnzbd v%s %s (dl %s, ~%.0f GB disk free)", d.Queue.Version, state, d.Queue.Speed, disk/1024)
		}
	case "sonarr", "radarr", "prowlarr":
		var arr []struct {
			Source  string `json:"source"`
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &arr) == nil {
			if len(arr) == 0 {
				return "all checks OK"
			}
			issues := []string{}
			for _, it := range arr {
				issues = append(issues, fmt.Sprintf("%s(%s): %s", it.Type, it.Source, it.Message))
			}
			return fmt.Sprintf("%d check(s): %s", len(issues), strings.Join(issues, "; "))
		}
	}
	return summarizeJSON(body)
}

func summarizeJSON(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 140 {
		return s[:140] + "…"
	}
	if s == "" {
		return "empty"
	}
	return s
}

// --- transmission RPC ---

// transmissionSessionID performs the CSRF handshake against the transmission
// RPC endpoint and returns the session id, plus the resolved basic-auth pair.
// Transmission returns HTTP 409 with the id in the X-Transmission-Session-Id
// RESPONSE HEADER (not the body). Implemented once and shared by health + rpc.
func (c *Client) transmissionSessionID() (sid string, user, pass string, err error) {
	rpcURL := c.baseURL() + "/rpc"
	user, pass = splitBasicAuth(c.Key)
	req, _ := http.NewRequest("POST", rpcURL, strings.NewReader(`{}`))
	req.SetBasicAuth(user, pass)
	body, code, resp, err := c.doFull(req)
	if err != nil {
		return "", "", "", err
	}
	if code != 409 {
		return "", "", "", fmt.Errorf("transmission handshake: HTTP %d %s", code, strings.TrimSpace(string(body)))
	}
	sid = resp.Header.Get("X-Transmission-Session-Id")
	if sid == "" {
		return "", "", "", fmt.Errorf("transmission handshake: no session id in 409")
	}
	return sid, user, pass, nil
}

// transmissionHealth performs the RPC handshake (session-id) then requests
// session stats to confirm the daemon is alive.
func (c *Client) transmissionHealth() (string, error) {
	rpcURL := c.baseURL() + "/rpc"
	sid, user, pass, err := c.transmissionSessionID()
	if err != nil {
		return "", err
	}
	setCSRF := func(r *http.Request) {
		r.Header.Set("X-Transmission-Session-Id", sid)
	}
	// Step 2: authenticated session-stats call
	req2, _ := http.NewRequest("POST", rpcURL, strings.NewReader(`{"method":"session-stats","arguments":{}}`))
	req2.SetBasicAuth(user, pass)
	setCSRF(req2)
	body, code, err := c.do(req2)
	if err != nil {
		return "", err
	}
	if code >= 300 {
		return "", fmt.Errorf("transmission session-stats: HTTP %d %s", code, strings.TrimSpace(string(body)))
	}
	var r struct {
		Result    string `json:"result"`
		Arguments struct {
			ActiveTorrentCount int `json:"activeTorrentCount"`
			TorrentCount       int `json:"torrentCount"`
			DownloadSpeed      int `json:"downloadSpeed"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("transmission parse: %w", err)
	}
	return fmt.Sprintf("transmission: %s (%d torrents, %d active, dl %d B/s)", r.Result, r.Arguments.TorrentCount, r.Arguments.ActiveTorrentCount, r.Arguments.DownloadSpeed), nil
}

// splitBasicAuth splits a "user:pass" credential into user and password.
func splitBasicAuth(s string) (string, string) {
	if i := strings.Index(s, ":"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// genericToFloat converts an interface{} (number or numeric string) to float64.
func genericToFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	default:
		return 0
	}
}
