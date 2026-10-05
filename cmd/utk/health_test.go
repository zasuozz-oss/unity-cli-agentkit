package main

import (
	"strings"
	"testing"
)

func TestReloadTimes(t *testing.T) {
	log := "Begin MonoManager ReloadAssembly\n" +
		"Domain Reload Profiling: 471ms\n" +
		"\tReloadAssembly (300ms)\n" +
		strings.Repeat("x", 200_000) + "\n" + // one stack trace line longer than any buffer
		"Domain Reload Profiling: 2979ms\n" +
		"[Log] Domain Reload Profiling: 99ms mentioned mid-line\n" +
		"Domain Reload Profiling: 34500ms" // no trailing newline
	got := reloadTimes(strings.NewReader(log))
	if len(got) != 3 || got[0] != 471 || got[1] != 2979 || got[2] != 34500 {
		t.Fatalf("reloadTimes = %v", got)
	}
}

func TestReloadHealth(t *testing.T) {
	for _, c := range []struct {
		name     string
		ms       []int
		degraded bool
	}{
		{"fresh editor", []int{500, 3000, 1700, 2600, 2400, 2800, 3100, 2900, 3000}, false},
		{"leaked editor", []int{500, 3000, 1700, 2600, 2400, 9000, 15000, 24000, 31000, 34000, 35000}, true},
		// A big project reloads slowly from the first minute: restarting it
		// would change nothing, so it must not restart forever.
		{"slow but stable", []int{11000, 12000, 12500, 11800, 12100, 12900, 13000, 12700, 12800}, false},
		{"too few reloads to judge", []int{2000, 30000, 31000}, false},
		{"one slow reload is not a trend", []int{2000, 2100, 2200, 2000, 2100, 2300, 2200, 2100, 40000}, false},
	} {
		if _, _, got := reloadHealth(c.ms); got != c.degraded {
			t.Errorf("%s: degraded = %v, want %v", c.name, got, c.degraded)
		}
	}
	last, base, _ := reloadHealth([]int{500, 3000, 1700, 2600, 2400, 9000, 15000, 24000, 31000, 34000, 35000})
	if last != 34000 || base != 2400 {
		t.Errorf("last/base = %d/%d, want 34000/2400", last, base)
	}
}

func TestParseEditorPS(t *testing.T) {
	ps := `  711 /Applications/Unity Hub.app/Contents/MacOS/Unity Hub
69861 /Applications/Unity/Hub/Editor/6000.0.84f1/Unity.app/Contents/MacOS/Unity -adb2 -batchMode -noUpm -name AssetImportWorker0 -projectPath /Users/z/Game/Echo-Pals -logFile Logs/AssetImportWorker0.log
45504 /Applications/Unity/Hub/Editor/6000.0.84f1/Unity.app/Contents/MacOS/Unity -projectpath /Users/z/Game/Echo-Pals-a4 -useHub -hubIPC
36872 /Applications/Unity/Hub/Editor/6000.0.84f1/Unity.app/Contents/MacOS/Unity -projectpath /Users/z/Game/Echo-Pals -acceptSoftwareTermsForThisRunOnly -useHub
36885 /Applications/Unity/Hub/Editor/6000.0.84f1/Unity.app/Contents/Resources/PackageManager/Server/UnityPackageManager -s 36872
`
	pid, bin := parseEditorPS(ps, "/Users/z/Game/Echo-Pals")
	if pid != 36872 || bin != "/Applications/Unity/Hub/Editor/6000.0.84f1/Unity.app/Contents/MacOS/Unity" {
		t.Fatalf("got %d %q", pid, bin)
	}
	if pid, _ := parseEditorPS(ps, "/Users/z/Game/Other"); pid != 0 {
		t.Fatalf("matched an Editor of another project: %d", pid)
	}
	// A shell or a hook whose command line only quotes the Editor's is not the
	// Editor (both seen live): `editor restart` would quit nothing and kill it.
	noise := `53672 /bin/zsh -c source snap.sh && pgrep -f "MacOS/Unity -projectpath /Users/z/Game/Echo-Pals " | head -1
98131 /opt/homebrew/bin/agy --print-timeout 90s -p Input: /Applications/Unity/Unity.app/Contents/MacOS/Unity -projectPath /Users/z/Game/Echo-Pals
`
	if pid, bin := parseEditorPS(noise, "/Users/z/Game/Echo-Pals"); pid != 0 {
		t.Fatalf("took %d %q for the Editor", pid, bin)
	}
	if pid, _ := parseEditorPS(noise+ps, "/Users/z/Game/Echo-Pals"); pid != 36872 {
		t.Fatalf("noise first: got %d, want the Editor 36872", pid)
	}
	// Launched by hand: -projectPath, and the project is the last argument.
	pid, bin = parseEditorPS("  9 /Apps/My Unity/Unity -projectPath /p/My Game\n", "/p/My Game")
	if pid != 9 || bin != "/Apps/My Unity/Unity" {
		t.Fatalf("spaces: got %d %q", pid, bin)
	}
}
