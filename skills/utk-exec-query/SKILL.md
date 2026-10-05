---
name: utk-exec-query
description: Use when you need to inspect or change live Unity state — GameObjects, components, scene hierarchy, ECS entities, assets — or evaluate arbitrary C# inside a running Unity Editor.
---

# Unity Exec — Token-Lean Queries

Use `utk exec '<csharp>'`. It compiles the snippet with Roslyn inside the
Editor — no domain reload — and returns the value as compact JSON. Shape data
**inside Unity** so little crosses the wire; this is the biggest token saver.

**On Windows, run `utk exec` through bash** (Bash tool / `bash -lc`), never
PowerShell — PowerShell 5.1 strips the embedded double quotes from the C#
argument, so string literals arrive corrupted. Wrap the snippet in single
quotes and use double quotes for C# strings. For anything longer than a line
or two, put it in a file and use `utk exec --file <snippet.cs>` — that sidesteps
the quoting rules of every shell at once, and never do `utk exec "$(cat f.cs)"`.
`--file` is resolved from the **project root**, not the shell's cwd — pass an
absolute path (`File not found: x.cs` after a `cd` is this).

Feeding the value to a script? `UTK_NO_EXEC_LOGS=1 utk exec …` prints the
returned value alone. Folded console lines (including warnings the Editor
raised while the snippet ran) otherwise follow it — never parse `--raw` with
`grep -o '"result":"…"'`, which cuts the value at its first quote.

## The snippet's contract

A snippet is a **method body**, not a file. Three consequences, and they cause
most first-attempt failures:

- `using` directives are **invalid** — a `using System;` line is parsed as a
  `using` *statement* and produces a pile of unrelated errors.
- Only `UnityEngine` and `UnityEditor` are in scope. Everything else is written
  in full: `UnityEngine.UI.Image`, `TMPro.TextMeshProUGUI`,
  `System.Linq.Enumerable.Select(...)`.
- The last statement must `return` the value you want back.

Three more that the compiler reports without naming the cause — measured as the
most frequent failures after the three above:

- **Bare `Object.` is ambiguous** (`'Object' is an ambiguous reference`): both
  `System` and `UnityEngine` are in scope, so `Object` matches `object` too.
  Write `UnityEngine.Object.DestroyImmediate(...)`. `utk` retries a snippet that
  fails this way with the name qualified, but the retry costs a round-trip you
  can skip.
- **`FindAnyObjectByType<T>(true)` / `FindFirstObjectByType<T>(true)` do not
  exist** in Unity 6 — the flag is an enum (CS1503 `cannot convert from 'bool'`):
  `UnityEngine.Object.FindFirstObjectByType<T>(UnityEngine.FindObjectsInactive.Include)`.
  The `Object.` auto-retry only rewrites statements; in a local function's
  signature (`void Set(Object o)`) write `UnityEngine.Object` yourself.
- **Extension methods from other namespaces don't resolve** — there is no
  `using`, so `task.Forget()` fails with CS1061 (`'UniTask' does not contain a
  definition for 'Forget'`). Call the static form:
  `Cysharp.Threading.Tasks.UniTaskExtensions.Forget(task)`,
  `System.Linq.Enumerable.Where(list, x => …)`. Same for any project extension.
- **"The type 'X' exists in both 'A' and 'B'"** (CS0433) — the snippet sees
  every assembly, and some packages embed a copy (`JsonConvert` is in
  `Newtonsoft.Json` *and* `Unity.Localization.ThirdParty.Editor`). Go through
  reflection: `System.Type.GetType("Newtonsoft.Json.JsonConvert, Newtonsoft.Json")`
  — the assembly-qualified name picks one.
- **`INVALID_COMMAND_ARGS … timeout is already set by --timeout, so there is
  no slot for <a fragment of your code>`** — a C# char literal (`' '`, `'/'`)
  closed the shell's single quotes and split the snippet into several
  arguments. Use `"…"` strings in the snippet, or put it in a file and run
  `utk exec --file`.
- **`UniTask<T>` has no `.Forget()`** — only the non-generic `UniTask` does.
  `await` it, or discard the result first.
- **`dynamic` does not compile** (`Missing compiler required member
  'Microsoft.CSharp.RuntimeBinder…'`) — the binder assembly is not referenced,
  in `exec` or `run_script`. Use `object` and cast, or reflection.
- **Scene APIs are mode-specific** — `EditorSceneManager` in edit mode,
  `SceneManager` in play mode; the wrong one throws. See utk-playmode-driving.

`utk` drops the "Unreachable code detected" warning every failed compile
carries: the wrapper appends its own `return` after your body, so it is never
about the snippet.

## Printing vs returning

`return` is the answer; `UnityEngine.Debug.Log` also comes back — `utk` folds
the snippet's own console entries into the output, under the returned value:

```
42
[Log] rebuilt 12 prefabs
```

Prefer `return` for data (it stays JSON) and `Debug.Log` for progress inside a
loop.

## UI layout: measure, don't look

A screenshot answers "does it look right", never "is it the right size". Dump
the numbers; one exec replaces a round of screenshot-and-squint:

```csharp
var list = new System.Collections.Generic.List<string>();
foreach (var r in UnityEngine.GameObject.Find("Canvas")
        .GetComponentsInChildren<UnityEngine.RectTransform>(true))
    list.Add(r.name + " " + r.rect.width + "x" + r.rect.height);
return list;
```

**`AddComponent` defaults are not the Inspector's defaults.** Measured:
`AddComponent<UnityEngine.UI.HorizontalLayoutGroup>()` gives
`childControlWidth/Height = false` and `childForceExpandWidth/Height = true`,
and calling `Reset()` does *not* change them — so children keep their own size
(a 100×100 element stays 100×100) instead of being driven by the group. Set
every flag you depend on explicitly right after `AddComponent`.

## Principles
- **Batch same-type operations into one snippet.** Each `utk exec` call costs
  a full agent round-trip (inference + queueing on Unity's main thread), so
  one C# loop that renames 20 objects beats 20 separate exec calls by an
  order of magnitude in wall time.
- Return a **count + a few samples**, not the full collection:
  `utk exec 'return new { count = items.Length, sample = items.Take(5) };'`
- Project only the fields you need:
  `utk exec 'return objs.Select(o => new { o.name, o.tag });'`
- `utk` compacts JSON whitespace automatically (loss-safe). It does **not**
  drop array elements unless you opt in with `--truncate N`.
- Use `--truncate N` only when you knowingly accept a shortened array; the
  marker `"…+K more"` shows what was cut.
- Need the literal full result? Append `--raw`.
- Before reaching for exec on assets: deterministic text changes (renames,
  `m_Name:`, field values, GUID swaps) are faster as direct YAML/file edits —
  see utk-asset-edit. Keep exec for structural changes and live state.

## Timeouts & latency

Exec runs on Unity's **main thread**: the command executes only while the
editor loop is pumping. Typical timeout causes, in order of likelihood:

- **Script compilation / domain reload** in progress — in-flight requests are
  dropped; `utk` retries them. A reload that takes 3 s on a fresh Editor takes
  30 s+ on one that has been up for a day, and then every call around a
  compile, a test run or a Play times out. Run `utk editor status` first: it
  says `the Editor has degraded …` when that is the cause, and
  `utk editor restart` fixes it (utk-cli-core). This, not the next cause, was
  behind a full day of timeouts across seven agents.
- **Modal dialog or progress bar** open in the editor.
- **Editor backgrounded** — the OS can throttle an unfocused Unity. Rule it
  out before blaming it: Preferences → General → Interaction Mode =
  No Throttling keeps the loop pumping with the window in the back.
  In play mode, step frames yourself instead (utk-playmode-driving).
  Game frozen while `utk` still answers → Run In Background is off; see the
  hang triage in utk-playmode-driving.

Waiting for the Editor is `utk editor wait [--timeout S]`, and a hard deadline
on any call is `utk --max-time <seconds> <verb> …` (exit 124) — never a
`sleep` loop or `perl -e 'alarm …'`.

The budget is `--timeout <ms>`, **default 60000** — raise it (e.g.
`--timeout 300000`) for a snippet that legitimately does heavy work, rather
than retrying a timeout. `--timeout 120` is 120 *milliseconds*; utk refuses
values under 1000.

**A timeout does not mean the snippet did nothing.** `400 … Main thread
operation timed out` is the reply giving up; the main thread may finish the
work afterwards (scene created, textures reimported). Check the state the
snippet was meant to produce before re-running it — a blind re-run of a build
script builds twice. Import-heavy snippets (hundreds of reimports) have also
died at 30000ms regardless of `--timeout`: batch them (~50 assets per call,
inside `AssetDatabase.StartAssetEditing`/`StopAssetEditing`) or use
`run_script --timeout_ms`. utk routes the one value to both the CLI transport
and the eval tool, so never pass a raw `unity command eval --timeout` yourself:
the CLI keeps that flag (in seconds) and the tool falls back to its enforced
5000ms default.

`run_script` is an official tool, not a utk verb, and has its own budget:
`--timeout_ms` (default 30000). A batch job over hundreds of assets needs
`--timeout_ms 300000`; `--timeout` does not apply to it.

### Fire-and-forget async stalls, and lies about it

A snippet that kicks off async work (UniTask/`Task`) without awaiting it stalls
**silently** when the editor is backgrounded: continuations never pump, so
there is no log, no exception, and no partial result — `utk` just times out
with "cannot reach Unity health endpoint", which reads like a dead connection
rather than a throttled one.

- Verify the **synchronous** part first — `return` a value directly and confirm
  it comes back — before debugging the async chain.
- Focus the Unity window and re-fire.
- The original fire-and-forget may still complete late, after the window
  regains focus. Before re-running, check for side effects it already made
  (files written, assets created), or you will do the work twice.
