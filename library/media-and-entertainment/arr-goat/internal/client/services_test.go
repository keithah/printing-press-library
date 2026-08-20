package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/config"
)

// mockClient spins an httptest server that serves a canned payload at path.
func mockClient(t *testing.T, service, path string, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rpc") || path == "rpc" {
			if r.Header.Get("X-Transmission-Session-Id") == "" {
				w.Header().Set("X-Transmission-Session-Id", "SID")
				w.WriteHeader(409)
				w.Write([]byte(`{}`))
				return
			}
			w.Write([]byte(body))
			return
		}
		if status != 0 {
			w.WriteHeader(status)
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(config.Service{Name: service, BaseURL: srv.URL}, "u:p")
}

func TestTorrentsListAndFilter(t *testing.T) {
	c := mockClient(t, "transmission", "rpc", 0, `{"result":"success","arguments":{"torrents":[
		{"id":1,"name":"Alpha Show S01","status":6,"percentDone":1},
		{"id":2,"name":"Beta Movie","status":4,"percentDone":0.25}
	]}}`)
	all, err := c.Torrents("")
	if err != nil || len(all) != 2 {
		t.Fatalf("all: len=%d err=%v", len(all), err)
	}
	beta, err := c.Torrents("beta")
	if err != nil || len(beta) != 1 || beta[0].Name != "Beta Movie" {
		t.Fatalf("filter: %+v err=%v", beta, err)
	}
}

func TestAddTorrentReturnsID(t *testing.T) {
	c := mockClient(t, "transmission", "rpc", 0, `{"result":"success","arguments":{"torrent-added":{"id":42,"name":"x"}}}`)
	id, err := c.AddTorrent("magnet:?xt=abc", false)
	if err != nil || id != 42 {
		t.Fatalf("add: id=%d err=%v", id, err)
	}
}

func TestSABQueueParsing(t *testing.T) {
	c := mockClient(t, "sabnzbd", "/api", 200, `{"queue":{"speed":"0","paused":false,"mbleft":"1.2","slots":[
		{"filename":"Show.S01E01","status":"Downloading","percentage":"33"}]}}`)
	q, err := c.Queue()
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Slots) != 1 || q.Slots[0].Status != "Downloading" {
		t.Fatalf("queue slots: %+v", q.Slots)
	}
}

func TestBazarrWanted(t *testing.T) {
	c := mockClient(t, "bazarr", "/api/episodes/wanted", 200, `{"data":[
		{"seriesTitle":"Show","episode_number":"1x2","episodeTitle":"Ep",
		 "missing_subtitles":[{"name":"Spanish","code2":"es"}]}],"total":1}`)
	rows, total, err := c.Wanted("episodes", "missing", 20)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("wanted: total=%d rows=%d err=%v", total, len(rows), err)
	}
	if rows[0].SeriesTitle != "Show" || len(rows[0].MissingSubtitles) != 1 {
		t.Fatalf("row: %+v", rows[0])
	}
}

func TestBazarrStatus(t *testing.T) {
	c := mockClient(t, "bazarr", "/api/system/status", 200, `{"data":{"bazarr_version":"1.6.0","sonarr_version":"4.0","radarr_version":"6.2"}}`)
	st, err := c.BazarrStatus()
	if err != nil || st.BazarrVersion != "1.6.0" {
		t.Fatalf("status: %+v err=%v", st, err)
	}
}
