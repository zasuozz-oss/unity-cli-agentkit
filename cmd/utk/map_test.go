package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestMapVerb(t *testing.T) {
	cases := []struct {
		cmd  string
		args []string
		want []string
		kind string
		only string
	}{
		{"console", nil, []string{"command", "console"}, "console", ""},
		{"console", []string{"--type", "error", "--limit", "20"},
			[]string{"command", "console", "--level", "error", "--tail", "20"}, "console", ""},
		{"console", []string{"--type=error"},
			[]string{"command", "console", "--level=error"}, "console", ""},
		// --level is a minimum, so warning and log over-select and the filter
		// has to narrow them back to utk's exact --type meaning.
		{"console", []string{"--type", "warning"},
			[]string{"command", "console", "--level", "warn"}, "console", "warning"},
		{"console", []string{"--type", "log", "--limit=5"},
			[]string{"command", "console", "--level", "log", "--tail=5"}, "console", "log"},
		// all is the tool's default: no --level at all.
		{"console", []string{"--type", "all"}, []string{"command", "console"}, "console", ""},
		{"exec", []string{"return 1;"}, []string{"command", "eval", "return 1;"}, "exec", ""},
		{"list", nil, []string{"list"}, "list", ""},
		{"status", nil, []string{"pipeline", "list"}, "status", ""},
		{"test", []string{"--mode", "PlayMode"}, []string{"test", "--mode", "PlayMode"}, "batchtest", ""},
		{"editor", []string{"refresh", "--compile"}, []string{"command", "recompile"}, "exec", ""},
		{"editor", []string{"play"}, []string{"command", "editor_play"}, "exec", ""},
		{"get_scene_hierarchy", []string{"--max-depth", "2"},
			[]string{"command", "get_scene_hierarchy", "--max-depth", "2"}, "exec", ""},
		{"import", []string{"--source", "/tmp/a.png", "--path", "a.png"},
			[]string{"command", "import_asset", "--source", "/tmp/a.png", "--path", "a.png"}, "exec", ""},
	}
	for _, c := range cases {
		got, kind, only := mapVerb(c.cmd, c.args)
		if !reflect.DeepEqual(got, c.want) || kind != c.kind || only != c.only {
			t.Errorf("mapVerb(%s %v) = %v %q %q; want %v %q %q",
				c.cmd, c.args, got, kind, only, c.want, c.kind, c.only)
		}
	}
}

func TestMapVerbReserialize(t *testing.T) {
	got, kind, _ := mapVerb("reserialize", []string{"Assets/My Prefab.prefab"})
	if kind != "exec" || got[0] != "command" || got[1] != "eval" {
		t.Fatalf("got %v %q", got, kind)
	}
	if !strings.Contains(got[2], `"Assets/My Prefab.prefab"`) ||
		!strings.Contains(got[2], "ForceReserializeAssets") {
		t.Fatalf("bad snippet: %s", got[2])
	}
}

func TestMapVerbReserializeBatchesEveryPath(t *testing.T) {
	got, _, _ := mapVerb("reserialize", []string{"Assets/A.prefab", "Assets/B.asset"})
	if !strings.Contains(got[2], `{"Assets/A.prefab", "Assets/B.asset"}`) {
		t.Fatalf("both paths must land in one eval: %s", got[2])
	}
}

// A suite bigger than a few dozen tests blows `unity command`'s hardcoded 30s
// wait, which fails the run and leaves the Editor busy behind it. The async
// hand-off is the only way past it — the CLI's own --timeout is a no-op here.
func TestMapVerb_RunTestsAsync(t *testing.T) {
	got, kind, _ := mapVerb("run_tests", []string{"--mode", "editor"})
	want := []string{"command", "run_tests", "--mode", "editor", "--async_tests", "true"}
	if kind != "tests" || !reflect.DeepEqual(got, want) {
		t.Fatalf("mapVerb = %v (%s), want %v", got, kind, want)
	}
	// An explicit value wins, in either form.
	for _, user := range [][]string{{"--async_tests", "false"}, {"--async_tests=true"}} {
		got, _, _ := mapVerb("run_tests", user)
		if len(got) != 2+len(user) {
			t.Fatalf("user --async_tests must not be doubled, got %v", got)
		}
	}
	// test_status is a poll, not a run: nothing to hand off.
	if got, _, _ := mapVerb("test_status", nil); len(got) != 2 {
		t.Fatalf("test_status = %v, want no injected flag", got)
	}
}
