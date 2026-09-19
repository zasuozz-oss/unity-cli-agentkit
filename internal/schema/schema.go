// Package schema caches the official CLI's tool catalogue on disk so utk can
// reject a misspelled flag locally, before anything reaches the Editor.
//
// The official CLI ignores parameters it does not recognise instead of
// refusing them, which turns a typo into a silent change of meaning:
// `open_scene --mode Additive` drops the unknown --mode and opens the scene in
// the default mode, replacing whatever the user had open. Catching that costs
// one stat and one cache read (~1ms); asking the CLI for the schema on every
// call would cost a round-trip (~200ms) on every call instead.
package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zasuo/unity-cli-agentkit/internal/initcmd"
	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// cacheFile is the catalogue's name under the kit home.
const cacheFile = "tools-schema.json"

// cliFlags are accepted by `unity command` itself rather than by any tool, so
// they never appear in a tool's parameter list (from `unity command --help`,
// 1.0.0-beta.10). utk adds --json/--no-banner itself and mapVerb adds --timeout.
// Missing one here turns a valid CLI flag into a bogus "unknown flag" report.
// The listing-query flags (--limit, --query, --sort…) are deliberately absent:
// they belong to `unity command`'s list mode, and --limit is also a real tool
// parameter, so whitelisting them would blunt typo detection on the exec path.
var cliFlags = map[string]bool{
	"--project-path": true, "--runtime": true, "--runtime-path": true,
	"--timeout": true, "--help": true, "--format": true, "--json": true,
	"--no-banner": true, "--non-interactive": true, "--quiet": true,
	"--verbose": true, "--proxy": true, "--proxy-disable": true,
	"--log-proxy": true, "--no-log-proxy": true, "--version": true,
	"--color": true, "--no-color": true, "--no-pager": true,
	"--caller": true, "--skill": true, "--detach": true,
	"--result-only": true,
}

// Catalogue maps each tool name to the parameter names it accepts.
type Catalogue map[string]map[string]bool

// cache is the on-disk form. The binary's size and modification time key the
// entry: a CLI upgrade replaces the binary and so invalidates the catalogue,
// which a plain TTL would keep serving until it expired.
type cache struct {
	Bin     string              `json:"bin"`
	Size    int64               `json:"size"`
	ModTime int64               `json:"modTimeUnixNano"`
	Tools   map[string][]string `json:"tools"`
}

// Load returns the catalogue, rebuilding it from `unity list` when the cache is
// absent or the CLI binary has changed since it was written. It returns nil for
// every failure — a missing catalogue must never block a command, so callers
// treat nil as "cannot validate" and carry on.
func Load(stderr io.Writer) Catalogue {
	bin, err := official.Bin()
	if err != nil {
		return nil
	}
	st, err := os.Stat(bin)
	if err != nil {
		return nil
	}
	path, err := cachePath()
	if err != nil {
		return nil
	}
	if c, err := read(path); err == nil && c.Bin == bin &&
		c.Size == st.Size() && c.ModTime == st.ModTime().UnixNano() {
		return toCatalogue(c.Tools)
	}
	// Rebuilding costs one `unity list` (~600ms). It happens on the first run
	// after install and after every CLI upgrade, so say so rather than letting
	// an unexplained pause look like a hang.
	fmt.Fprintln(stderr, "utk: caching the tool schema (first run after a CLI change)…")
	tools := fetch(stderr)
	if tools == nil {
		return nil
	}
	write(path, cache{Bin: bin, Size: st.Size(), ModTime: st.ModTime().UnixNano(), Tools: tools})
	return toCatalogue(tools)
}

// Unknown returns the first flag in args that tool does not accept, together
// with any near matches. It reports nothing when the tool is absent from the
// catalogue, so a catalogue that has fallen behind the CLI cannot reject a
// valid call.
func (c Catalogue) Unknown(tool string, args []string) (flag string, near []string) {
	params, ok := c[tool]
	if !ok {
		return "", nil
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			continue
		}
		name, hasValue := a, false
		if n, _, found := strings.Cut(a, "="); found {
			name, hasValue = n, true
		}
		if !params[strings.TrimPrefix(name, "--")] && !cliFlags[name] {
			return name, nearNames(params, strings.TrimPrefix(name, "--"))
		}
		// A known flag consumes the token after it, so a value that happens to
		// start with -- is never mistaken for a flag of its own.
		if !hasValue {
			i++
		}
	}
	return "", nil
}

// nearNames returns parameter names that overlap the typo as a substring, the
// same closeness rule the unknown-tool error uses.
func nearNames(params map[string]bool, typo string) []string {
	var out []string
	for p := range params {
		if strings.Contains(p, typo) || strings.Contains(typo, p) {
			out = append(out, "--"+p)
		}
	}
	return out
}

func cachePath() (string, error) {
	home, err := initcmd.KitHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, cacheFile), nil
}

func read(path string) (cache, error) {
	var c cache
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

func write(path string, c cache) {
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, b, 0o644)
}

// fetch asks the official CLI for the catalogue. Returns nil on any shape it
// does not recognise, which keeps a broken listing from poisoning the cache.
func fetch(stderr io.Writer) map[string][]string {
	raw, code := official.Capture([]string{"list", "--json", "--no-banner"}, stderr)
	if code != 0 {
		return nil
	}
	env, err := official.Parse(raw)
	if err != nil || !env.Success {
		return nil
	}
	var d struct {
		Tools []struct {
			Name       string `json:"name"`
			Parameters []struct {
				Name string `json:"name"`
			} `json:"parameters"`
		} `json:"tools"`
	}
	if json.Unmarshal(env.Payload(false), &d) != nil || len(d.Tools) == 0 {
		return nil
	}
	out := make(map[string][]string, len(d.Tools))
	for _, t := range d.Tools {
		names := make([]string, 0, len(t.Parameters))
		for _, p := range t.Parameters {
			names = append(names, p.Name)
		}
		out[t.Name] = names
	}
	return out
}

func toCatalogue(tools map[string][]string) Catalogue {
	c := make(Catalogue, len(tools))
	for name, params := range tools {
		set := make(map[string]bool, len(params))
		for _, p := range params {
			set[p] = true
		}
		c[name] = set
	}
	return c
}
