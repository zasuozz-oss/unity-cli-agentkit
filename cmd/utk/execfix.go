package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
)

// ambiguityError is the compile failure this file exists for. The eval wrapper
// has both `using System;` and `using UnityEngine;` in scope, so the bare
// `Object.FindAnyObjectByType<T>()` that every Unity sample and every piece of
// project code is written with does not compile inside a snippet — `Object`
// matches `object` just as well. Nothing in the message says which `Object` to
// prefer, and the fix is mechanical.
const ambiguityError = "'Object' is an ambiguous reference"

// bareObject matches a qualifier-less `Object.` member access. The leading
// class excludes `.` so `UnityEngine.Object.` and `go.Object.` are left alone,
// and excludes word characters so `MyObject.` is not rewritten mid-identifier.
var bareObject = regexp.MustCompile(`(^|[^\w.])Object\.`)

// ambiguousObject reports whether a raw envelope carries the ambiguity error.
func ambiguousObject(raw []byte) bool {
	return bytes.Contains(raw, []byte(ambiguityError))
}

// qualifyObject rewrites bare `Object.` to `UnityEngine.Object.`, reporting
// whether anything changed.
func qualifyObject(src string) (string, bool) {
	out := bareObject.ReplaceAllString(src, "${1}UnityEngine.Object.")
	return out, out != src
}

// qualifyObjectArgs rebuilds an eval/eval_file argv with the snippet's bare
// `Object.` references qualified, for one retry after the ambiguity error. It
// returns ok=false when there is nothing to rewrite, so a failure with some
// other cause is reported as-is rather than retried in a loop.
//
// cleanup removes the temporary file the eval_file form needs; it is never nil
// when ok is true.
func qualifyObjectArgs(args []string) (out []string, cleanup func(), ok bool) {
	if len(args) < 3 || args[0] != "command" {
		return nil, nil, false
	}
	switch args[1] {
	case "eval":
		if strings.HasPrefix(args[2], "--") {
			return nil, nil, false
		}
		fixed, changed := qualifyObject(args[2])
		if !changed {
			return nil, nil, false
		}
		out = append([]string(nil), args...)
		out[2] = fixed
		return out, func() {}, true
	case "eval_file":
		path := findFlag(args, "--file")
		if path == "" {
			return nil, nil, false
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, false
		}
		fixed, changed := qualifyObject(string(src))
		if !changed {
			return nil, nil, false
		}
		// The caller's file is left untouched: the rewrite is a guess utk makes
		// on its behalf, not an edit it asked for.
		tmp, err := os.CreateTemp("", "utk-exec-*.cs")
		if err != nil {
			return nil, nil, false
		}
		if _, err := tmp.WriteString(fixed); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return nil, nil, false
		}
		tmp.Close()
		out = append([]string(nil), args...)
		for i, a := range out {
			if a == "--file" && i+1 < len(out) {
				out[i+1] = tmp.Name()
				break
			}
			if strings.HasPrefix(a, "--file=") {
				out[i] = "--file=" + tmp.Name()
				break
			}
		}
		return out, func() { os.Remove(tmp.Name()) }, true
	}
	return nil, nil, false
}
