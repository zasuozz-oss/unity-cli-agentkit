package filter

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// descCut caps a tool's description in the one-line listing. Tuned so the full
// 140-tool listing stays within the ~8KB budget (spec §6); the full text is
// always available via ListDetail.
const descCut = 33

type listParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default"`
}

type listTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Group       string      `json:"group"`
	Parameters  []listParam `json:"parameters"`
}

type listData struct {
	Count *int       `json:"count"`
	Tools []listTool `json:"tools"`
}

// List renders one line per tool from `unity list --json` data. Unexpected
// input passes through unchanged (loss-safe).
func List(data []byte) []byte {
	var d listData
	if err := json.Unmarshal(data, &d); err != nil || d.Count == nil {
		return data
	}
	var b strings.Builder
	b.WriteString(strconv.Itoa(*d.Count) + " tools. Detail: utk list <tool>\n")
	group := ""
	for _, t := range d.Tools {
		if t.Group != group {
			group = t.Group
			b.WriteString("# " + group + "\n")
		}
		b.WriteString(t.Name + " — " + cutLine(t.Description, descCut) + "\n")
	}
	return []byte(b.String())
}

// Grep keeps only the tool lines of a rendered listing that match pat, plus the
// group headings they sit under. `unity list` has no filter of its own, so
// finding a tool means paying for all ~140 of them (~7.5KB) and piping the
// result through grep — the bytes are spent before the pipe ever sees them.
// An invalid regexp is treated as a literal, so a stray `[` searches rather
// than errors.
func Grep(rendered []byte, pat string) []byte {
	re, err := regexp.Compile("(?i)" + pat)
	if err != nil {
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pat))
	}
	lines := strings.Split(strings.TrimRight(string(rendered), "\n"), "\n")
	if len(lines) < 2 {
		return rendered // not the rendered listing → hand it back untouched
	}
	var out []string
	pending, hits := "", 0
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "# ") {
			pending = l
			continue
		}
		if !re.MatchString(l) {
			continue
		}
		if pending != "" {
			out = append(out, pending)
			pending = ""
		}
		out = append(out, l)
		hits++
	}
	head := strconv.Itoa(hits) + " tools match /" + pat + "/. Detail: utk list <tool>"
	if hits == 0 {
		head = "no tool matches /" + pat + "/ (of " + strings.TrimSuffix(lines[0], ". Detail: utk list <tool>") + ")"
	}
	return []byte(strings.Join(append([]string{head}, out...), "\n") + "\n")
}

// ListDetail renders one tool's full description and parameters. An unknown
// name lists close matches (substring) instead.
func ListDetail(data []byte, tool string) []byte {
	var d listData
	if err := json.Unmarshal(data, &d); err != nil || d.Count == nil {
		return data
	}
	for _, t := range d.Tools {
		if t.Name != tool {
			continue
		}
		var b strings.Builder
		b.WriteString(t.Name + " (" + t.Group + ")\n" + t.Description + "\n")
		for _, p := range t.Parameters {
			line := "  --" + p.Name + " <" + p.Type + ">"
			if p.Required {
				line += " (required)"
			} else if p.Default != nil {
				dv, _ := json.Marshal(p.Default)
				line += " (default " + string(dv) + ")"
			}
			b.WriteString(line + " " + p.Description + "\n")
		}
		return []byte(b.String())
	}
	var near []string
	for _, t := range d.Tools {
		if strings.Contains(t.Name, tool) || strings.Contains(tool, t.Name) {
			near = append(near, t.Name)
		}
	}
	return []byte("no tool named " + tool + "; close: " + strings.Join(near, ", ") + "\n")
}

// cutLine returns the first line of s, capped at n runes with an ellipsis.
func cutLine(s string, n int) string {
	s, _, _ = strings.Cut(s, "\n")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
