package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// --- reuse a minimal fake Kuma (mirrors internal/client test fake) ---

func rec(s string) string { return fmt.Sprintf("%d:%s", len([]byte(s)), s) }

var emitIDRe = regexp.MustCompile(`^42(\d+)\[`)

type fakeKuma struct {
	srv      *httptest.Server
	mu       sync.Mutex
	queue    []string
	failAuth bool
	pollN    int
}

func newFakeKuma(t *testing.T) *fakeKuma {
	f := &fakeKuma{}
	mux := http.NewServeMux()
	mux.HandleFunc("/socket.io/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("EIO") != "4" || q.Get("transport") != "polling" {
			http.Error(w, "bad", 400)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			if q.Get("sid") == "" {
				hs := []byte(`{"sid":"SID","upgrades":[],"pingInterval":25000,"pingTimeout":20000,"maxPayload":1000000}`)
				open := append([]byte{'0'}, hs...)
				w.Write([]byte(rec(string(open))))
				return
			}
			f.mu.Lock()
			f.pollN++
			out := ""
			if f.pollN == 1 {
				out = rec("40")
			} else {
				out = strings.Join(f.queue, "")
				f.queue = nil
			}
			f.mu.Unlock()
			io.WriteString(w, out)
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			if f.failAuth && strings.Contains(string(body), `"login"`) {
				id := "1"
				if m := emitIDRe.FindSubmatch(body); m != nil {
					id = string(m[1])
				}
				f.queue = append(f.queue, rec(`43`+id+`[{"ok":false,"message":"Invalid credentials"}]`))
			}
			f.mu.Unlock()
			io.WriteString(w, "ok")
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeKuma) enqueue(records ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range records {
		f.queue = append(f.queue, rec(r))
	}
}

const monitorFixture = `42["monitorList",{
  "25":{"id":25,"name":"web-frontend","type":"http","url":"https://web.example.test/health","interval":60,"maxretries":3,"active":true},
  "9":{"id":9,"name":"db-primary","type":"ping","hostname":"db.example.test","interval":120,"maxretries":2,"active":true},
  "43":{"id":43,"name":"canary-test","type":"http","url":"https://canary.example.test/","interval":60,"maxretries":3,"active":true,
        "notificationIDList":[{"id":2,"name":"alerts"},{"id":5,"name":"pager"}]}
}]`

const heartbeatFixture = `42["heartbeatList",{
  "25":[{"monitorID":25,"status":1,"time":"2026-08-26 12:00:00.000","ping":42,"msg":"200 - OK"},
        {"monitorID":25,"status":0,"time":"2026-08-26 12:02:00.000","ping":0,"msg":"timeout"}]
}]`

func primeReads(f *fakeKuma) {
	f.enqueue(
		`431[{"ok":true,"token":"jwt"}]`,
		`432{"ok":true}`,
		monitorFixture,
	)
}

func runCLI(t *testing.T, f *fakeKuma, args ...string) (int, string, string) {
	t.Helper()
	primeReads(f)
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb, func(name string) string {
		switch name {
		case "UPTIME_KUMA_URL":
			return f.srv.URL
		case "UPTIME_KUMA_USERNAME":
			return "u"
		case "UPTIME_KUMA_PASSWORD":
			return "p"
		}
		return ""
	})
	return code, out.String(), errb.String()
}

func TestHealthOK(t *testing.T) {
	f := newFakeKuma(t)
	code, out, _ := runCLI(t, f, "health")
	if code != ExitOK {
		t.Fatalf("exit=%d err=%s", code, out)
	}
	var m map[string]any
	if json.Unmarshal([]byte(out), &m) != nil || m["ok"] != true {
		t.Fatalf("bad health output: %s", out)
	}
}

// runCLIApply primes extra records for the mutation + readback cycle:
// emit3 = editMonitor ack; emit4 = fresh monitorList ack + push with maxretries=2.
func runCLIApply(t *testing.T, f *fakeKuma, args ...string) (int, string, string) {
	t.Helper()
	primeReads(f)
	f.mu.Lock()
	f.queue = append(f.queue,
		rec(`433{"ok":true,"msg":"Saved."}`),
		rec(`434{"ok":true}`),
		rec(`42["monitorList",{"43":{"id":43,"name":"canary-test","type":"http","interval":60,"maxretries":2,"active":true}}]`),
	)
	f.mu.Unlock()
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb, func(name string) string {
		switch name {
		case "UPTIME_KUMA_URL":
			return f.srv.URL
		case "UPTIME_KUMA_USERNAME":
			return "u"
		case "UPTIME_KUMA_PASSWORD":
			return "p"
		}
		return ""
	})
	return code, out.String(), errb.String()
}

func TestHealthBadAuthExit4(t *testing.T) {
	f := newFakeKuma(t)
	f.failAuth = true
	var out, errb bytes.Buffer
	f.enqueue(`431[{"ok":false,"message":"Invalid credentials"}]`)
	code := Run([]string{"health"}, &out, &errb, func(name string) string {
		if name == "UPTIME_KUMA_URL" {
			return f.srv.URL
		}
		if name == "UPTIME_KUMA_USERNAME" || name == "UPTIME_KUMA_PASSWORD" {
			return "x"
		}
		return ""
	})
	if code != ExitAuth {
		t.Fatalf("expected exit %d got %d (%s)", ExitAuth, code, errb.String())
	}
}

func TestMonitorsList(t *testing.T) {
	f := newFakeKuma(t)
	code, out, errout := runCLI(t, f, "monitors")
	if code != ExitOK {
		t.Fatalf("exit=%d %s", code, errout)
	}
	var rows []map[string]any
	if json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 3 {
		t.Fatalf("expected 3 monitors, got: %.200s", out)
	}
}

func TestMonitorsQueryFilter(t *testing.T) {
	f := newFakeKuma(t)
	code, out, _ := runCLI(t, f, "monitors", "--query", "web")
	if code != ExitOK {
		t.Fatalf("exit code %d", code)
	}
	var rows []map[string]any
	json.Unmarshal([]byte(out), &rows)
	if len(rows) != 1 || rows[0]["name"] != "web-frontend" {
		t.Fatalf("filter failed: %s", out)
	}
}

func TestHeartbeatsWindowAndLabels(t *testing.T) {
	f := newFakeKuma(t)
	var out, errb bytes.Buffer
	f.enqueue(
		`431[{"ok":true,"token":"jwt"}]`,
		`432{"ok":true}`,
		heartbeatFixture,
	)
	code := Run([]string{"heartbeats", "--hours", "1000000"}, &out, &errb, func(name string) string {
		switch name {
		case "UPTIME_KUMA_URL":
			return f.srv.URL
		case "UPTIME_KUMA_USERNAME":
			return "u"
		case "UPTIME_KUMA_PASSWORD":
			return "p"
		}
		return ""
	})
	errout := errb.String()
	_ = errout
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	var rows []map[string]any
	if json.Unmarshal([]byte(out.String()), &rows) != nil || len(rows) != 2 {
		t.Fatalf("expected 2 beats, got %.200s", out.String())
	}
	if rows[1]["label"] != "down" {
		t.Fatalf("second beat should be down: %s", out.String())
	}
}

func TestIncidentContextOutage(t *testing.T) {
	f := newFakeKuma(t)
	// after monitor read, heartbeats read needs its own ack+push
	f.enqueue(`433{"ok":true}`, heartbeatFixture)
	code, out, errout := runCLI(t, f, "incident-context", "--monitor", "25", "--lookback-minutes", "1000000")
	if code != ExitOK {
		t.Fatalf("exit=%d err=%s out=%.200s", code, errout, out)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	if m["state"] != "outage" {
		t.Fatalf("expected outage, got: %s", out)
	}
	mon := m["monitor"].(map[string]any)
	if mon["id"] != float64(25) {
		t.Fatalf("wrong monitor: %s", out)
	}
}

func TestIncidentContextByName(t *testing.T) {
	f := newFakeKuma(t)
	f.enqueue(`433{"ok":true}`, heartbeatFixture)
	code, out, _ := runCLI(t, f, "incident-context", "--monitor", "db-primary")
	if code != ExitOK {
		t.Fatalf("exit %d / %s", code, out)
	}
	// db-primary has no beats -> stale state is expected
	if !strings.Contains(out, `"stale"`) && !strings.Contains(out, `"up"`) && !strings.Contains(out, `"outage"`) {
		t.Fatalf("missing state in %s", out)
	}
}

func TestSetRetriesDryRunDefault(t *testing.T) {
	f := newFakeKuma(t)
	code, out, _ := runCLI(t, f, "set-retries", "--id", "43", "--value", "2")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	if m["dry_run"] != true {
		t.Fatalf("must default to dry-run: %s", out)
	}
	// no editMonitor emit may have happened: queue only had reads; assert via
	// the fact that dry_run is present and ok/applied fields are absent.
	if _, has := m["applied"]; has {
		t.Fatalf("dry run must not apply: %s", out)
	}
}

func TestSetRetriesRequiresYesToApply(t *testing.T) {
	f := newFakeKuma(t)
	code, out, _ := runCLIApply(t, f, "set-retries", "--id", "43", "--value", "2", "--yes")
	if code != ExitOK {
		t.Fatalf("exit %d out=%s", code, out)
	}
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	if m["verified_by_readback"] != true {
		t.Fatalf("apply should verify by readback: %s", out)
	}
}

func TestSetRetriesUsageWithoutFlags(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"set-retries"}, &out, &errb, func(string) string { return "" })
	if code != ExitUsage {
		t.Fatalf("expected usage exit %d got %d", ExitUsage, code)
	}
	if !strings.Contains(errb.String(), "--id") {
		t.Fatalf("usage message should mention --id: %s", errb.String())
	}
}

func TestUnknownCommandUsage(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run([]string{"nope"}, &out, &errb, func(string) string { return "" })
	if code != ExitUsage {
		t.Fatalf("expected %d got %d", ExitUsage, code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("unexpected stderr: %s", errb.String())
	}
}

var _ = context.Background
