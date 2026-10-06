package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	serveIdleSleep  = 2 * time.Second
	editorProbeWait = 60 * time.Second
)

// lockTouchEvery matches unity-job.sh's toucher: unity-lock.sh deletes a lock
// older than 15 min as stale, so a long cycle that never touched its lock would
// let a legacy job take the Editor mid-cycle.
var lockTouchEvery = 240 * time.Second

// serveIdleExit ends a coordinator nobody has used for a while. It would
// otherwise run forever on the binary and env of whichever agent submitted
// first; the next submit respawns it fresh.
var serveIdleExit = 10 * time.Minute

func editorAnswers(cwd string) bool {
	_, code := queueExec(cwd, editorProbeWait, nil, utkSelf(), "status")
	return code == 0
}

func acquireEditorLock(cwd string) bool {
	_, code := queueExec(cwd, 61*time.Minute, lockEnv(cwd), "bash", lockScript(), "acquire", "coordinator", "3600")
	return code == 0
}

func releaseEditorLock(cwd string) {
	queueExec(cwd, time.Minute, lockEnv(cwd), "bash", lockScript(), "release", "coordinator")
}

// serveOnce runs one cycle over whatever is pending. It returns how many
// requests it answered. The gate runs before the lock: a failing dotnet build
// must cost nobody else Editor time.
func serveOnce(dir string, stderr io.Writer) int {
	sweepResults(dir)
	reqs, err := readRequests(dir)
	if err != nil {
		fmt.Fprintln(stderr, "utk queue serve:", err)
		return 0
	}
	if len(reqs) == 0 {
		return 0
	}
	cwd := reqs[0].Cwd
	lastServedCwd = cwd
	var results map[string]queueResult
	blocked := func(msg string) {
		results = map[string]queueResult{}
		for _, r := range reqs {
			results[r.ID] = queueResult{ID: r.ID, Status: "EDITOR_BLOCKED", Exit: 1, Stdout: msg}
		}
	}
	if !editorAnswers(cwd) {
		// A dead server: nothing we send will run. Say so to everyone now
		// rather than let them wait out an hour each.
		blocked("the Editor does not answer `utk status` (not running?); fix it and resubmit\n")
	} else if !busyElsewhere() && probeMainThread(cwd) == "hung" {
		// `utk status` answers from the HTTP thread; this asked the main thread.
		// Hung, not busy: keep the requests for after the watchdog's restart.
		notice := hungNotice(dir)
		if notice == "" {
			return 0
		}
		blocked("EDITOR_BLOCKED: " + notice + "\n")
	} else if n, _ := playUnowned(dir, cwd, time.Now()); n != "" {
		// Play nobody owns: run nothing, never stop it. Jobs stay queued until their wait budget ends BLOCKED.
		blockedByPlay(reqs, n, &results, time.Now())
		if len(results) == 0 {
			return 0
		}
	} else {
		c := newCycle(dir, reqs, stderr)
		editorOK := c.runGateAndRefreshBeforeLock()
		if len(c.results) == len(reqs) {
			// The gate failed everyone: no Editor work left, so do not queue for the lock.
		} else if acquireEditorLock(cwd) {
			c.start = time.Now() // wait_s/hold_s mean lock wait/hold, as in unity-job.sh's rows
			c.dropWithdrawn()
			if len(c.reqs) == 0 {
				releaseEditorLock(cwd) // everyone left during the wait
			} else {
				c.holdEditor(cwd, editorOK)
			}
		} else {
			for _, r := range c.byKind("compile") {
				c.finish(r, "FAIL", 1, "could not take the Editor lock within an hour\n")
			}
			for _, kind := range []string{"test", "scene", "shot"} {
				for _, r := range c.byKind(kind) {
					c.finish(r, "FAIL", 1, "could not take the Editor lock within an hour\n")
				}
			}
		}
		results = c.results
	}
	written := 0
	for _, r := range reqs {
		res, ok := results[r.ID]
		if !ok {
			continue
		}
		if err := writeResult(dir, res); err != nil {
			fmt.Fprintln(stderr, "utk queue serve: write result:", err)
			continue
		}
		written++
		removeRequest(dir, r.ID)
		appendTelemetry(telemetryEntry{TS: time.Now().UTC().Format(time.RFC3339), Owner: r.Agent, Task: r.ID,
			Kind: r.Kind, WaitS: res.WaitS, HoldS: res.RunS, Result: res.Status, Via: "queue"})
	}
	return written // failed writes must not count, or serve would skip its idle sleep and hammer the Editor
}

// lastServedCwd is the project of the latest cycle: the one maybeRestart checks.
var lastServedCwd string

// maybeRestart restarts a degraded Editor (health.go) in the gap after a
// cycle, when nobody is queued. It holds the Editor lock so a legacy
// unity-job.sh user waits instead of finding no Editor; `utk editor restart`
// itself refuses while the Editor plays or holds unsaved scenes.
func maybeRestart(dir, cwd string, stderr io.Writer) {
	if cwd == "" || os.Getenv("UTK_NO_AUTO_RESTART") != "" {
		return
	}
	if reqs, _ := readRequests(dir); len(reqs) > 0 {
		return
	}
	msg, bad := editorDegraded(cwd)
	if !bad {
		return
	}
	if _, code := queueExec(cwd, time.Minute, lockEnv(cwd), "bash", lockScript(), "acquire", "coordinator", "5"); code != 0 {
		return // somebody is using the Editor outside the queue; try after the next cycle
	}
	defer releaseEditorLock(cwd)
	fmt.Fprintf(stderr, "utk queue: %s; restarting it while nobody is waiting (UTK_NO_AUTO_RESTART=1 turns this off)\n", msg)
	out, _ := queueExec(cwd, restartReadyWait+3*time.Minute, nil, utkSelf(), "editor", "restart")
	io.WriteString(stderr, out)
}

// sweepResults deletes results nobody collected within an hour (spec §7): the
// submitter withdrew or died, and nothing else ever reads them.
func sweepResults(dir string) {
	ents, _ := os.ReadDir(filepath.Join(dir, "results"))
	for _, e := range ents {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
			os.Remove(filepath.Join(dir, "results", e.Name()))
		}
	}
}

// holdEditor is the part of a cycle that runs under the Editor lock. The lock
// script calls a lock untouched for 15 min stale, so a long cycle touches it
// every lockTouchEvery or a waiting unity-job.sh user takes the Editor mid-cycle.
func (c *cycle) holdEditor(cwd string, editorOK bool) {
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		t := time.NewTicker(lockTouchEvery)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				queueExec(cwd, time.Minute, lockEnv(cwd), "bash", lockScript(), "touch", "coordinator")
			}
		}
	}()
	// Autotick stays on after the cycle: an unfocused macOS Editor is throttled
	// without it, and the next command (a unity-job, the next cycle) pays for
	// that. runServe turns it off when the coordinator idles out.
	defer func() {
		clearRunning(c.dir)
		close(done)
		<-stopped // a late touch must not land after the release
		releaseEditorLock(cwd)
	}()
	queueExec(cwd, time.Minute, nil, utkSelf(), "set_autotick", "--enable", "true")
	if editorOK {
		c.runEditorRefresh()
	}
	// Fonts Unity leaked before the reload above are collectable now (gc.go).
	if out, code := queueExec(cwd, 2*time.Minute, nil, utkSelf(), "editor", "gc"); code == 0 && !strings.HasPrefix(out, "destroyed 0 ") {
		io.WriteString(c.stderr, "utk queue: gc: "+firstLine(out)+"\n")
	}
	c.runTests()
	c.runSceneJobs()
	c.runShots()
}

// dropWithdrawn forgets requests whose file is gone or whose submitter died:
// the lock wait can outlast a submitter's --wait (its withdraw deletes the
// file) or the agent tab itself. Running them anyway is Editor time for nobody.
func (c *cycle) dropWithdrawn() {
	var keep []queueRequest
	for _, r := range c.reqs {
		if _, err := os.Stat(filepath.Join(c.dir, "requests", r.ID+".json")); err == nil && pidAlive(r.PID) {
			keep = append(keep, r)
		}
	}
	c.reqs = keep
}

// runServe is the coordinator loop: one per queue dir, guarded by a mkdir lock
// and a pid file so a second `serve` (two agents' submits racing) exits.
func runServe(dir string, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(stderr, "utk queue serve:", err)
		return 1
	}
	lock := filepath.Join(dir, "serve.lock.d")
	pidFile := filepath.Join(dir, "serve.pid")
	// The Editor lock records this pid: if the coordinator dies holding it,
	// the next acquire frees it instead of waiting out the 15-min stale rule.
	os.Setenv("UNITY_LOCK_PID", strconv.Itoa(os.Getpid()))
	if err := os.Mkdir(lock, 0o755); err != nil {
		b, _ := os.ReadFile(pidFile)
		if pidIsServe(atoi(string(b))) {
			fmt.Fprintln(stderr, "utk queue serve: already running (pid", strings.TrimSpace(string(b))+")")
			return 0
		}
		if atoi(string(b)) == 0 {
			// A starting server has made the lock dir but not written its pid yet.
			time.Sleep(time.Second)
			b, _ = os.ReadFile(pidFile)
			if pidIsServe(atoi(string(b))) {
				fmt.Fprintln(stderr, "utk queue serve: already running (pid", strings.TrimSpace(string(b))+")")
				return 0
			}
		}
		os.RemoveAll(lock) // stale: the previous server died without cleaning up
		if err := os.Mkdir(lock, 0o755); err != nil {
			fmt.Fprintln(stderr, "utk queue serve:", err)
			return 1
		}
	}
	defer os.RemoveAll(lock)
	os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
	defer os.Remove(pidFile)
	fmt.Fprintln(stdout, "utk queue serve:", dir)
	lastWork := time.Now()
	wd := newWatchdog(dir)
	for {
		if serveOnce(dir, stderr) == 0 {
			if os.Getenv("UTK_QUEUE_ONCE") == "1" {
				return 0
			}
			wd.tick(lastServedCwd, stderr)
			// The defers remove serve.pid and serve.lock.d. A submit racing
			// this exit is covered: runSubmit re-runs ensureServe while it waits.
			// Requests held back for a hung Editor are not idleness.
			if reqs, _ := readRequests(dir); len(reqs) == 0 && !wd.watching() && time.Since(lastWork) >= serveIdleExit {
				if lastServedCwd != "" {
					queueExec(lastServedCwd, time.Minute, nil, utkSelf(), "set_autotick", "--enable", "false")
				}
				fmt.Fprintln(stdout, "utk queue serve: idle, exiting")
				return 0
			}
			time.Sleep(serveIdleSleep)
			continue
		}
		lastWork = time.Now()
		maybeRestart(dir, lastServedCwd, stderr)
		if os.Getenv("UTK_QUEUE_ONCE") == "1" {
			return 0
		}
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// ensureServe starts a detached coordinator when none is alive, so the first
// submit of a session needs no separate setup step.
func ensureServe(dir string, stderr io.Writer) {
	if b, err := os.ReadFile(filepath.Join(dir, "serve.pid")); err == nil && pidIsServe(atoi(string(b))) {
		return
	}
	if err := serveSpawn(dir); err != nil {
		fmt.Fprintln(stderr, "utk queue: could not start the coordinator:", err)
		return
	}
	fmt.Fprintln(stderr, "utk queue: started coordinator for", dir)
}

// serveSpawn starts a detached `utk queue serve`. A variable so tests do not
// re-exec the test binary as a coordinator.
var serveSpawn = func(dir string) error {
	logf, _ := os.OpenFile(filepath.Join(dir, "serve.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	defer logf.Close() // the child keeps its own copy of the fd
	cmd := exec.Command(utkSelf(), "queue", "serve")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Env = os.Environ()
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Process.Release()
	return nil
}
