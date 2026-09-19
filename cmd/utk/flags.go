package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zasuo/unity-cli-agentkit/internal/schema"
)

// checkFlags refuses a flag the target tool does not accept. It only inspects
// `unity command <tool> …` argvs, and only when there is a flag to inspect, so
// the common flagless call never touches the cache. Anything it cannot verify —
// no cache, unknown tool — passes through: blocking a valid command would be a
// worse failure than the silent drop this guards against.
func checkFlags(unityArgs []string, stderr io.Writer) int {
	if len(unityArgs) < 2 || unityArgs[0] != "command" {
		return 0
	}
	args := unityArgs[2:]
	if !anyFlag(args) {
		return 0
	}
	// The catalogue is the CLI's own view of the tools. A project on a newer
	// com.unity.pipeline can expose parameters that view has not caught up with,
	// so leave a way to send the argv through untouched.
	if os.Getenv("UTK_NO_FLAG_CHECK") != "" {
		return 0
	}
	cat := schema.Load(stderr)
	if cat == nil {
		return 0
	}
	flag, near := cat.Unknown(unityArgs[1], args)
	if flag == "" {
		return 0
	}
	fmt.Fprintf(stderr, "utk: %s does not accept %s (the official CLI would ignore it silently)\n",
		unityArgs[1], flag)
	if len(near) > 0 {
		fmt.Fprintln(stderr, "  did you mean:", strings.Join(near, ", "))
	}
	fmt.Fprintf(stderr, "  see: utk list %s  (or set UTK_NO_FLAG_CHECK=1 to skip this check)\n", unityArgs[1])
	return 2
}

func anyFlag(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			return true
		}
	}
	return false
}
