package schema

import (
	"reflect"
	"sort"
	"testing"
)

func cat() Catalogue {
	return toCatalogue(map[string][]string{
		"open_scene":       {"path", "additive"},
		"create_scene":     {"path", "additive", "template"},
		"set_transform":    {"path", "position", "rotation", "scale"},
		"editor_status":    {},
		"list_open_scenes": {},
	})
}

func TestUnknownFlags(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args []string
		want string
	}{
		{"accepted flag", "open_scene", []string{"--path", "A.unity"}, ""},
		{"accepted with equals", "open_scene", []string{"--path=A.unity", "--additive=true"}, ""},
		// The bug this exists for: the CLI drops --mode and opens the scene in
		// the default mode, replacing whatever was open.
		{"unknown flag", "open_scene", []string{"--path", "A.unity", "--mode", "Additive"}, "--mode"},
		{"cli-level flag allowed", "open_scene",
			[]string{"--path", "A.unity", "--project-path", "/p", "--timeout", "600"}, ""},
		// Both belong to `unity command` itself, so a tool with no parameters of
		// its own must still accept them. --result-only arrived in CLI
		// 1.0.0-beta.10; --detach was missing from the whitelist before it.
		{"command-level flags on a tool with no params", "editor_status",
			[]string{"--result-only", "--detach"}, ""},
		// A value that begins with -- belongs to the flag before it, not to the
		// tool: rejecting it would break a legitimate call.
		{"value looking like a flag", "set_transform",
			[]string{"--path", "/Cube", "--position", "--1,0,0"}, ""},
		{"first unknown wins", "open_scene",
			[]string{"--nope", "x", "--alsonope", "y"}, "--nope"},
		{"positional args ignored", "open_scene", []string{"A.unity"}, ""},
		// Fail-open: a tool the cache has never heard of is never rejected.
		{"unknown tool", "brand_new_tool", []string{"--whatever"}, ""},
		{"tool with no params", "editor_status", []string{"--bogus"}, "--bogus"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := cat().Unknown(c.tool, c.args)
			if got != c.want {
				t.Fatalf("Unknown(%s, %v) = %q, want %q", c.tool, c.args, got, c.want)
			}
		})
	}
}

func TestUnknownSuggestsNearNames(t *testing.T) {
	_, near := cat().Unknown("create_scene", []string{"--add"})
	sort.Strings(near)
	if !reflect.DeepEqual(near, []string{"--additive"}) {
		t.Fatalf("near = %v, want [--additive]", near)
	}
}

func TestNilCatalogueRejectsNothing(t *testing.T) {
	var c Catalogue
	if got, _ := c.Unknown("open_scene", []string{"--mode", "x"}); got != "" {
		t.Fatalf("nil catalogue rejected %q", got)
	}
}
