---
name: unity-bug-regression-workflow
description: "Use when starting any bug fix from a bug report (version/build, description, steps to reproduce), when a bug 'came back', regressed, or was fixed before, or before changing error routing, badge, or state-machine logic."
---

# Unity Bug Fix & Regression Workflow

## Overview
Workflow for fixing bugs in long-lived Unity game projects without reintroducing past bugs. The core rule: **recall prior fix history BEFORE reading code.** A fix made in ignorance of the previous fix usually reintroduces the previous bug.

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

## Best Practices
- ✅ **Always** run memory/lesson search with the subsystem keywords (e.g. "social login popup", "notification badge") before proposing a fix.
- ✅ When a prior fix exists, state explicitly what the OLD fix protected against, and verify the new fix preserves it.
- ✅ Assess blast radius (callers/dependents) before editing shared methods — use the project's code index (e.g. `codegraph impact`).
- ✅ After fixing, re-run the ORIGINAL bug's STR plus the previous regression's STR.
- ✅ Save the root cause as a lesson when the bug class is reusable (timing race, recycled state, error-route collapse).
- ❌ **NEVER** treat a popup/badge/state bug as "a simple one-liner" — these classes regress the most.
- ❌ **NEVER** close a task without recording why the previous fix failed, if it did.

## Few-Shot Examples

### Example 1: Regression-aware fix
**User**: "fix this bug, and check memory first because the last fix for it caused a different bug: the error popup does not show when login fails"

**Agent**:
1. Search memory: "login error popup" → finds prior fix that routed all auth errors to a network-error popup.
2. Identify the regression pair: prior fix fixed "popup missing" but caused "false no-internet popup".
3. New fix must distinguish auth failure from network failure — satisfy BOTH constraints.
4. Verify both STRs: (a) login fail shows correct error popup, (b) login fail with network up does NOT show "no internet".
5. Save lesson: "auth-vs-network error routing must stay separated".

## Related Skills
- `@unity-social-auth` - Known regression pairs in login/link flows.
- `@unity-event-safety` - Stuck-flag and ghost-handler bug classes.
