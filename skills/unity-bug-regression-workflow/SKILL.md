---
name: unity-bug-regression-workflow
description: "Use when starting any bug fix from a bug report (version/build, description, steps to reproduce) — the bug must be reproduced in the Editor or by a failing test before any fix is written — when a bug 'came back', regressed, or was fixed before, or before changing error routing, badge, or state-machine logic."
---

# Unity Bug Fix & Regression Workflow

## Overview
Workflow for fixing bugs in long-lived Unity game projects without reintroducing past bugs. Two gates, in order: **recall prior fix history BEFORE reading code** — a fix made in ignorance of the previous fix usually reintroduces the previous bug — and **reproduce the bug BEFORE writing the fix** — a fix you never saw fail is a guess, and "it works now" proves nothing if it never broke in front of you.

## When to Use
- Receiving any bug report, whatever format the team uses.
- A bug matches a subsystem that was fixed before ("this happened before", "it came back").
- Changing shared logic that multiple bug fixes have already touched (error routing, notification badges, state-machine rules).

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Memory-first** | Search session memory / lessons for the subsystem name before opening any file. |
| **Bug note** | Before working, make sure the report has three things: the build it was seen on, what is wrong, and the steps to reproduce (STR). Keep the team's own format; ask for whichever piece is missing. |
| **Regression pair** | The previous fix and the new bug are often two sides of one constraint; the new fix must satisfy both. |
| **Root-cause capture** | Saving the reusable root cause after fixing is what breaks the fix → regress → re-fix loop. |

## Reproduce Before Fixing (mandatory gate)

No fix is written until the bug has been **seen failing on the current code**, by one of two routes:

| Route | Use when | How |
|---|---|---|
| **Logic repro** (preferred) | The fault lives in C# logic that can run without the scene: calculations, state machines, parsers, routing, save data | Write a test that feeds the report's inputs and asserts the *expected* behaviour (`@unity-editmode-tests`), then run it with `utk run_tests --mode editor --filter <TestName>`. It must **fail with the report's symptom** — a compile error or an unrelated assert is not a repro. For a one-off check, call the method directly with `utk exec` (`@utk-exec-query`). |
| **Editor repro** | The fault needs the scene, UI, timing, animation or input | Follow the STR in Play Mode through `@utk-playmode-driving`, then capture evidence: `utk console --type error` (`@utk-console-triage`), `utk exec` reading the wrong value from live state, and a `utk screenshot`. |

Before touching code, write down the repro: the exact commands or steps, the observed result, and the expected result. That record is the proof the bug existed.

After the fix, run **the same repro** again and show that it now passes. A logic-repro test stays in the suite as the regression guard. An Editor repro stays as the STR that the next fix must re-run.

**When it will not reproduce:**
- Do not fix blindly. Report what was tried (build, scene, data, steps) and what happened instead.
- Ask for what is missing: the exact build, device, account or save state, network conditions, a log or video.
- Device-only or SDK-only bugs (store, ads, login, hardware): simulate the external response in a test or a stub (mock the SDK callback or error code) so the logic path fails in the Editor. If even that is impossible, say so explicitly.
- Adding logging to catch the bug next time is allowed. A speculative fix is allowed only if the user explicitly approves it, and it must be reported as **unverified**.

## Best Practices
- ✅ **Always** run memory/lesson search with the subsystem keywords (e.g. "social login popup", "notification badge") before proposing a fix.
- ✅ When a prior fix exists, state explicitly what the OLD fix protected against, and verify the new fix preserves it.
- ✅ Assess blast radius (callers/dependents) before editing shared methods — use the project's code index (e.g. `codegraph impact`).
- ✅ After fixing, re-run the ORIGINAL bug's repro (it must now pass) plus the previous regression's STR.
- ✅ Save the root cause as a lesson when the bug class is reusable (timing race, recycled state, error-route collapse).
- ❌ **NEVER** write a fix before the bug has failed in front of you (failing test or Editor repro), and never call a fix done without re-running that repro.
- ❌ **NEVER** treat a popup/badge/state bug as "a simple one-liner" — these classes regress the most.
- ❌ **NEVER** close a task without recording why the previous fix failed, if it did.

## Few-Shot Examples

### Example 1: Regression-aware fix
**User**: "fix this bug, and check memory first because the last fix for it caused a different bug: the error popup does not show when login fails"

**Agent**:
1. Search memory: "login error popup" → finds prior fix that routed all auth errors to a network-error popup.
2. Identify the regression pair: prior fix fixed "popup missing" but caused "false no-internet popup".
3. Reproduce first: an EditMode test feeds a mocked auth-failure callback with the network up and asserts the auth error popup is requested. It fails, because the network-error popup is requested instead, which is the report's symptom.
4. New fix must distinguish auth failure from network failure — satisfy BOTH constraints.
5. Re-run the test (now passes) and verify both STRs: (a) login fail shows correct error popup, (b) login fail with network up does NOT show "no internet".
6. Save lesson: "auth-vs-network error routing must stay separated".

## Related Skills
- `@unity-social-auth` - Known regression pairs in login/link flows.
- `@unity-event-safety` - Stuck-flag and ghost-handler bug classes.
- `@unity-editmode-tests` - Writing the failing test for a logic repro.
- `@utk-playmode-driving` - Driving Play Mode for an Editor repro.
- `@utk-console-triage` - Reading the errors a repro produces.
- `@unity-ui-spam-click` - Reproducing and guarding fast-click / spam bugs.
