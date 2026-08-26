package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	kuma "github.com/mvanhorn/printing-press-library/library/monitoring/kuma/internal/client"
)

// Monitor is the normalized subset of Kuma's monitor object the CLI surfaces.
type Monitor struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	URL        string `json:"url,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	Port       *int   `json:"port,omitempty"`
	Interval   int    `json:"interval,omitempty"`
	MaxRetries *int   `json:"maxretries,omitempty"`
	Active     bool   `json:"active"`
}

func target(m *Monitor) string {
	switch {
	case m.URL != "":
		return m.URL
	case m.Hostname != "" && m.Port != nil && *m.Port != 0:
		return fmt.Sprintf("%s:%d", m.Hostname, *m.Port)
	case m.Hostname != "":
		return m.Hostname
	default:
		return ""
	}
}

var statusLabels = map[int]string{0: "down", 1: "up", 2: "pending", 3: "maintenance"}

type beatRaw struct {
	MonitorID int64           `json:"monitorID"`
	Status    int             `json:"status"`
	Time      string          `json:"time"`
	Ping      json.RawMessage `json:"ping"`
	Msg       string          `json:"msg"`
}

func parseKumaTime(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable time %q", s)
}

func emit(stdout io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, werr := stdout.Write(append(b, '\n'))
	return werr
}

func fetchMonitors(ctx context.Context, client *kuma.Client) ([]*Monitor, []json.RawMessage, error) {
	raw, err := client.CallWithPushFallback(ctx, "getMonitorList", nil, "monitorList", 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	var byID map[string]json.RawMessage
	if err := json.Unmarshal(raw, &byID); err != nil {
		return nil, nil, fmt.Errorf("unexpected monitorList shape: %w", err)
	}
	matches := struct {
		ids []int64
		raw map[int64]json.RawMessage
	}{raw: map[int64]json.RawMessage{}}
	for k, r := range byID {
		id, cerr := strconv.ParseInt(k, 10, 64)
		if cerr != nil {
			continue
		}
		matches.ids = append(matches.ids, id)
		matches.raw[id] = r
	}
	sortInt64s(matches.ids)
	out := make([]*Monitor, 0, len(matches.ids))
	for _, id := range matches.ids {
		r := matches.raw[id]
		var m Monitor
		if json.Unmarshal(r, &m) != nil {
			continue
		}
		out = append(out, &m)
	}
	raws := make([]json.RawMessage, len(out))
	for i, id := range matches.ids {
		raws[i] = matches.raw[id]
	}
	return out, raws, nil
}

func sortInt64s(xs []int64) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func fetchHeartbeats(ctx context.Context, client *kuma.Client) (map[string][]beatRaw, error) {
	raw, err := client.CallWithPushFallback(ctx, "getHeartbeats", nil, "heartbeatList", 5*time.Second)
	if err != nil {
		return nil, err
	}
	var byMonitor map[string][]beatRaw
	if err := json.Unmarshal(raw, &byMonitor); err != nil {
		var tuple []json.RawMessage
		if json.Unmarshal(raw, &tuple) != nil || len(tuple) < 3 {
			return nil, fmt.Errorf("unexpected heartbeatList shape: %w", err)
		}
		var id string
		if json.Unmarshal(tuple[1], &id) != nil {
			var numeric int64
			if json.Unmarshal(tuple[1], &numeric) != nil {
				return nil, fmt.Errorf("unexpected heartbeat monitor id")
			}
			id = strconv.FormatInt(numeric, 10)
		}
		var beats []beatRaw
		if json.Unmarshal(tuple[2], &beats) != nil {
			return nil, fmt.Errorf("unexpected heartbeat list shape")
		}
		byMonitor = map[string][]beatRaw{id: beats}
		for i := range byMonitor[id] {
			if byMonitor[id][i].MonitorID == 0 {
				byMonitor[id][i].MonitorID, _ = strconv.ParseInt(id, 10, 64)
			}
		}
	}
	for _, extra := range client.DrainStashedEvents("heartbeatList") {
		var tuple []json.RawMessage
		if json.Unmarshal(extra, &tuple) == nil && len(tuple) >= 3 {
			var id string
			if json.Unmarshal(tuple[1], &id) != nil {
				var numeric int64
				if json.Unmarshal(tuple[1], &numeric) == nil {
					id = strconv.FormatInt(numeric, 10)
				}
			}
			var beats []beatRaw
			if id != "" && json.Unmarshal(tuple[2], &beats) == nil {
				for i := range beats {
					if beats[i].MonitorID == 0 {
						beats[i].MonitorID, _ = strconv.ParseInt(id, 10, 64)
					}
				}
				byMonitor[id] = append(byMonitor[id], beats...)
			}
		}
	}
	return byMonitor, nil
}

func runHealth(ctx context.Context, client *kuma.Client, fs *flag.FlagSet, args []string, stdout, stderr io.Writer, urlF *string) error {
	urlVal := fs.Lookup("url").Value.String()
	if err := fs.Parse(args); err != nil {
		return &exitError{ExitUsage}
	}
	if err := client.EnsureConnected(ctx); err != nil {
		return err
	}
	return emit(stdout, map[string]any{"ok": true, "authenticated": true, "url": strings.TrimRight(urlVal, "/")})
}

func runMonitors(ctx context.Context, client *kuma.Client, fs *flag.FlagSet, args []string, stdout, stderr io.Writer) error {
	query := fs.String("query", "", "substring filter on monitor name")
	if err := fs.Parse(args); err != nil {
		return &exitError{ExitUsage}
	}
	if err := client.EnsureConnected(ctx); err != nil {
		return err
	}
	monitors, _, err := fetchMonitors(ctx, client)
	if err != nil {
		return err
	}
	q := strings.ToLower(*query)
	type row struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
		Target     string `json:"target,omitempty"`
		Interval   int    `json:"interval,omitempty"`
		MaxRetries *int   `json:"max_retries,omitempty"`
		Active     bool   `json:"active"`
	}
	rows := make([]row, 0, len(monitors))
	for _, m := range monitors {
		if q != "" && !strings.Contains(strings.ToLower(m.Name), q) {
			continue
		}
		rows = append(rows, row{m.ID, m.Name, m.Type, target(m), m.Interval, m.MaxRetries, m.Active})
	}
	return emit(stdout, rows)
}

func runHeartbeats(ctx context.Context, client *kuma.Client, fs *flag.FlagSet, args []string, stdout, stderr io.Writer) error {
	hours := fs.Int("hours", 3, "lookback window in hours")
	if err := fs.Parse(args); err != nil {
		return &exitError{ExitUsage}
	}
	if err := client.EnsureConnected(ctx); err != nil {
		return err
	}
	byMonitor, err := fetchHeartbeats(ctx, client)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-time.Duration(*hours) * time.Hour)
	type beatRow struct {
		MonitorID int64  `json:"monitor_id"`
		Status    int    `json:"status"`
		Label     string `json:"label"`
		Time      string `json:"time"`
		Ping      any    `json:"ping,omitempty"`
		Msg       string `json:"msg,omitempty"`
	}
	beats := make([]beatRow, 0, 32)
	for _, list := range byMonitor {
		for _, b := range list {
			t, terr := parseKumaTime(b.Time)
			if terr != nil || t.Before(cutoff) {
				continue // cannot be placed in the requested window -> excluded
			}
			lbl, ok := statusLabels[b.Status]
			if !ok {
				lbl = "unknown"
			}
			var ping any
			if len(b.Ping) > 0 {
				json.Unmarshal(b.Ping, &ping)
			}
			beats = append(beats, beatRow{b.MonitorID, b.Status, lbl, b.Time, ping, b.Msg})
		}
	}
	sort.SliceStable(beats, func(i, j int) bool {
		if beats[i].MonitorID != beats[j].MonitorID {
			return beats[i].MonitorID < beats[j].MonitorID
		}
		return beats[i].Time < beats[j].Time
	})
	return emit(stdout, beats)
}

func runIncident(ctx context.Context, client *kuma.Client, fs *flag.FlagSet, args []string, stdout, stderr io.Writer) error {
	monitorArg := fs.String("monitor", "", "monitor id or name (required)")
	lookback := fs.Int("lookback-minutes", 60, "timeline window in minutes")
	if err := fs.Parse(args); err != nil {
		return &exitError{ExitUsage}
	}
	if strings.TrimSpace(*monitorArg) == "" {
		return usageError(stderr, "--monitor is required")
	}
	if err := client.EnsureConnected(ctx); err != nil {
		return err
	}
	monitors, _, err := fetchMonitors(ctx, client)
	if err != nil {
		return err
	}
	chosen := matchMonitor(monitors, *monitorArg)
	if chosen == nil {
		return fmt.Errorf("no monitor matches %q", *monitorArg)
	}
	byMonitor, err := fetchHeartbeats(ctx, client)
	if err != nil {
		return err
	}
	key := strconv.FormatInt(chosen.ID, 10)
	beats := byMonitor[key]

	cutoff := time.Now().Add(-time.Duration(*lookback) * time.Minute)
	type beatRow struct {
		Status int    `json:"status"`
		Label  string `json:"label"`
		Time   string `json:"time"`
		Ping   any    `json:"ping,omitempty"`
		Msg    string `json:"msg,omitempty"`
	}
	timeline := make([]beatRow, 0, len(beats))
	down, total := 0, 0
	lastIsDown := false
	var lastTime time.Time
	for _, b := range beats {
		t, terr := parseKumaTime(b.Time)
		if terr != nil || t.Before(cutoff) {
			continue
		}
		total++
		lbl, ok := statusLabels[b.Status]
		if !ok {
			lbl = "unknown"
		}
		var ping any
		if len(b.Ping) > 0 {
			json.Unmarshal(b.Ping, &ping)
		}
		timeline = append(timeline, beatRow{b.Status, lbl, b.Time, ping, b.Msg})
		if b.Status == 0 {
			down++
		}
		lastIsDown = b.Status == 0
		lastTime = t
	}
	failureRate := 0.0
	if total > 0 {
		failureRate = float64(down) / float64(total)
	}
	state := "stale"
	switch {
	case total > 0 && lastIsDown:
		state = "outage"
	case total > 0:
		state = "up"
	}
	_ = lastTime
	return emit(stdout, map[string]any{
		"state": state,
		"monitor": map[string]any{
			"id": chosen.ID, "name": chosen.Name, "type": chosen.Type, "target": target(chosen),
		},
		"failure_rate": failureRate,
		"beat_count":   total,
		"timeline":     timeline,
	})
}

func matchMonitor(monitors []*Monitor, arg string) *Monitor {
	arg = strings.TrimSpace(arg)
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		for _, m := range monitors {
			if m.ID == id {
				return m
			}
		}
		return nil
	}
	for _, m := range monitors {
		if strings.EqualFold(m.Name, arg) {
			return m
		}
	}
	lower := strings.ToLower(arg)
	for _, m := range monitors {
		if strings.Contains(strings.ToLower(m.Name), lower) {
			return m
		}
	}
	return nil
}

func runSetRetries(ctx context.Context, client *kuma.Client, fs *flag.FlagSet, args []string, stdout, stderr io.Writer) error {
	idF := fs.Int64("id", 0, "monitor id to edit (required)")
	valueF := fs.Int("value", -1, "new maxretries value (required, >= 0)")
	yesF := fs.Bool("yes", false, "apply the change (required; without it this is a dry run)")
	if err := fs.Parse(args); err != nil {
		return &exitError{ExitUsage}
	}
	if *idF == 0 || *valueF < 0 {
		return usageError(stderr, "--id and --value (>=0) are required")
	}
	if err := client.EnsureConnected(ctx); err != nil {
		return err
	}
	monitors, raws, err := fetchMonitors(ctx, client)
	if err != nil {
		return err
	}
	idx := -1
	for i, m := range monitors {
		if m.ID == *idF {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("no monitor with id %d", *idF)
	}
	chosen, fullRaw := monitors[idx], raws[idx]
	current := -1
	if chosen.MaxRetries != nil {
		current = *chosen.MaxRetries
	}
	if !*yesF {
		return emit(stdout, map[string]any{
			"dry_run":   true,
			"id":        chosen.ID,
			"name":      chosen.Name,
			"current":   current,
			"would_set": *valueF,
			"hint":      "re-run with --yes to apply",
		})
	}
	if current == *valueF {
		return emit(stdout, map[string]any{"ok": true, "unchanged": true, "id": chosen.ID, "value": *valueF})
	}
	// Kuma v2 editMonitor is a FULL REPLACE: it copies only fields present on
	// the payload, so we send the complete live object with one field changed.
	var obj map[string]any
	if err := json.Unmarshal(fullRaw, &obj); err != nil {
		return fmt.Errorf("cannot prepare full-object edit payload: %w", err)
	}
	obj["maxretries"] = *valueF
	if list, has := obj["notificationIDList"]; has {
		if arr, isArr := list.([]any); isArr {
			mapped := map[string]bool{}
			for _, item := range arr {
				switch v := item.(type) {
				case float64:
					mapped[strconv.Itoa(int(v))] = true
				case string:
					mapped[v] = true
				}
			}
			obj["notificationIDList"] = mapped
		}
	}
	ack, err := client.CallRaw(ctx, "editMonitor", obj)
	if err != nil {
		return err
	}
	if err := checkAckOK(ack); err != nil {
		return err
	}
	applied := false
	for attempt := 0; attempt < 5 && !applied; attempt++ {
		monitors2, _, fetchErr := fetchMonitors(ctx, client)
		if fetchErr != nil {
			return fetchErr
		}
		for _, m := range monitors2 {
			if m.ID == *idF && m.MaxRetries != nil && *m.MaxRetries == *valueF {
				applied = true
			}
		}
		if !applied && attempt < 4 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return emit(stdout, map[string]any{
		"ok":                   applied,
		"id":                   chosen.ID,
		"name":                 chosen.Name,
		"previous":             current,
		"applied":              *valueF,
		"verified_by_readback": applied,
	})
}

func checkAckOK(ack []byte) error {
	var arr []map[string]json.RawMessage
	var obj map[string]json.RawMessage
	if json.Unmarshal(ack, &arr) == nil && len(arr) > 0 && arr[0] != nil {
		obj = arr[0]
	} else if json.Unmarshal(ack, &obj) != nil {
		return fmt.Errorf("unparseable edit ack")
	}
	okRaw, has := obj["ok"]
	if !has {
		return fmt.Errorf("edit ack missing ok")
	}
	var ok bool
	json.Unmarshal(okRaw, &ok)
	if !ok {
		msg := ""
		if m, hasMsg := obj["message"]; hasMsg {
			json.Unmarshal(m, &msg)
		}
		return fmt.Errorf("edit rejected: %s", msg)
	}
	return nil
}

func usageError(stderr io.Writer, msg string) error {
	fmt.Fprintln(stderr, msg)
	return &exitError{ExitUsage}
}
