package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zasuo/unity-cli-agentkit/internal/filter"
	"github.com/zasuo/unity-cli-agentkit/internal/initcmd"
	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

const usage = `utk — token filter for the official Unity CLI.

Every verb shells out to ` + "`unity`" + ` and compresses its output; exit codes and
stderr are preserved. Run ` + "`utk list`" + ` for the ~150 official tools.

  utk console [--type all|log|warning|error] [--limit N]
                          Editor console; compile errors grouped by root cause
                          (first error first), stacktraces trimmed
  utk exec '<csharp>'     run C# in the Editor; the snippet's own Debug.Log is
                          folded into the answer
                          --timeout <ms> is the snippet's budget (default 60000)
  utk exec --file <f.cs>  same, reading the snippet from a file
                          NOTE: a snippet is a *method body* — ` + "`using`" + ` is invalid
                          and only UnityEngine/UnityEditor are in scope; write
                          other namespaces in full (TMPro.TextMeshProUGUI)
  utk run_script --file <f.cs> [--entry Type.Method] [--args '[…]']
                          a *real* C# file instead: ` + "`using`" + `, namespaces and
                          several types, compiled in memory with no domain
                          reload. Keep the file OUTSIDE Assets/ or writing it
                          triggers an import. --dry_run true compiles only and
                          returns diagnostics with line/column; its Debug.Log
                          goes to the console, not the answer
  utk run_tests [--mode all|editor|playmode] [--filter <name>]
                          run tests; summary + failures only, exit 1 if any fail
  utk test_status         poll results after PlayMode tests hand off
  utk test [args…]        official batchmode runner; needs the Editor CLOSED
                          same summary as run_tests, read from its NUnit report
  utk editor refresh|play|pause|stop|status
                          recompile (waits for it; exit 1 on compile errors)
                          and play-mode control
  utk build --confirm true [--outputPath P] [--target T]
                          Player build; waits for the verdict (up to 30 min),
                          exit 1 unless Succeeded; --wait false hands off
                          run it in the background and let the harness notify
  utk import <file> [dest] copy an external file (image/audio/model…) into
                          Assets/ and import it — never base64 through exec;
                          dest is relative to Assets/, defaults to the file name
  utk reserialize <path…> validate assets edited as text
  utk list [<tool>|--grep P]
                          one line per tool, one tool's schema, or a search
  utk screenshot [--max N] capture the game view, downscaled to N px (default 512)
  utk status              editor instances and pipeline reachability
  utk init [--uninstall]  install skills + CLAUDE.md/AGENTS.md guidance
  utk <tool> [--k v]      any other official tool

Flags:
  --raw                   bypass filtering; stream unity's output unchanged
  --truncate N            shorten JSON arrays in exec output (opt-in, lossy)
  --grep <pattern>        (list) keep only matching tools
  --max <px>              (screenshot) cap the long edge; 0 keeps native size

Env:
  UTK_UNITY_BIN           path to the unity binary if it is not on PATH
  UNITY_CLI_AGENTKIT_HOME kit home (default ~/.unity-cli-agentkit)
  UTK_NO_EXEC_LOGS        skip folding a snippet's Debug.Log into exec output
`

// projectRoot resolves the Unity project the official CLI will target, in the
// same order it does: the UNITY_PROJECT_PATH override, then the nearest
// ancestor of cwd holding Assets/ and ProjectSettings/, and finally — when cwd
// is outside any project — the one reachable Editor instance. Returns "" only
// when none of the three answers. stderr carries the lookup's own warnings.
func projectRoot(stderr io.Writer) string {
	if p := localProjectRoot(); p != "" {
		return p
	}
	raw, _ := official.Capture([]string{"pipeline", "list", "--json", "--no-banner"}, stderr)
	env, perr := official.Parse(raw)
	if perr != nil || !env.Success {
		return ""
	}
	return filter.ProjectPath(env.Payload(false))
}

// localProjectRoot answers the first two steps of projectRoot — the
// UNITY_PROJECT_PATH override, then the nearest ancestor of cwd that is a Unity
// project — without the subprocess the third one costs. It is on the path of
// every command, so it may not spawn a `unity` of its own.
func localProjectRoot() string {
	if p := os.Getenv("UNITY_PROJECT_PATH"); p != "" {
		return p
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if initcmd.DetectUnityProject(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// checkReserializePaths refuses paths that are not on disk. Unity skips missing
// assets without complaint and the generated snippet returns a fixed
// "reserialized", so without this a typo'd path reads as a clean validation.
//
// Asset paths are project-root relative — the same base the Editor resolves
// them against — not cwd relative, so running utk from anywhere but the project
// root must not turn a valid path into a "no such file".
func checkReserializePaths(paths []string, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "utk: reserialize needs at least one asset path")
		return 2
	}
	root := projectRoot(stderr)
	var missing []string
	for _, p := range paths {
		// Skip flags: reserialize forwards none today, but a future one must
		// not be mistaken for a path.
		if strings.HasPrefix(p, "-") {
			continue
		}
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, p) // root "" → cwd-relative, as before
		}
		if _, err := os.Stat(abs); err != nil {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		base := root
		if base == "" {
			base = "the current directory (no Unity project found above it)"
		}
		fmt.Fprintln(stderr, "utk: reserialize: no such file:", strings.Join(missing, ", "))
		fmt.Fprintln(stderr, "  resolved against", base+"; nothing was sent to the Editor")
		return 2
	}
	return 0
}
