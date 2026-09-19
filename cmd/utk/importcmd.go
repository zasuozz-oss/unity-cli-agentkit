package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// prepImportArgs turns `utk import <file> [dest]` positionals into the
// --source/--path flags import_asset expects. The tool requires --source to
// be an absolute path and reports a missing file only after a round-trip to
// the Editor — resolve and stat locally so a typo fails here in milliseconds
// instead. Explicit --source passes through untouched (the flag form is the
// tool's own interface and needs no translation).
func prepImportArgs(args []string, stderr io.Writer) ([]string, int) {
	if hasFlag(args, "--source") {
		return args, 0
	}
	var pos, flags []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			flags = append(flags, a)
			// Every import_asset flag takes a value; keep it with its flag so
			// `--confirm true` is not read as a destination path.
			if !strings.Contains(a, "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) < 1 || len(pos) > 2 {
		fmt.Fprintln(stderr, "utk: usage: utk import <file> [dest-under-Assets] [--confirm true] [--dry_run true]")
		return nil, 2
	}
	src, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintln(stderr, "utk: import:", err)
		return nil, 2
	}
	if _, err := os.Stat(src); err != nil {
		fmt.Fprintln(stderr, "utk: import: no such file:", pos[0])
		fmt.Fprintln(stderr, "  nothing was sent to the Editor")
		return nil, 2
	}
	dest := filepath.Base(src)
	if len(pos) == 2 {
		dest = pos[1]
	}
	return append([]string{"--source", src, "--path", dest}, flags...), 0
}
