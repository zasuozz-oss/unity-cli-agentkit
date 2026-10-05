package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// An Editor that has been up for a day reloads its domain ten times slower
// than a fresh one: objects created with HideFlags.DontSave survive every
// reload and each one is backed up and restored again (measured on Unity
// 6000.0.84f1: 2.6 s after start, 34.5 s after 348 reloads and 118k objects).
// Past ~30 s a reload outlasts CaptureRetry and every agent reads it as a
// dead Editor. The Editor's own log is the one place that records the cost.

var reloadPrefix = []byte("Domain Reload Profiling: ")

// reloadTimes returns every domain reload duration (ms) an Editor log records,
// oldest first.
func reloadTimes(r io.Reader) []int {
	var out []int
	br := bufio.NewReaderSize(r, 64<<10)
	skip := false // inside a line longer than the buffer
	for {
		line, err := br.ReadSlice('\n')
		if !skip && bytes.HasPrefix(line, reloadPrefix) {
			v := strings.TrimSpace(string(line[len(reloadPrefix):]))
			if n, e := strconv.Atoi(strings.TrimSuffix(v, "ms")); e == nil {
				out = append(out, n)
			}
		}
		skip = err == bufio.ErrBufferFull
		if err != nil && err != bufio.ErrBufferFull {
			return out
		}
	}
}

// reloadLimitMS is the reload time past which an Editor counts as degraded.
// A knob because "slow" depends on the machine.
func reloadLimitMS() int {
	if n, err := strconv.Atoi(os.Getenv("UTK_RELOAD_LIMIT_MS")); err == nil && n > 0 {
		return n
	}
	return 10000
}

func median(v []int) int {
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[len(s)/2]
}

// reloadHealth compares the latest reloads with the session's first ones.
// Both conditions must hold: slow in absolute terms, and three times slower
// than this very Editor was when it started — a project whose reload is slow
// from the first minute gains nothing from a restart and must not loop on one.
func reloadHealth(ms []int) (last, base int, degraded bool) {
	if len(ms) < 8 {
		return 0, 0, false
	}
	last, base = median(ms[len(ms)-3:]), median(ms[:5])
	return last, base, last >= reloadLimitMS() && last >= 3*base
}

// parseEditorPS finds the interactive Editor of one project in
// `ps -axo pid=,command=` output. Its import workers carry the same project
// path, so they are told apart by -batchMode.
func parseEditorPS(ps, project string) (pid int, bin string) {
	const flag = " -projectpath "
	for _, line := range strings.Split(ps, "\n") {
		f := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(f) != 2 {
			continue
		}
		low := strings.ToLower(f[1])
		i := strings.Index(low, flag)
		if i < 0 || strings.Contains(low, " -batchmode") {
			continue
		}
		// The Editor is the process whose executable is Unity itself and whose
		// first argument is the project: a shell, grep or hook that only quotes
		// such a command line has arguments before it.
		// ponytail: a Unity installed under a folder containing " -" is not found.
		bin := f[1][:i]
		if base := strings.ToLower(bin[strings.LastIndexAny(bin, `/\`)+1:]); strings.Contains(bin, " -") || base != "unity" && base != "unity.exe" {
			continue
		}
		if rest := f[1][i+len(flag):]; rest == project || strings.HasPrefix(rest, project+" ") {
			n, _ := strconv.Atoi(f[0])
			return n, bin
		}
	}
	return 0, ""
}

// findEditor is parseEditorPS over the live process table.
// ponytail: ps/lsof are Unix; on Windows no Editor is found, so health is
// unknown and restart refuses — add a tasklist/wmic path if Windows agents need it.
func findEditor(project string) (int, string) {
	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return 0, ""
	}
	return parseEditorPS(string(out), project)
}

// editorLogPath asks the OS which log the Editor holds open. The default path
// lies: a second Editor starting up renames Editor.log to Editor-prev.log
// under the first one, which keeps writing to the renamed file.
func editorLogPath(pid int) string {
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-Fn").Output()
	if err != nil {
		return ""
	}
	any := ""
	for _, l := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(l, "n") || !strings.HasSuffix(l, ".log") {
			continue
		}
		if strings.HasPrefix(filepath.Base(l), "Editor") {
			return l[1:]
		}
		any = l[1:]
	}
	return any
}

// editorDegraded reports whether the project's Editor has slowed down enough
// to be worth a restart, with the sentence to show for it. A variable so the
// coordinator's tests need no Editor.
var editorDegraded = func(project string) (string, bool) {
	pid, _ := editor.find(project)
	if pid == 0 {
		return "", false
	}
	f, err := os.Open(editorLogPath(pid))
	if err != nil {
		return "", false
	}
	defer f.Close()
	ms := reloadTimes(f)
	last, base, bad := reloadHealth(ms)
	if !bad {
		return "", false
	}
	return fmt.Sprintf("the Editor has degraded: a domain reload now takes %.1fs (%.1fs when it started, %d reloads ago)",
		float64(last)/1000, float64(base)/1000, len(ms)), true
}
