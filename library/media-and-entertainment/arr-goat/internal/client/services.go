package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Transmission RPC deep helpers ---

// rpcSession performs the CSRF handshake then executes an RPC method.
// For read calls it returns the "arguments" object; mutating calls return nil.
func (c *Client) rpc(method string, args map[string]interface{}) (map[string]interface{}, error) {
	rpcURL := c.baseURL() + "/rpc"
	sid, user, pass, err := c.transmissionSessionID()
	if err != nil {
		return nil, err
	}

	do := func(sid string) ([]byte, int, error) {
		payload, _ := json.Marshal(map[string]interface{}{"method": method, "arguments": args})
		req, _ := http.NewRequest("POST", rpcURL, strings.NewReader(string(payload)))
		req.SetBasicAuth(user, pass)
		if sid != "" {
			req.Header.Set("X-Transmission-Session-Id", sid)
		}
		return c.do(req)
	}

	body, code, err := do(sid)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("transmission %s: HTTP %d %s", method, code, strings.TrimSpace(string(body)))
	}
	var r struct {
		Result    string                 `json:"result"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("transmission %s parse: %w", method, err)
	}
	if r.Result != "success" {
		return nil, fmt.Errorf("transmission %s: %s", method, r.Result)
	}
	return r.Arguments, nil
}

// Torrent is a transmission torrent summary for display.
type Torrent struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Status       int     `json:"status"`
	PercentDone  float64 `json:"percentDone"`
	RateDownload int     `json:"rateDownload"`
	RateUpload   int     `json:"rateUpload"`
	SizeWhenDone int     `json:"sizeWhenDone"`
}

// Torrents lists torrents (optionally matching a name substring).
func (c *Client) Torrents(term string) ([]Torrent, error) {
	args, err := c.rpc("torrent-get", map[string]interface{}{
		"fields": []string{"id", "name", "status", "percentDone", "rateDownload", "rateUpload", "sizeWhenDone"},
	})
	if err != nil {
		return nil, err
	}
	var out []Torrent
	if raw, ok := args["torrents"].([]interface{}); ok {
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &out)
	}
	if term != "" {
		f := out[:0]
		for _, t := range out {
			if strings.Contains(strings.ToLower(t.Name), strings.ToLower(term)) {
				f = append(f, t)
			}
		}
		out = f
	}
	return out, nil
}

// SetTorrents applies an action (torrent-start/stop/remove) to the given ids.
// Empty ids means all torrents.
func (c *Client) SetTorrents(method string, ids []int, deleteData bool) error {
	args := map[string]interface{}{}
	if len(ids) > 0 {
		args["ids"] = ids
	}
	if method == "torrent-remove" {
		args["delete-local-data"] = deleteData
	}
	_, err := c.rpc(method, args)
	return err
}

// AddTorrent adds a torrent by magnet URI or URL.
func (c *Client) AddTorrent(magnetOrURL string, paused bool) (int, error) {
	args := map[string]interface{}{"filename": magnetOrURL}
	args["paused"] = paused
	got, err := c.rpc("torrent-add", args)
	if err != nil {
		return 0, err
	}
	// The added torrent id is nested in arguments.torrent-added.id (or torrent-duplicate).
	for _, key := range []string{"torrent-added", "torrent-duplicate"} {
		if m, ok := got[key].(map[string]interface{}); ok {
			if id, ok := m["id"].(float64); ok {
				return int(id), nil
			}
		}
	}
	return 0, nil
}

// --- SABnzbd deep helpers ---

func (c *Client) sabCall(mode string) ([]byte, error) {
	u := c.baseURL() + "/api?mode=" + url.QueryEscape(mode) + "&output=json&apikey=" + url.QueryEscape(c.Key)
	req, _ := http.NewRequest("GET", u, nil)
	body, code, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if code >= 300 {
		return nil, fmt.Errorf("sabnzbd %s: HTTP %d", mode, code)
	}
	return body, nil
}

// SABQueue is a summary of the sabnzbd download queue.
type SABQueue struct {
	Speed  string      `json:"speed"`
	Paused bool        `json:"paused"`
	MBLeft interface{} `json:"mbleft"`
	Slots  []SABSlot   `json:"slots"`
}

type SABSlot struct {
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Category string `json:"category"`
	Percent  string `json:"percentage"`
}

// Queue returns the current sabnzbd download queue.
func (c *Client) Queue() (*SABQueue, error) {
	body, err := c.sabCall("queue")
	if err != nil {
		return nil, err
	}
	var d struct {
		Queue SABQueue `json:"queue"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	return &d.Queue, nil
}

// Pause / Resume sabnzbd.
func (c *Client) SABPause(pause bool) error {
	mode := "resume"
	if pause {
		mode = "pause"
	}
	_, err := c.sabCall(mode)
	return err
}

// --- Bazarr deep helpers ---

// WantedRow is a row from bazarr's wanted/missing endpoints.
type WantedRow struct {
	Title            string `json:"title"`
	SeriesTitle      string `json:"seriesTitle"`
	EpisodeNumber    string `json:"episode_number"`
	EpisodeTitle     string `json:"episodeTitle"`
	MissingSubtitles []struct {
		Name  string `json:"name"`
		Code2 string `json:"code2"`
	} `json:"missing_subtitles"`
}

// Wanted returns bazarr's wanted (missing subtitle) rows for episodes or movies.
func (c *Client) Wanted(kind string, action string, limit int) ([]WantedRow, int, error) {
	path := "/api/" + kind + "/wanted?action=" + url.QueryEscape(action) + "&page=1&length=" + fmt.Sprint(limit)
	req, _ := http.NewRequest("GET", c.baseURL()+path, nil)
	req.Header.Set("X-API-Key", c.Key)
	body, code, err := c.do(req)
	if err != nil {
		return nil, 0, err
	}
	if code >= 300 {
		return nil, 0, fmt.Errorf("bazarr %s wanted: HTTP %d %s", kind, code, strings.TrimSpace(string(body)))
	}
	var d struct {
		Data  []WantedRow `json:"data"`
		Total int         `json:"total"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, 0, fmt.Errorf("bazarr %s wanted parse: %w", kind, err)
	}
	return d.Data, d.Total, nil
}

// BazarrStatus returns the bazarr version + sonarr/radarr versions.
type BazarrStatus struct {
	BazarrVersion string `json:"bazarr_version"`
	SonarrVersion string `json:"sonarr_version"`
	RadarrVersion string `json:"radarr_version"`
}

// Status fetches bazarr system status.
func (c *Client) BazarrStatus() (*BazarrStatus, error) {
	req, _ := http.NewRequest("GET", c.baseURL()+"/api/system/status", nil)
	req.Header.Set("X-API-Key", c.Key)
	body, code, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if code >= 300 {
		return nil, fmt.Errorf("bazarr status: HTTP %d", code)
	}
	var d struct {
		Data BazarrStatus `json:"data"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	return &d.Data, nil
}
