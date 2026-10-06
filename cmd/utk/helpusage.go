package main

// localUsage is the --help text of the verbs utk handles itself (they never reach the official CLI,
// so `list <tool>` has no schema for them). "" = not a local verb.
func localUsage(cmd string, rest []string) string {
	switch cmd {
	case "init":
		return initUsage
	case "queue":
		return "usage: utk queue submit compile | test --filter <Class> | scene --cmd '<utk args>'|--script <f.sh> | shot --script <f.sh> --scene <path>\n" +
			"       utk queue status | stats [--since 24h] | cancel <id> | serve | play-check\n" +
			"shared-Editor queue; submit blocks until YOUR result (exit = its exit; 75 = BLOCKED by a Play no job owns). See `utk --help`.\n"
	case "editor":
		if len(rest) == 0 {
			return ""
		}
		switch rest[0] {
		case "wait":
			return "usage: utk editor wait [--timeout S] [--project-path P]\nblock until the Editor answers and is neither compiling nor reloading (default 300 s; exit 124 on timeout)\n"
		case "restart":
			return "usage: utk editor restart [--force]\nquit the Editor, start it again, wait until ready. Refuses while playing or with unsaved scenes unless --force. Needs the Editor lock: it takes the shared Editor down for ~1-2 min.\n"
		case "play":
			return "usage: utk editor play [--force]\nenter Play. Needs the Editor lock (UNITY_LOCK_OWNER = holder) or a queue job; --force is for the user / manual use only.\n"
		case "stop":
			return "usage: utk editor stop\nleave Play. Never stop a Play your job did not start; warns when it ends a Play nobody owns.\n"
		}
	}
	return ""
}
