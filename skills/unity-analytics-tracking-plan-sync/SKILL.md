---
name: unity-analytics-tracking-plan-sync
description: Use when syncing a Google Sheet tracking plan with the analytics code of a Unity game — auditing every event/param against the plan (fired or not, all params, Number vs String, trigger matches), fixing Firebase/analytics event code, hunting double-fire, tutorial funnel (start/step/end) or loading-span events, then marking the sheet verified and updating Notes. Triggers - "tracking plan", "analytics audit", "audit event", "đối chiếu sheet tracking", "check event firebase", "Google Sheet sync", "double-fire", "bắn event 2 lần", "tutorial funnel".
---

# Sync a Google Sheet tracking plan with Unity analytics code

The plan is the contract; the code is what ships. The job is one row at a
time: **does the code fire this event, at this trigger, with these params, in
these types?** Right → mark verified. Wrong → fix code, test, then update the
sheet — never the other way round, and never a blind write.

Every rule below exists because a real session got it wrong: assumed column
positions, read a CSV export that dropped blank rows (every row number off by
N), wrote into the wrong row, let a stale Apps Script run, pasted onto merged
cells, or wrote change-log prose into Notes until the user rewrote it by hand.

For dispatcher/SDK init/offline buffering, see `unity-telemetry-analytics`.
For tests, `utk-test-runner` and `unity-editmode-tests`.

## Placeholders — discover, never hard-code

Nothing in this skill names a game. Fill these from the project and the sheet
URL the user gives you; ask only for what you cannot find:

| Placeholder | How to find it |
|---|---|
| `<SHEET_ID>` / `<MAIN_GID>` / `<ENUM_GID>` | Sheet URL `/d/<SHEET_ID>/edit#gid=<GID>`; list tabs with the dump script |
| `<EVENT_LOGGER_CLASS>` | `grep -rn "LogEvent\|logEvent\|TrackEvent" Assets --include=*.cs` → the one wrapper everyone calls |
| `<EVENT_NAME_SOURCE>` | const class / enum holding event & param names (`grep -rn "\"level_start\"\|const string" …` near the logger) |
| `<ANALYTICS_TEST_FILTER>` | existing analytics test fixture/namespace, or the one you create |
| `<SCRIPT_DIR>` | a folder **outside `Assets/`**, e.g. `AgentScripts/gsheet/` — scripts kept for next time |

## Phase checklist

Create one todo per phase. Do not start a phase before the previous one's
output exists.

1. **Discover (code)** — find `<EVENT_LOGGER_CLASS>`, `<EVENT_NAME_SOURCE>`,
   every call site (`grep -rn "<EVENT_LOGGER_CLASS>\.\w*(" Assets`). Build a
   map `event → [file:line, params sent, C# type of each value]`.
2. **Dump the sheet** — run `scripts/dump_sheet.gs` (below). Output: header
   row → column map **by header text**, one record per event block with its
   real sheet row numbers, merged ranges, and the enum tab values.
3. **Audit each event** — for every event row (skip section rows): fired?
   trigger matches? every param present? type matches Data Type? value inside
   the enum table? double-fire? Record a verdict: `OK` / `FIX-CODE` /
   `FIX-SHEET` (plan is wrong about behaviour the product wants) / `ASK`.
   Label every finding **"sheet wants"** vs **"code does"**. A requested sheet
   change has not been made: warn only about the risk of *adopting* it, never
   describe it as if the code already did it (user: "đã làm đâu mà ảnh hưởng").
   Auditing many events with subagents → split by **event group**, one
   subagent per group.
4. **Fix code + test** — one fix at a time, patterns below. Compile, run
   tests (Verify). No sheet write until the code it describes is green.
5. **Update the sheet (guarded)** — `scripts/guarded_write.gs`: verify column
   ← `V` for `OK`, Notes per the Notes rules, plan corrections for `FIX-SHEET`.
   Every write is keyed on (row + expected cell values); mismatch = SKIP + log.
6. **Report** — table below.

## Reading the sheet

- **Map columns from the real header row**, never by position. Typical
  headers: `STT | Event | Trigger | Param | Definition | Values | Value
  Definition | Data Type | Param/Property Type | Android | Version | iOS |
  Status | Notes` plus a verify column — but names and order vary per game.
  Match case-insensitively; if a needed header is missing, stop and ask.
- **Read with Apps Script, not gviz/CSV export.** gviz drops blank rows, so
  its row numbers drift from the sheet's — every guarded write then misses.
- An event is a **block**: first row has Event/Trigger, following rows carry
  one param each with Event blank (often merged vertically, e.g. Version).
  Carry the event name down while parsing.
- **Section rows** (only a group name, no Trigger/Param) are not events and
  do not count toward STT.
- Enum values (placement, tutorial id, step…) live in a secondary tab, linked
  from the main tab by `#gid=<ENUM_GID>&range=<A1>` hyperlinks. The dump
  script logs those links so you can resolve them.

## Access: Apps Script in the user's browser

No API key, no service account, no OAuth grant made by the agent.

- **The user signs in to Google themselves.** Never type, read, or store a
  password; never click an OAuth consent/"Allow" screen — hand that to the
  user and wait.
- **Two browsers, never one.** Driving the user's visible window raises it to
  the front on every `open` / `tab` switch and steals their focus.
  - **View window** — headed, the user's own profile
    (`agent-browser --headed --session <PROFILE> --profile ~/.agent-browser/profiles/<PROFILE> open <sheet-url>`).
    Open it once, when the user asks to look. After that **do not touch it**:
    no `open`, no `tab`, no `eval`, no screenshot. Sheet edits show up there
    live (Sheets is collaborative), no reload needed.
  - **Work browser** — headless clone of that profile, does all Apps Script
    load/save/run:
    ```bash
    rsync -a --delete --exclude 'Singleton*' --exclude '*/Cache/*' \
      --exclude '*/Code Cache/*' --exclude '*/GPUCache/*' \
      ~/.agent-browser/profiles/<PROFILE>/ ~/.agent-browser/profiles/<PROFILE>-bg/
    agent-browser --session <PROFILE>-bg --profile ~/.agent-browser/profiles/<PROFILE>-bg open <apps-script-url>
    agent-browser --session <PROFILE>-bg eval 'typeof monaco'   # "object" = signed in
    ```
    The clone carries the Google login. If Google asks the clone to sign in
    again, stop and hand it to the user — never type credentials.
  - To screenshot for your own check, use the work browser (open the sheet
    in a second tab there), not the view window.
- Open the sheet's bound Apps Script (Extensions → Apps Script). First line of
  every script: `/** @OnlyCurrentDoc */` — limits the permission to this one file.
- Load code with `scripts/as_load.sh <file.gs>`: `monaco…setValue`, click
  **Save** by `aria-label` (Save shortcut keys leak stray text into the
  code), wait, then **assert editor content === file**. Keyboard/IME state can
  prepend stray text (`lại `, `t`) to the saved file; the script then fails at
  load (`ReferenceError … Code.gs:1`) and nothing runs. Mismatch → reload
  content and save again, never Run.
- **A failed Save means the OLD script runs.** `scripts/as_run.sh <fn>` checks
  the function dropdown shows `<fn>` before clicking Run, then polls the
  execution log ("Execution log" / "Nhật ký thực thi") for done/error.
- Finite timeouts; at most ~3 attempts per step, then stop and report.
- Save every script you run as `<SCRIPT_DIR>/partNN_<purpose>.gs`.

## Sheet-editing gotchas

- **Merged cells**: pasting/copying a range that touches a merge fails or
  scrambles it. Write cell by cell, skip `range.isPartOfMerge()` cells that
  are not the merge's top-left, or only write into unmerged empty areas.
- **Inserting/deleting rows in the enum tab** shifts every
  `#gid=…&range=…` link in the main tab → re-point those links afterwards
  (the dump lists them).
- **Try a formatting experiment on ONE event first** (borders, merges, fills),
  look at it, and revert it cleanly if wrong, before touching the rest.
- **Inserted rows** lose the event block's border and the vertical merge
  (e.g. Version) → redraw the block border and extend the merge.
- Keep the plan inside SDK limits. Firebase: event/param name ≤ 40 chars,
  string value ≤ 100 chars, ≤ 25 params per event. Never design a param that
  concatenates a growing list (all steps, all items) — it truncates.

## Notes column rules

Notes describe **caveats of the current behaviour, at product level**. They
are not a change log. The user has rewritten these by hand more than once.

- No prefixes or history: no `Fix code:`, `Fix sheet:`, `Lưu ý:`, `Note:`,
  "trước đó… nay…", "previously… now…", "đã gắn", "đã sửa", "event mới".
- Do not repeat what another column already says (Data Type, Values…).
- Empty is fine — no caveat, no note. Only caveats or open issues belong here, never an action log.

| Wrong | Right |
|---|---|
| Fix code: `ad_unit_id` read directly from the ads SDK (fallback from config when SDK returns empty) | `ad_unit_id` from the ads SDK (fallback from config when SDK returns empty) |
| Rewarded only; previously the placement check let interstitial send an empty `end_type` | Rewarded only, interstitial does not send `end_type` |

What you *changed* goes in the report, not the sheet.

## Code patterns

### Double-fire — check every event

Does one user action produce two events? Usual causes:

| Cause | Fix |
|---|---|
| Same trigger/flag raised from several UI paths (popup A and popup B both call it) | log at the single point every path converges on |
| Two entry points start one flow (list adapter + grid adapter); the second is rejected but still logs | log inside the flow runner (e.g. its `RunAsync`) after it actually starts, not at the call site |
| An "end" event guarded by a completion flag that is set late → fires again next time | guard with its own `_endLogged` flag set in the same statement that logs |

A trigger is defined by the **user action**, not SDK readiness. Seen: an
ad-request event fires on every button tap even when no ad is loaded; don't
gate it on the ad being ready.

Prove it: count calls per action in a test or a Play-mode log; don't argue
it from reading.

### Tutorial funnel

- `tutorial_start` — when the flow really runs (not when queued); not for
  sub-steps.
- `tutorial_step` — every step **passed**, with `tutorial_id`, `step`,
  `step_count` = consecutive repeats of the same step (reset to 1 on a new
  step). Skipped steps are not logged.
- `tutorial_end` — on done.
- Never replace these with one "sequence" event sent at the end: a user who
  quits midway never sends it, so drop-off becomes unmeasurable.

Names above are the shape; use whatever the plan calls them.

### Loading span

If `loading_finish` needs `load_time` and the caller doesn't have it, record
the start time in a `Dictionary<placement, float>` on `loading_start` and
consume (remove) it on `loading_finish` for the same placement.

### Types

Match the plan's Data Type exactly: Number → pass a numeric (`(long)x` /
`(double)x`), String → `x.ToString()`. An int sent as a string is a separate
column in BigQuery and breaks every dashboard query.

### Leave one test behind

Each non-trivial fix (dedupe guard, `step_count`, load-time dictionary) gets
one small EditMode test. If the test assembly can't reference
`Assembly-CSharp`, reach the type through reflection
(`Type.GetType("<Namespace>.<Class>, Assembly-CSharp")`). Stub the logger so
the test counts calls instead of hitting the SDK.

## Verify

```bash
utk editor refresh        # must compile clean — fix every error first
utk run_tests --mode editor --filter <ANALYTICS_TEST_FILTER>
```

Transient network/connection error from `utk`: wait ~5 s, retry, max 3. If
you cannot verify on a device / Firebase DebugView, say so in the report —
EditMode green is not "verified on device". On-device check: `unity-device-testing`
(Firebase debug mode + event log).

## Report

```
| Event | Param | Verdict | Change (code/sheet) | Test |
|-------|-------|---------|---------------------|------|
New/changed Notes: <row → text>
Not verified: <device/DebugView, backend-dependent params…>
Open questions: <backend doesn't return X yet, server must attach additionalData…>
Scripts: <SCRIPT_DIR>/partNN_*.gs
```

## Apps Script templates

`scripts/dump_sheet.gs` — logs tabs, header map, every non-empty cell as
`row|col|value`, merged ranges and cell hyperlinks. `scripts/guarded_write.gs`
— applies a list of writes, each keyed on expected cell values, skipping and
logging any mismatch or non-top-left merged cell. Edit only the `CONFIG`
block at the top of each.
