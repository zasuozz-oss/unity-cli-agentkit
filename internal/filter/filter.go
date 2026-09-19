// Package filter holds the deterministic, loss-safe output filters and the
// metrics that quantify token savings (spec §3.2, §3.4).
package filter

import "strconv"

// Options carries utk-specific tuning extracted from the command line.
type Options struct {
	Raw      bool   // bypass all filtering, emit upstream output verbatim
	Truncate int    // opt-in array truncation for `exec` (0 = disabled)
	Grep     string // keep only matching lines of `list` (empty = no filter)
	Max      int    // screenshot long-edge cap in pixels (0 = full size)
	// ConsoleOnly narrows `console` to one exact severity. mapVerb sets it,
	// not SplitUtkFlags: --type is forwarded (as --level) as well as read.
	ConsoleOnly string
}

// ShotMax is the default long edge for `utk screenshot`. A 720×1600 game view
// costs ~1,450 tokens to read and a 512-wide one ~520, and layout, hierarchy
// and text placement all survive the reduction — which is what a screenshot is
// read for. `--max 0` keeps the native size.
const ShotMax = 512

// EstimateTokens approximates token count from bytes using the ~4 chars/token
// heuristic. Kept dependency-free on purpose (no external tokenizer).
func EstimateTokens(bytes int) int { return bytes / 4 }

// SplitUtkFlags removes utk-only flags (--raw, --truncate N, --grep P, --max N)
// from args, returning the remaining args (forwarded to unity-cli untouched)
// and Options. No official tool declares a `grep` or `max` parameter, so
// stripping them cannot shadow one.
func SplitUtkFlags(args []string) ([]string, Options) {
	var rest []string
	opts := Options{Max: ShotMax}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--raw":
			opts.Raw = true
		case "--truncate":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					opts.Truncate = n
					i++ // consume the value
					continue
				}
			}
		case "--grep":
			if i+1 < len(args) {
				opts.Grep = args[i+1]
				i++
				continue
			}
		case "--max":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					opts.Max = n
					i++
					continue
				}
			}
		default:
			rest = append(rest, args[i])
		}
	}
	return rest, opts
}

// Apply routes a payload to the right filter for kind. Unknown kinds are
// identity-mapped (loss-safe).
func Apply(kind string, payload []byte, opts Options) []byte {
	switch kind {
	case "console":
		return Console(payload, opts.ConsoleOnly)
	case "list":
		out := List(payload)
		if opts.Grep != "" {
			out = Grep(out, opts.Grep)
		}
		return out
	case "exec":
		return Exec(payload, opts.Truncate)
	case "tests":
		return Tests(payload)
	case "status":
		return Status(payload)
	default:
		return payload
	}
}
