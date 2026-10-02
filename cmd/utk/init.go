package main

import (
	"fmt"
	"os"

	"github.com/zasuo/unity-cli-agentkit/internal/initcmd"
)

const initUsage = `usage: utk init [--uninstall]
  (run from a Unity project root)
  utk init              install/refresh the kit's skills in .claude/skills, the
                        CLAUDE.md/AGENTS.md guidance block, and com.unity.pipeline
                        — replaces any local edits to the installed skills
  utk init --uninstall  remove the project pointers
`

func runInit(args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "utk:", err)
		return 1
	}
	// init rewrites the installed skills and the CLAUDE.md/AGENTS.md block, so
	// an agent probing `utk init --help` must not get a real run: answer help,
	// refuse anything else unknown before touching the project.
	for _, a := range args {
		switch a {
		case "--help", "-h", "help":
			fmt.Fprint(os.Stdout, initUsage)
			return 0
		case "--uninstall":
		default:
			fmt.Fprintf(os.Stderr, "utk init: unknown argument %q\n%s", a, initUsage)
			return 2
		}
	}
	for _, a := range args {
		if a == "--uninstall" {
			if err := initcmd.Uninstall(cwd); err != nil {
				fmt.Fprintln(os.Stderr, "utk init:", err)
				return 1
			}
			fmt.Fprintln(os.Stderr, "utk: uninstalled project pointers")
			return 0
		}
	}
	kit, err := initcmd.KitHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, "utk init:", err)
		return 1
	}
	if err := initcmd.Run(cwd, kit); err != nil {
		fmt.Fprintln(os.Stderr, "utk init:", err)
		return 1
	}
	// Success first, then the one remaining step. Leading with the pipeline
	// warning reads as a failed init when init in fact succeeded — and the
	// "focus the Editor" note is nonsense when the package was never installed,
	// so print exactly one of the two tails.
	fmt.Fprintln(os.Stderr, "utk: OK - installed skills + CLAUDE.md/AGENTS.md guidance in", cwd)
	if err := initcmd.InstallPipeline(cwd); err != nil {
		fmt.Fprintln(os.Stderr, "utk: next step - com.unity.pipeline was NOT installed:", err)
		fmt.Fprintln(os.Stderr, "  once the official CLI is on PATH, run: unity pipeline install")
		return 0
	}
	fmt.Fprintln(os.Stderr, "utk: next step - if the Unity Editor is already open, focus its window once")
	fmt.Fprintln(os.Stderr, "  (or reopen the project) so com.unity.pipeline resolves, then verify")
	fmt.Fprintln(os.Stderr, "  with: utk status")
	return 0
}
