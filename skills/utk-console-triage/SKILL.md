---
name: utk-console-triage
description: Use when you need to read, triage, or debug Unity Editor errors, warnings, or console output — NullReferenceException, compile errors, runtime exceptions, or repeated/noisy log spam in a Unity project.
---

# Unity Console Triage

Read logs through `utk console` so duplicates collapse and long stacktraces are
trimmed to first + last frame automatically.

## Recipes
- Errors only, recent: `utk console --type error --limit 50`
- Warnings only: `utk console --type warning`
- Last N logs of any kind: `utk console --limit 20` (default type is `all`)
- Full untrimmed output (debugging the filter): add `--raw`

`--type` takes **one** value — `all`, `log`, `warning` or `error` (it maps to
the official `--severity`); there is no comma-separated form. Always
`--type error` first; expand to warnings/all only when needed. Don't re-read
logs if the previous compile was already clean.

If `utk console` itself cannot connect while an Editor is open, the Editor may
have **booted into Safe Mode** from compile errors — the console is unreadable
exactly when you need it. Check `utk status` and follow the Safe Mode recovery
loop in utk-cli-core (read the errors from `Logs/Editor.log` instead).

## What utk does for you
- Identical entries collapse into one line headed `[Error ×N] <message>`.
- **Compile errors are grouped by root cause** — one bad type name reports at
  every site that used it, so 60 lines collapse to the handful of mistakes
  behind them:

  ```
  console: 61 returned; compile: 58 errors, 3 warnings
  [error CS0246 ×20] The type or namespace name 'Bar' could not be found
   Assets/Scripts/Foo.cs(12,20) +19 more sites
  ```

- Compile errors come **first error first**. The underlying tool answers
  newest-first, so the causal first error is the one a plain `--limit` drops.
- **Warnings stay warnings.** An obsolete API or an unreachable statement is
  labelled `[warning CS0618 …]` and counted separately; it never fails a build.
- Stacktrace runs longer than 2 frames keep the first and last, replacing the
  middle with `… N frames omitted`.
- Timestamps are dropped; every unique message survives verbatim.
- Output that isn't the expected JSON shape is passed through unchanged — no
  data is ever silently dropped.

## Before you fix
Open the relevant `file:line` and read it — never propose a fix from a guess.
Fix the **first** grouped error first: the ones under it are usually its
fallout and disappear with it. Don't address each line separately.

For an error with more than one reasonable fix, report concisely **before** acting:
1. **Error** — the exact error line + `file:line` (not the whole stack trace).
2. **Root cause** — one sentence.
3. **Options** — 2–3 approaches, each with its trade-off.
4. **Recommendation** — which to pick and why, one sentence.

Trivial errors (missing `;`, wrong name, missing `using`) — skip the report, fix
directly. Don't blind-loop fix-and-retry more than twice on the same error; if the
2nd attempt still fails, stop and report what you already tried.

After fixing, run `utk editor refresh` — it waits for the compile and prints
the errors that remain, exiting non-zero while any are left. It reports compile
errors only; for runtime or import errors, re-read `utk console --type error`.
