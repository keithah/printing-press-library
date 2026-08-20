package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/media-and-entertainment/arr-goat/internal/config"
)

func testHealth(t *testing.T, service, body string, status int, roundtrips map[string][]byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == "transmission" {
			if r.Header.Get("X-Transmission-Session-Id") == "" {
				w.Header().Set("X-Transmission-Session-Id", "TEST-SID")
				w.WriteHeader(409)
				w.Write([]byte(`{"result":"success"}`))
				return
			}
			w.Write([]byte(`{"result":"success","arguments":{"torrentCount":5,"activeTorrentCount":2,"downloadSpeed":1024}}`))
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(config.Service{Name: service, BaseURL: srv.URL}, "testkey")
	msg, err := c.Health()
	return msg + "||" + errStr(err)
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return "ERR:" + err.Error()
}

func TestHealthSonarrSummary(t *testing.T) {
	body := `[{"source":"UpdateCheck","type":"warning","message":"update available"}]`
	got := testHealth(t, "sonarr", body, 200, nil)
	if !strings.Contains(got, "1 check") || !strings.Contains(got, "update available") {
		t.Fatalf("sonarr summary = %q", got)
	}
}

func TestHealthBazarrVersion(t *testing.T) {
	body := `{"data":{"bazarr_version":"1.6.0"}}`
	got := testHealth(t, "bazarr", body, 200, nil)
	if !strings.Contains(got, "bazarr v1.6.0") {
		t.Fatalf("bazarr summary = %q", got)
	}
}

func TestHealthSabnzbdStringDisk(t *testing.T) {
	// sabnzbd returns diskspace1 / speed as strings.
	body := `{"queue":{"version":"5.1.1","paused":false,"diskspace1":"180332.94","speed":"42.3"}}`
	got := testHealth(t, "sabnzbd", body, 200, nil)
	if !strings.Contains(got, "sabnzbd v5.1.1 running") || !strings.Contains(got, "dl 42.3") {
		t.Fatalf("sabnzbd summary = %q", got)
	}
}

func TestTransmissionHealth(t *testing.T) {
	got := testHealth(t, "transmission", "", 0, nil)
	// key is "testkey" (no colon) so user=testkey pass=""
	if !strings.Contains(got, "5 torrents") || !strings.Contains(got, "2 active") {
		t.Fatalf("transmission health = %q", got)
	}
}

func TestHealthBadStatus(t *testing.T) {
	got := testHealth(t, "prowlarr", "nope", 403, nil)
	if !strings.Contains(got, "ERR") {
		t.Fatalf("expected error on 403, got %q", got)
	}
}
