---
name: utk-test-runner
description: Use when you have edited C# in a Unity project and need to confirm it compiles, surface compile errors, or run the project's EditMode/PlayMode tests before considering the change done.
---

# Unity Compile Check & Test Runner

After editing C#, two questions decide if you're done: **does it compile?** and
**do the tests pass?** Drive both through `utk` and read only what matters.

All snippets below are **bash**. On Windows, run them through bash (Bash tool /
`bash -lc`), never PowerShell — PowerShell 5.1 strips embedded double quotes
from `utk exec` arguments, and the reload-wait loop below is bash syntax.

## Verify it compiles
Auto Refresh is off, so edits don't compile until you ask. After a **batch** of C#
edits (once, not per file), trigger a recompile, wait for it, then read errors:

```bash
utk editor refresh                # compiles, waits for it, prints the errors
```

That is the whole check. `refresh` blocks until the compile finishes (up to 5
minutes) and answers with the compiler's own report, so there is no
`recompile_status` poll and no follow-up `utk console` — reading the console
right after a refresh used to hand back the *previous* compile's errors anyway.
It **exits non-zero when the compile failed**, so `utk editor refresh &&
utk run_tests --mode editor` stops at a broken build instead of testing it.
**Do not report "done" until it exits clean.** If you only need to check a
one-off C# snippet, `utk exec '<csharp>'` compiles it on the spot and reports
the error inline — no refresh needed.

## Run tests

```bash
utk run_tests --mode editor --filter <name>  # default choice: works with the Editor open
utk run_tests --mode playmode               # PlayMode (triggers a domain reload)
utk list_tests                              # discover test names/assemblies
utk test_status                             # poll an async run
utk test                                    # batchmode runner — Editor must be CLOSED
```

**Use `utk run_tests`.** It goes through the tool API in the running Editor, so
its output is envelope-stripped down to a summary plus the failures, and it
**exits non-zero when any test fails** — safe to chain with `&&`. `--mode` is
`all | editor | playmode` (default `all`) and `--filter` is a case-insensitive
partial match on the test name, so target an assembly or class by a fragment of
its name.

A suite longer than 30s used to fail: `unity command` waits 30s and gives up
while the Editor keeps running the run, leaving it unreachable behind the
failure. `utk run_tests` now hands off asynchronously and polls `test_status`
for up to 10 minutes, so the results still arrive on stdout — pass
`--async_tests true` yourself if you want the hand-off without the wait.

The Bash tool, however, waits only 120s by default and then backgrounds the
command — after which the temptation is a `until grep … sleep` loop over its
output file (measured: 601s of it). For a whole assembly or PlayMode, start
the call with `run_in_background: true` from the outset and let the harness
notify you; `utk` prints a reminder at 90s if you did not.

`utk test` drives the official `unity test` batchmode runner (`utk test --help`
for its flags), whose own JSON reports only where it wrote the NUnit report —
so `utk` reads that report and renders it like `run_tests`: a summary line plus
the failures, passing test names dropped (measured: 158KB down to 1KB on a
212-test suite). Batchmode needs exclusive access to the project, so with the
Editor open it aborts with `Multiple Unity instances cannot open the same
project` and a `stopped with signal SIGABRT` note. Only reach for it when the
Editor is closed, e.g. in CI.

Its exit code tells you which kind of bad news you got: **8** means tests ran
and failed, **6** means the run never reached a verdict at all — a compile
error, an expired license, the SIGABRT above. Retry a 6; never retry an 8.

Requires the Unity Test Framework package. PlayMode tests reload the domain —
let the run finish.

## Stale results trap (Auto Refresh off)

With Auto Refresh **off**, a test run can silently execute the **previously
loaded** test assembly even after the recompile reports complete and the new DLL
is on disk. Compilation finishes before the *domain reload* that swaps the
running IL — so EditMode tests race against the reload and execute the old code
(and old referenced assemblies).

**How to spot it:** the failure/value doesn't match an edit you KNOW you saved —
e.g. you changed an expected number or added an assert message, but the result is
byte-identical to before. (`Assembly.Location` pointing at the fresh DLL path does
**not** prove the in-memory IL is current.)

**Reliable fix — force a real domain reload and wait for it before testing:**

```bash
utk editor refresh                                 # compile to disk, waits for it
utk exec 'UnityEditor.EditorUtility.RequestScriptReload(); return "reloading";'
# wait for a genuine reload cycle: the pipeline server goes DOWN, then comes back UP
down=0
for i in $(seq 1 20); do
  if utk status 2>&1 | grep -q reachable; then   # case-sensitive: UNREACHABLE won't match
    [ "$down" -eq 1 ] && break                   # back UP after going DOWN = reload finished
  else
    down=1                                       # domain is reloading
  fi
  sleep 1
done
utk run_tests --mode editor --filter <ClassName>   # now runs the fresh assembly
```

Prefer a **broad** filter (whole test class, not one single-test name) for the
run after a reload. If results still look stale, repeat the reload-and-wait once
before concluding anything about the code.

**Ground-truth shortcut for pure C# logic:** to verify a Core/non-MonoBehaviour
method without fighting the test runner at all, call it directly with `utk exec`
against the loaded assembly — it always reflects the current domain:

```bash
utk exec 'var r = MyNs.MyClass.MyMethod(args); return "result=" + r;'
```

(`utk exec` snippets must `return` a value, must **not** put `using` directives in
the body — that throws "Identifier expected" — and should use fully-qualified type
names.)

## Loop
Edit batch → `utk editor refresh` → fix the errors it prints
→ (force reload + wait, see **Stale results trap**) → `utk run_tests --filter <name>`
→ repeat until the summary shows 0 failed. If a failure won't budge and matches the
pre-edit result exactly, suspect a stale assembly before suspecting your code:
reload-and-wait, or confirm the logic directly with `utk exec`. Don't blind
fix-and-retry the same error more than twice — if it still fails, stop and report.
