package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// queueArgs is the per-kind payload of a request (see the spec, §5).
type queueArgs struct {
	Filter   string   `json:"filter,omitempty"`    // test: testName filter, "A;B" allowed
	Mode     string   `json:"mode,omitempty"`      // test: editmode|playmode
	Cmd      []string `json:"cmd,omitempty"`       // scene: utk argv
	Script   string   `json:"script,omitempty"`    // scene|shot: bash script path
	EstS     int      `json:"est_s,omitempty"`     // scene: shortest-job-first key
	TimeoutS int      `json:"timeout_s,omitempty"` // scene|shot: wall clock per job
	Scene    string   `json:"scene,omitempty"`     // shot: scene path to (re)load before the script
	Isolated bool     `json:"isolated,omitempty"`  // shot: own Play session
	Edit     bool     `json:"edit,omitempty"`      // shot: no Play at all
}

type queueRequest struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Agent     string    `json:"agent"`
	PID       int       `json:"pid"`
	Cwd       string    `json:"cwd"`
	Submitted time.Time `json:"submitted"`
	Args      queueArgs `json:"args"`
}

type queueResult struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Exit      int      `json:"exit"`
	Stdout    string   `json:"stdout"`
	Artifacts []string `json:"artifacts,omitempty"`
	WaitS     int      `json:"wait_s"`
	RunS      int      `json:"run_s"`
}

// queueDir is one directory per Editor, named like the lock so a worktree
// Editor with its own UNITY_LOCK_NAME gets its own queue.
func queueDir() string {
	if d := os.Getenv("UTK_QUEUE_DIR"); d != "" {
		return d
	}
	name := os.Getenv("UNITY_LOCK_NAME")
	if name == "" {
		name = "unity-editor"
	}
	return filepath.Join(kitHome(), "queue", name)
}

func newRequestID(now time.Time) string {
	var b [2]byte
	rand.Read(b[:])
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

// writeJSONAtomic lands the file whole or not at all: the coordinator lists
// the directory while agents write into it.
func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeRequest(dir string, r queueRequest) error {
	return writeJSONAtomic(filepath.Join(dir, "requests", r.ID+".json"), r)
}

func writeResult(dir string, r queueResult) error {
	return writeJSONAtomic(filepath.Join(dir, "results", r.ID+".json"), r)
}

func readResult(dir, id string) (queueResult, bool) {
	var r queueResult
	b, err := os.ReadFile(filepath.Join(dir, "results", id+".json"))
	if err != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	return r, true
}

// readRequests returns pending requests oldest first and drops the ones whose
// submitter is gone — nobody is waiting for that result, so it must not cost
// Editor time (unity-lock.sh drops dead tickets the same way).
func readRequests(dir string) ([]queueRequest, error) {
	ents, err := os.ReadDir(filepath.Join(dir, "requests"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []queueRequest
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, "requests", e.Name())
		b, err := os.ReadFile(p)
		var r queueRequest
		if err != nil || json.Unmarshal(b, &r) != nil {
			continue
		}
		if !pidAlive(r.PID) {
			os.Remove(p)
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func removeRequest(dir, id string) { os.Remove(filepath.Join(dir, "requests", id+".json")) }

// parseSubmit turns `utk queue submit <kind> [flags]` into a request. It
// refuses what the Editor would refuse later, so a bad request never waits
// for a cycle to learn it was bad.
func parseSubmit(args []string, now time.Time) (queueRequest, int, error) {
	if len(args) == 0 {
		return queueRequest{}, 0, errors.New("usage: utk queue submit compile|test|scene|shot [flags]")
	}
	r := queueRequest{ID: newRequestID(now), Kind: args[0], PID: os.Getpid(), Submitted: now,
		Args: queueArgs{Mode: "editmode", EstS: 60}}
	r.Cwd, _ = os.Getwd()
	if p := localProjectRoot(); p != "" {
		r.Cwd = p
	}
	r.Agent = os.Getenv("UNITY_LOCK_OWNER")
	if r.Agent == "" {
		r.Agent = "pid-" + strconv.Itoa(r.PID)
	}
	wait := 3600
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		next := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("%s needs a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		var v string
		var err error
		switch rest[i] {
		case "--filter":
			v, err = next()
			r.Args.Filter = v
		case "--mode":
			v, err = next()
			r.Args.Mode = v
		case "--cmd":
			v, err = next()
			r.Args.Cmd = strings.Fields(v)
		case "--script":
			// The coordinator runs it from the project root, not this cwd.
			if v, err = next(); err == nil {
				r.Args.Script, err = filepath.Abs(v)
			}
		case "--scene":
			v, err = next()
			r.Args.Scene = v
		case "--agent":
			v, err = next()
			r.Agent = v
		case "--est-seconds":
			r.Args.EstS, err = intFlag(rest[i], next)
		case "--timeout":
			r.Args.TimeoutS, err = intFlag(rest[i], next)
		case "--wait":
			wait, err = intFlag(rest[i], next)
		case "--isolated":
			r.Args.Isolated = true
		case "--edit":
			r.Args.Edit = true
		default:
			err = fmt.Errorf("unknown flag %s", rest[i])
		}
		if err != nil {
			return r, 0, err
		}
	}
	if r.Args.TimeoutS <= 0 {
		// A test class gets more room than a scene command; either way the
		// Editor-side run is stopped when it runs out (queue_watchdog.go).
		r.Args.TimeoutS = 300
		if r.Kind == "test" {
			r.Args.TimeoutS = 600
		}
	}
	switch r.Kind {
	case "compile":
	case "test":
		if r.Args.Filter == "" {
			return r, 0, errors.New("test needs --filter <TestClass>")
		}
		if why := wholeSuite([]string{"--filter", r.Args.Filter}); why != "" {
			return r, 0, errors.New(why)
		}
		if r.Args.Mode != "editmode" && r.Args.Mode != "playmode" {
			return r, 0, errors.New("--mode must be editmode or playmode")
		}
	case "scene":
		if len(r.Args.Cmd) == 0 && r.Args.Script == "" {
			return r, 0, errors.New("scene needs --cmd '<utk args>' or --script <file>")
		}
	case "shot":
		if r.Args.Script == "" || (r.Args.Scene == "" && !r.Args.Edit) {
			return r, 0, errors.New("shot needs --script <file> and --scene <path> (or --edit)")
		}
	default:
		return r, 0, fmt.Errorf("unknown kind %q (compile|test|scene|shot)", r.Kind)
	}
	return r, wait, nil
}

// intFlag reads a seconds value. `--wait 2h` used to become 0 and withdraw at
// once; `--timeout 10m` silently became the 300 s default.
func intFlag(name string, next func() (string, error)) (int, error) {
	v, err := next()
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", name, v)
	}
	return n, nil
}

// runQueue dispatches `utk queue <sub>`.
func runQueue(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: utk queue submit|status|stats|cancel|serve")
		return 2
	}
	dir := queueDir()
	switch args[0] {
	case "submit":
		return runSubmit(dir, args[1:], stdout, stderr)
	case "status":
		reqs, err := readRequests(dir)
		if err != nil {
			fmt.Fprintln(stderr, "utk queue:", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s: %d pending\n", dir, len(reqs))
		if n := hungNotice(dir); n != "" {
			fmt.Fprintln(stdout, "HUNG:", n)
		}
		if b, err := os.ReadFile(filepath.Join(dir, "running")); err == nil {
			fmt.Fprint(stdout, "running: ", string(b))
		}
		if owner, pid := lockHolder(); owner != "" {
			state := ""
			if pid > 0 && !pidAlive(pid) {
				state = " (holder gone; the next acquire frees it)"
			}
			fmt.Fprintf(stdout, "editor lock: %s pid %d%s\n", owner, pid, state)
		}
		for _, r := range reqs {
			fmt.Fprintf(stdout, "%s %-7s %s %s %s\n", r.ID, r.Agent, r.Kind, describeArgs(r), r.Submitted.Format("15:04:05"))
		}
		return 0
	case "stats":
		since := time.Now().Add(-24 * time.Hour)
		if v := findFlag(args[1:], "--since"); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				since = time.Now().Add(-d)
			}
		}
		f, err := os.Open(telemetryPath())
		if err != nil {
			fmt.Fprintln(stderr, "utk queue stats: no telemetry yet at", telemetryPath())
			return 1
		}
		defer f.Close()
		fmt.Fprint(stdout, telemetryStats(f, since))
		return 0
	case "serve":
		return runServe(dir, stdout, stderr)
	case "cancel":
		// Removing the request is the whole protocol: a queued one is never
		// picked up, a running one is stopped by its job's gone() check, and
		// the submitter sees its request vanish.
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: utk queue cancel <id>   (ids: utk queue status)")
			return 2
		}
		if err := os.Remove(filepath.Join(dir, "requests", args[1]+".json")); err != nil {
			fmt.Fprintf(stderr, "utk queue cancel: no request %s (see utk queue status)\n", args[1])
			return 1
		}
		fmt.Fprintln(stdout, "cancelled", args[1])
		return 0
	}
	fmt.Fprintln(stderr, "usage: utk queue submit|status|stats|cancel|serve")
	return 2
}

func describeArgs(r queueRequest) string {
	switch r.Kind {
	case "test":
		return r.Args.Filter
	case "scene":
		if r.Args.Script != "" {
			return r.Args.Script
		}
		return strings.Join(r.Args.Cmd, " ")
	case "shot":
		return r.Args.Scene + " " + r.Args.Script
	}
	return ""
}

var submitPoll = time.Second

// runSubmit writes the request, makes sure a coordinator is up, and blocks
// until its own result lands (or --wait runs out).
func runSubmit(dir string, args []string, stdout, stderr io.Writer) int {
	r, wait, err := parseSubmit(args, time.Now())
	if err != nil {
		fmt.Fprintln(stderr, "utk queue submit:", err)
		return 2
	}
	if err := writeRequest(dir, r); err != nil {
		fmt.Fprintln(stderr, "utk queue submit:", err)
		return 1
	}
	ensureServe(dir, stderr)
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for i := 1; time.Now().Before(deadline); i++ {
		// The coordinator may die or idle out while we wait; without this
		// every waiter sits out its --wait until some new submit respawns it.
		if i%30 == 0 {
			ensureServe(dir, stderr)
		}
		if res, ok := readResult(dir, r.ID); ok {
			fmt.Fprint(stdout, res.Stdout)
			if !strings.HasSuffix(res.Stdout, "\n") && res.Stdout != "" {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stderr, "utk queue: %s %s (waited %ds, ran %ds)\n", r.Kind, res.Status, res.WaitS, res.RunS)
			os.Remove(filepath.Join(dir, "results", r.ID+".json"))
			return res.Exit
		}
		// The coordinator writes a result before it removes the request, so a
		// request gone with no result was cancelled while still queued.
		if _, err := os.Stat(filepath.Join(dir, "requests", r.ID+".json")); err != nil {
			if _, ok := readResult(dir, r.ID); !ok {
				fmt.Fprintf(stderr, "utk queue: %s was cancelled before it ran\n", r.ID)
				return 130
			}
			continue
		}
		time.Sleep(submitPoll)
	}
	removeRequest(dir, r.ID)
	fmt.Fprintf(stderr, "utk queue: no result for %s after %ds; request withdrawn\n", r.ID, wait)
	return 1
}
