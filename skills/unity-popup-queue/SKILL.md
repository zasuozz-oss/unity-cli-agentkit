---
name: unity-popup-queue
description: "Use when working on auto/queued popups in a Unity game (startup or menu popup queues, rule-based 'show if' popups, notices after an event), one-time popups or overlays ('show only once'), or bugs like a popup reopening itself after closing, a popup shown every time instead of once, a notice re-announcing an old event, an OK button closing more than its own popup, a currency/top bar drawn over a popup, a close button that never lets the game continue, the popup queue freezing, or a countdown that leaves the screen stuck after the app was idle or backgrounded."
---

# Popup Queues, One-Time Popups and Layering

A popup queue is a small scheduler: rules decide *whether* to show, the queue
decides *when*, and a turn ends when the popup closes. Most bugs come from one
of those three contracts leaking: a turn that ends too early, a rule that
re-arms itself, or a popup whose close touches more than itself.

Back-stack navigation (back button, drag-to-close, stacking the same panel) is
`@unity-panel-navigation`. This skill covers popups the game shows *on its own*.

## 1. The turn contract

- **A queued turn completes only when its popup has actually closed.** Await a
  completion signalled from the popup's close path, never from enqueue, from
  "show started", or fire-and-forget. A turn that completes early lets the next
  popup show on top of it. A turn that *never* completes (e.g. an `async void`
  show that forgets to notify close) freezes every later popup, and everything
  awaiting "queue idle", tutorials and onboarding included.
- Signal completion on **every** close path: OK, close button, backdrop tap,
  back button, forced close on scene change. Put it in the one close handler all
  of them call.
- **Re-check the condition when the turn runs**, not only when it is enqueued.
  State can change while the popup waits behind others.
- **Do not add a wait between two systems that already wait on each other.**
  A tutorial step that waits for "popups idle" while a popup waits for a
  tutorial flag is a deadlock (`@unity-ui-spam-click`).

## 2. A rule must not re-arm itself

- **A popup's own data refresh must not reset its own "shown" rule.** If
  showing the popup triggers a refetch, and the refetch resets the rule because
  the triggering state "is still there", the queue enqueues a second turn, and
  the popup reopens right after OK.
- Compare the **identity** of the triggering event (an id, a timestamp such as
  "broken at"), not its presence. Reset the rule only when a *different*
  event arrives.
- **Server "last event" fields can be sticky.** A field like "previous value /
  broken at" may stay populated long after the event. A rule that only checks
  "field present" plus a device-local "already shown" flag resurrects old news
  whenever the flag is lost (reinstall, new device, debug reset). Also check
  current state (e.g. "the value has since recovered").

## 3. Dispatch is not idempotent

- One screen entry can dispatch the queue several times (after login, after a
  scene load, after a submit). Any rule scoped "Always" then shows once per
  dispatch. Scope one-shot rules per session or persist them, and make
  enqueueing a rule that is already queued or showing a no-op.
- A session-scoped "fired" set must reset when a new session really starts
  (e.g. a scene reload that means a new session), and **only** then. Repeat an
  action across a reload once in manual testing: this class of bug escapes
  scripted tests.

## 4. One-time popups and overlays

- **Persist the "seen" flag when the popup shows**, not when its animation
  completes. A kill mid-animation (scene change, another popup) otherwise
  replays it.
- **Use a new key when the meaning changes.** Reusing an old intro key for new
  content either hides the new popup from everyone who saw the old one, or
  shows the old one again.
- A local "seen" flag deduplicates display. It is not truth, so do not use it
  to decide whether the event happened.
- **"Show X after Y" means chain from Y's completion.** Raise the overlay from
  the first popup's show-complete callback, and call the **same** open method
  the manual button calls, so it animates exactly like a user tap. Only the
  manual path should pass "auto-show the info" flags. The queue-driven path
  must not also queue it.

## 5. A popup's buttons touch only that popup

- OK on a notice closes **the notice**. It does not close the popup underneath,
  restart another flow, or call a shared "confirm" callback that other popups
  also use, unless the spec says so. Users notice when one tap closes two
  things, and the second close usually re-triggers a rule.
- **A dismissible popup during boot must release the gate it holds.** A soft
  update or notice shown before the menu must, on close, clear the flag that
  blocks scene-advance and call the advance step directly. Re-running login or
  version checks from `Close()` shows the same popup again, forever
  (`@unity-startup-loading`).

## 6. Layering of shared bars

- A popup that raises a shared, always-on element (a currency bar, a top bar)
  above the popup layer for an effect (for example a reward flying into the
  bar) must **restore it on every exit**: in `OnDisable`/`finally` of the panel
  that raised it, not only on the success path. Otherwise the bar stays drawn
  over the next panel.
- Look at the sibling panels that already do the same raise. Match their
  restore instead of inventing a new one.

## 7. Timers that gate a UI state

- A countdown that flips the UI when it reaches zero must handle **"already
  expired at the first tick"**. If the app hangs or is backgrounded for longer
  than the tick interval, an edge-triggered "was counting → now expired"
  transition never fires, and the screen stays stuck in its waiting state.
- Compute remaining time from a stored end **timestamp** on every tick (and on
  resume), and treat `remaining <= 0` as expired regardless of what the last tick
  saw. Do not gate the expiry path on a flag that only a previous tick could set.

## 8. Picking which message or variant to show

- Choose the variant from computed truth (the actual counter, the full
  milestone list), not from a partial snapshot such as the visible window of a
  list. A snapshot silently drops the branches that fall outside it.

## 9. Verify

- For every popup rule you touched: first show, second dispatch in the same
  session, after a scene reload, after reinstall or with the local flag cleared,
  and with the popup killed mid-animation.
- For turn-contract changes: queue two popups and confirm the second waits for
  the first's close on every close path.
- A test that proves a guard should fail with the guard removed. Flip it off
  once to see it go red.

## Related Skills
- `@unity-panel-navigation` — back stack, drag/swipe close, re-open without stacking.
- `@unity-event-safety` — pairing raise/restore, show/hide on every path.
- `@unity-startup-loading` — boot sequence and popups that gate it.
- `@unity-ui-spam-click` — interleavings between popups and tutorials.
- `@unity-dotween-safety` — show/hide animations that get killed mid-way.
