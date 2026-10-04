---
name: utk-test-runner
description: Use when you have edited C# in a Unity project and need to confirm it compiles, surface compile errors, or run the project's EditMode/PlayMode tests before considering the change done — offline with dotnet first, the Editor only for what needs it, and only your own tests.
---

# Unity Compile Check & Test Runner

After editing C#, two questions decide if you're done: **does it compile?** and
**do the tests pass?** Answer both offline first (`dotnet`, no Editor), and go
to the Editor through `utk` only for what offline cannot cover.

## Order of checks (rules)
1. **Offline, every time:** `<this skill>/scripts/unity-test.sh offline <repo>
   --tests <YourTestClass>` — compile + your own logic tests, ~10s, no Editor,
   no queue behind other agents. Its `passed`/`FAIL` on a logic test is the
   answer; the Editor adds nothing to it.
2. **Editor only when needed:** tests offline counts `SKIPPED-NEEDS-EDITOR`,
   PlayMode, scenes/assets/prefabs, a visual check, or a `NOT-IN-CSPROJ` file
   (a new .cs only the Editor's project sync adds). Then `utk editor refresh &&
   utk run_tests --mode editor --filter <YourTestClass>`.
3. **Only your own tests, in both places.** Filter by your test class (or a
   `[Category]`). Never a namespace (`<Game>.Tests` matches every test),
   `--filter_type assembly` on the game's test assembly, or no filter — that is
   the whole suite (1000+ tests, 1–3 min in the Editor) and every other agent
   on that Editor waits behind it. The full suite is for the end of a task, by
   one agent, or when the user asks.
4. **Write tests that can run offline.** Measured on a 1055-test game: 95 ran
   offline, 958 needed the Editor because they touch `GameObject`, scenes,
   `JsonUtility` and other native Unity code. Put rules and numbers in plain C#
   classes (no `UnityEngine` types in the signature) and test those; keep the
   MonoBehaviour a thin shell. Those tests then cost seconds, not an Editor slot.

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

## Without the Editor (default first step)
**Rule: always run `<this skill>/scripts/unity-test.sh offline <repo>` first, every time.** It needs no Unity Editor (neither open nor batchmode): `dotnet build` on the csproj files Unity generated, ~3 s, prints CS errors and `COMPILE OK` / `COMPILE FAILED`. A file missing from the csproj is reported as `NOT-IN-CSPROJ` (open the Editor once to regenerate).

`offline <repo> --tests [<filter>]` also runs the built EditMode test assemblies with NUnitLite on plain .NET, still without Unity. `<filter>` is a regex on the test's full name, like `run_tests --filter` (a class name is enough): `offline <repo> --tests CatchGameTests`. Tests that call native UnityEngine/UnityEditor code cannot run there and are counted as `SKIPPED-NEEDS-EDITOR`, not failures; only real failures exit 1 (a test that swallows the native error can still show as a false FAIL, e.g. a `JsonUtility` fixture decode: re-check it in the Editor). `no test matched filter` means the name is wrong, not that the tests passed.

Use the Editor or batchmode only when offline cannot cover it (PlayMode, scenes/assets, tests that need the Unity runtime):
`<this skill>/scripts/unity-test.sh compile|editmode|playmode <repo>` (batchmode; the Editor must be closed for that project).

Details and gotchas: `references/headless-testing.md`.

## Run tests

```bash
utk run_tests --mode editor --filter <YourTestClass>  # after offline, for what needs the Editor
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
its name. Its `--timeout` counts **seconds** (default 300) — unlike `exec`,
which counts milliseconds.

The compact output names each failure with its message and first frame. For a
full stack trace, re-run that one test with `--raw`, or read the NUnit report
`TestResults.xml` the run writes under `Application.persistentDataPath`.

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

If the Test Framework itself dies mid-run (`An unexpected error happened while
running tests`, often with `Test tree is not available for
PostbuildCleanupTask`), `test_status` would say `running` forever. `utk
run_tests` watches the console for that entry, cancels the run and fails with
`TEST_RUN_CRASHED` within ~15s — check `utk editor status`, then retry once.

A run that dies *without* that entry looks different: no result after ~3
minutes on a suite that normally takes one, and the Unity process at ~100%
CPU. A test is spinning the main thread. It will not finish, and polling
`test_status` for 15 minutes only confirms it — see utk-cli-core, "Same
symptoms at ~100% CPU".

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

**Stop play mode first.** With the Editor in play mode, a queued compile waits
for it to end, so `run_tests` (and `editor refresh`) hang to their timeout
instead of failing: `utk editor stop; utk editor refresh && utk run_tests …`.

Chain refresh and tests with `&&`, never `;`. After a failed compile the
Editor keeps the previous assembly, so `refresh; run_tests` runs the old code
and can report green for a change that does not compile.

## One Editor, one test run

The pipeline keeps **one** test run and one result file per Editor. Before
`utk` waited, every `run_tests` cancelled the run in flight ("Invalidated
previous test run" in Editor.log), and the cancelled agent then read the other
agent's report as its own — a one-test filter came back with 1041 tests.

- `utk run_tests` now checks `test_status` first and, while another run is in
  progress, **waits** for it (stderr: "another test run … waiting") instead of
  cancelling it — up to 10 minutes, then it stops without starting yours. A
  run that crashed and never reported keeps saying `running`: `utk
  cancel_tests`, then retry.
- A report holding a test your `--filter` (testName) cannot match fails with
  **`TEST_RESULT_FOREIGN`**: another agent's run replaced yours. Rerun; do not
  read anything from that report. Assembly/category filters cannot be checked.
- Do not pass `--async_tests false`: a synchronous run is invisible to that
  wait, hits the CLI's 30s cap and blocks the Editor's main thread.
- Several agents need Editor tests at once → queue through `unity-job.sh`
  (utk-cli-core) or give each agent its own Editor (unity-parallel-branch).

## Tests share the Editor

`run_tests` runs inside the Editor the user may be playing in, against the
same PlayerPrefs, save files and static singletons. A test that calls
`PlayerPrefs.DeleteKey`/`DeleteAll` in `SetUp`/`TearDown` wipes the user's real
progress on every run — this happened, silently, for a whole session.

```csharp
string _saved; bool _had;
[SetUp]    public void SetUp()    { _had = PlayerPrefs.HasKey(Key); _saved = PlayerPrefs.GetString(Key); }
[TearDown] public void TearDown() { if (_had) PlayerPrefs.SetString(Key, _saved); else PlayerPrefs.DeleteKey(Key); }
```

Snapshot every global a test touches and restore it; never `DeleteAll`. A test
that needs a clean slate uses its own key prefix, not the game's.

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
Edit batch → `unity-test.sh offline <repo> --tests <YourTestClass>` → fix what it
prints → only if something needs the Editor: `utk editor refresh &&
utk run_tests --mode editor --filter <YourTestClass>` (force reload + wait first,
see **Stale results trap**) → repeat until 0 failed. If a failure won't budge and matches the
pre-edit result exactly, suspect a stale assembly before suspecting your code:
reload-and-wait, or confirm the logic directly with `utk exec`. Don't blind
fix-and-retry the same error more than twice — if it still fails, stop and report.
