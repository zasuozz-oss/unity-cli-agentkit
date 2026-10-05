package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// telemetryEntry is one lock-wait/hold record. unity-job.sh writes the same
// shape (via "job"); the coordinator writes via "queue". Both land in one
// file so `utk queue stats` sees the whole Editor, not one path through it.
type telemetryEntry struct {
	TS     string `json:"ts"`
	Owner  string `json:"owner"`
	Task   string `json:"task"`
	Kind   string `json:"kind"`
	WaitS  int    `json:"wait_s"`
	HoldS  int    `json:"hold_s"`
	Result string `json:"result"`
	Via    string `json:"via"`
}

// kitHome follows initcmd so a non-default install (UNITY_CLI_AGENTKIT_HOME,
// binary-relative home) finds its own skills scripts instead of exiting 127.
// kitHome is where queue state and telemetry live. It is the shell scripts'
// $HOME/.unity-cli-agentkit on purpose, not the binary-relative KitHome: two
// utk builds on one machine must share one queue, one lock and one telemetry
// file, or they coordinate nothing.
func kitHome() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".unity-cli-agentkit")
}

func telemetryPath() string {
	if p := os.Getenv("UTK_TELEMETRY"); p != "" {
		return p
	}
	return filepath.Join(kitHome(), "telemetry.jsonl")
}

func appendTelemetry(e telemetryEntry) error {
	p := telemetryPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, err = f.Write(append(b, '\n'))
	return err
}

func percentile(xs []int, p float64) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	// Nearest-rank: flooring (n-1)*p made p95 of two samples return the minimum.
	i := int(math.Ceil(p*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	if i > len(s)-1 {
		i = len(s) - 1
	}
	return s[i]
}

// telemetryStats renders per-kind wait/hold percentiles since a point in time.
// Unparseable lines are skipped: a half-written line from a killed job must
// not hide a week of good ones.
func telemetryStats(r io.Reader, since time.Time) string {
	type agg struct{ wait, hold []int }
	kinds := map[string]*agg{}
	gate := 0
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var e telemetryEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.TS)
		if err != nil || ts.Before(since) {
			continue
		}
		if e.Result == "GATE_FAILED" {
			gate++
			continue
		}
		a := kinds[e.Kind]
		if a == nil {
			a = &agg{}
			kinds[e.Kind] = a
		}
		a.wait = append(a.wait, e.WaitS)
		a.hold = append(a.hold, e.HoldS)
	}
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "since %s\n", since.Format(time.RFC3339))
	for _, k := range names {
		a := kinds[k]
		fmt.Fprintf(&b, "%-8s n=%d  wait p50=%ds p95=%ds  hold p50=%ds p95=%ds\n", k, len(a.wait),
			percentile(a.wait, .5), percentile(a.wait, .95), percentile(a.hold, .5), percentile(a.hold, .95))
	}
	fmt.Fprintf(&b, "gate blocked: %d\n", gate)
	if err := sc.Err(); err != nil {
		fmt.Fprintf(&b, "read error: %v\n", err)
	}
	return b.String()
}
