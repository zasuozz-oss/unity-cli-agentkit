---
name: unity-server-driven-feature-testing
description: "Use when building or testing a server-driven, time-based progression feature in a Unity game (daily streak, login rewards, weekly challenges, battle pass, event countdowns, milestone badges): planning it from a design doc and the real API spec, designing the in-game debug tool that advances days or resets state, writing test cases, testing on a fresh vs. shared QA account, or diagnosing a reward/badge/milestone that shows as reached too early, is not granted, or appears in the wrong order."
---

# Server-Driven, Time-Based Features — Plan, Debug Tool, Test

With these features the **server owns the clock and the rules**. The client
renders what the server says and asks it to act. Every class of bug below
comes from the client or the test setup quietly assuming something the server
never promised.

## 1. Plan from the real API, not from the design doc

- Read the design doc, then **check the real API spec** (OpenAPI/Swagger, or
  the backend's docs) before writing the client plan. Fields a design doc
  assumes on a GET may exist only on a submit response, or not at all.
- Grep the codebase for existing names before assuming a field is the right
  one. Similarly named, unrelated features are a common false match.
- Reconcile the written spec against the original design doc, line by line,
  and flag every divergence. Contradictions such as "reset immediately on a
  miss" in one section and "hold and ask the player" in another must be settled
  **before** implementation.
- Lock the open decisions with the user (what resets, what is permanent, what
  shows when) and write them down.
- Plan the debug tool and a mock data source in the first phase, so UI work
  does not wait for the backend.

## 2. The debug tool

- **Never simulate time with the device clock.** If the server computes the day
  boundary, the only valid time travel is a **server-side test action**. A
  device-clock change exercises rules production never runs.
- **Separate verbs for separate states.** At minimum:

  | Verb | Reaches |
  |---|---|
  | advance day, **no** submit/claim | "new day, today still pending": at-risk banners, reminders |
  | submit/claim today | the normal progress path |
  | advance twice with no submit | a real missed day, decided by the server's rule |
  | set progress to N | milestones far away |
  | clear | a restart of the feature's progress |

  A single "advance" that also submits makes the pending and at-risk states
  untestable.
- **Every verb states what it resets and what it leaves alone.** Transient
  progress (counters, claim flags) is not permanent ownership (inventory,
  badges, unlocks), which is usually additive-only by design. Put this in the
  tool's label or docs.
- **Do not reuse a "reset everything" helper** without checking what it wipes.
  One that also clears one-time "first reveal" flags makes a simulated new day
  look like a brand-new player.
- **After an action, reload through the production path** (the real scene or
  controller load), not a bespoke local patch. Otherwise the celebration, reward
  and badge UI the tool was meant to exercise never runs.

## 3. Accounts are state

- **A shared Editor/QA account is not a clean baseline.** Earlier passes that
  used "set progress" and claimed rewards left real, permanent server state
  that "clear" cannot remove. An "impossible" owned item may just be history.
- Before calling it a client bug, read the **raw ownership/state endpoint**.
  If the server says it is owned, the display is right.
- Use a fresh account for the real progression pass, and verify the fresh
  baseline by reading live state (level, flags, owned count all at their
  initial values) before running cases. Back up and restore local prefs around
  it (`@unity-ui-spam-click` pre-flight).

## 4. Bug classes to design out

- **"Owned" and "reached" are different booleans.** A milestone reward owned
  from an earlier run, or from a debug fast-forward, must not render as reached
  in the current run. Compute the state explicitly from both, e.g. Locked /
  Owned-not-reached / Reached (`owned && current >= threshold`). Pick the
  boundary (`>=` vs `>`) on purpose, and define what a zero or unset threshold
  means.
- **One convergence point per grant.** A reward can be granted by submit, claim,
  bundle, mission or admin. Update the ownership cache in the one place all
  grant paths share. A grant returned inline in a submit response that never
  calls `MarkOwned` stays grey until the next boot. `@unity-iap-purchase-ownership`
  §3 covers the same rule.
- **Never trust array order from a list API.** "Server-configured, don't
  hardcode values" does not mean the order is stable. Sort explicitly by the
  field that defines order, and put unset values in a defined place.
- **Render full catalogs from the catalog endpoint.** A view that renders only
  the current window of progress data silently omits everything outside it.

## 5. Test cases

- Generate the matrix with `@unity-qa-generator`'s template, built from
  **state combinations**, not from screens: day relative to each milestone ×
  submitted / not submitted × fresh / continuing / broken streak ×
  owned-but-not-reached.
- Map each case to an automated test where logic allows, and mark the rest
  manual.
- **The scripted pass is not the end.** Cross-session and device-only bugs
  escape it reliably: a session flag that does not reset on reload, or a popup
  that never signals close and freezes the queue. After the scripted pass, do
  one manual Editor pass on a fresh account that **repeats actions across a
  scene reload**. Then check the risky flows on a real device.
- When a QA ticket arrives later, reproduce it with the debug verbs first
  (`@unity-bug-regression-workflow`). Most tickets here are a state
  combination the matrix missed. Add it.

## Related Skills
- `@unity-qa-generator` — test-case format and IDs.
- `@unity-bug-regression-workflow` — reproduce before fixing, regression pairs.
- `@unity-popup-queue` — notices and celebrations these features trigger.
- `@unity-iap-purchase-ownership` — the same one-source ownership rules for purchases.
- `@utk-playmode-driving` — driving the Editor while testing.
