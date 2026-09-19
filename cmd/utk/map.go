package main

import (
	"fmt"
	"strings"
)

// editorSubMap routes `utk editor <sub>` onto official tools.
var editorSubMap = map[string]string{
	"refresh": "recompile",
	"play":    "editor_play",
	"pause":   "editor_pause",
	"stop":    "editor_stop",
	"status":  "editor_status",
}

// consoleLevel maps utk's --type (an exact severity) onto the `console` tool's
// --level (a *minimum* severity), and reports the severity the filter must
// narrow to afterwards. error is already exact — nothing is worse than an
// error — so only warning and log need narrowing. An unrecognised value is
// handed to the tool so it rejects it, rather than silently meaning "all".
func consoleLevel(t string) (level, only string) {
	switch t {
	case "all", "":
		return "", ""
	case "error":
		return "error", ""
	case "warning":
		return "warn", "warning"
	case "log":
		return "log", "log"
	default:
		return t, ""
	}
}

// mapVerb translates a utk verb + args into official CLI argv and the filter
// kind. kind "" means stream straight through with no filtering. only carries
// the exact severity `console` must be narrowed to (empty for every other verb).
func mapVerb(cmd string, args []string) (unityArgs []string, kind, only string) {
	switch cmd {
	case "console":
		// com.unity.pipeline 0.7.0 removed get_console_logs; `console` replaces
		// it and already exists on 0.6.0, so this works on both.
		out := []string{"command", "console"}
		for i := 0; i < len(args); i++ {
			switch {
			case args[i] == "--type" && i+1 < len(args):
				lvl, o := consoleLevel(args[i+1])
				only = o
				if lvl != "" {
					out = append(out, "--level", lvl)
				}
				i++
			case strings.HasPrefix(args[i], "--type="):
				lvl, o := consoleLevel(strings.TrimPrefix(args[i], "--type="))
				only = o
				if lvl != "" {
					out = append(out, "--level="+lvl)
				}
			case args[i] == "--limit" && i+1 < len(args):
				out = append(out, "--tail", args[i+1])
				i++
			case strings.HasPrefix(args[i], "--limit="):
				out = append(out, "--tail="+strings.TrimPrefix(args[i], "--limit="))
			default:
				out = append(out, args[i])
			}
		}
		return out, "console", only
	case "exec":
		// A snippet long enough to be worth a file is also long enough that
		// `utk exec "$(cat f.cs)"` mangles it: the shell re-splits it, and the
		// quoting rules differ per shell. The CLI already reads the file itself.
		if hasFlag(args, "--file") {
			return append([]string{"command", "eval_file"}, args...), "exec", ""
		}
		return append([]string{"command", "eval"}, args...), "exec", ""
	case "list":
		return []string{"list"}, "list", ""
	case "status":
		return []string{"pipeline", "list"}, "status", ""
	case "test":
		return append([]string{"test"}, args...), "batchtest", ""
	case "run_tests", "test_status":
		// Both return the same {Summary, Results} payload — test_status is how
		// PlayMode results arrive after run_tests hands off asynchronously.
		out := append([]string{"command", cmd}, args...)
		// `unity command` waits 30s for the Editor by default. beta.5 fixed its
		// --timeout flag (beta.3 parsed and dropped it), but a synchronous run
		// still needs the caller to guess the suite's duration up front and a
		// wrong guess reports failure while the Editor keeps running it. The
		// tool's async mode needs no guess: it hands off in one short call and
		// the results arrive through test_status, which main.go then polls.
		if cmd == "run_tests" && !hasFlag(args, "--async_tests") {
			out = append(out, "--async_tests", "true")
		}
		return out, "tests", ""
	case "reserialize":
		// One eval for the whole batch — the skills document `utk reserialize
		// <paths…>` as the single validation step after direct YAML edits.
		// %q escaping covers quotes/backslashes for both Go and C# string literals.
		quoted := make([]string, len(args))
		for i, p := range args {
			quoted[i] = fmt.Sprintf("%q", p)
		}
		snippet := fmt.Sprintf(
			`UnityEditor.AssetDatabase.ForceReserializeAssets(new string[]{%s}); return "reserialized";`,
			strings.Join(quoted, ", "))
		return []string{"command", "eval", snippet}, "exec", ""
	case "import":
		// `utk import <file> [dest]` — prepImportArgs (main.go) has already
		// rewritten the positionals into import_asset's --source/--path.
		return append([]string{"command", "import_asset"}, args...), "exec", ""
	case "editor":
		if len(args) > 0 {
			if tool, ok := editorSubMap[args[0]]; ok {
				// --compile is implicit in recompile; drop remaining legacy flags.
				return []string{"command", tool}, "exec", ""
			}
		}
		return append([]string{"command"}, args...), "exec", ""
	default:
		return append([]string{"command", cmd}, args...), "exec", ""
	}
}

// hasFlag reports whether name appears in args, in either `--f v` or `--f=v`
// form, so a user-supplied value is never shadowed by a default.
func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}
