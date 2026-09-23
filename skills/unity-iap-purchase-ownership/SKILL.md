---
name: unity-iap-purchase-ownership
description: "Use when adding or debugging in-app purchases, receipt/server confirmation, or item ownership in a Unity game — a purchase charges the user but then shows an unknown error, the server rejects a confirm as already used, a signed request fails with 'signature expired' after the payment sheet, a bought item still shows as locked until the screen is re-entered, a buy button does nothing after an item's uses run out, a count shown in a header disagrees with the items rendered, or new server-side gating fields must also protect already-shipped client builds."
---

# IAP & Ownership — Three Systems That Must Agree

A purchase is one write that has to land consistently in three places: the
**store** (charged, transaction/token), the **game server** (confirmed,
entitlement granted), and the **client** (every list, badge, counter and button
that shows ownership). Almost every purchase bug is one of those three lagging
behind, or disagreeing with, another. "IAP is broken" is rarely the finding.

Purchase code has a large blast radius and is hard to test for real. Start
from `@unity-bug-regression-workflow` (recall prior fixes, reproduce first),
keep changes inside the scope the user approved, and say plainly what was
verified only by compile and unit tests.

## 1. Store → server: confirmation must be idempotent on *your* id

- **Several in-game products sharing one store SKU** is the classic trap. The
  store may answer a second purchase on the same SKU with the *previous*
  transaction/token (Google Play's "item already owned" is one form of it),
  while the IAP SDK still reports success. The client then sends a stale token,
  and the server rejects it as already used. The user sees "paid, then unknown
  error", typically on the *first* purchase of the second product.
- Recommended fix, on the server: key confirmation idempotency on the game's own
  **purchase-intent id** (created before the store call), not on the store's
  transaction id. Map a shared SKU back to the product through that intent, never
  by "first product with this SKU". A lookup by SKU alone attributes the
  purchase to the wrong product.
- Consume or acknowledge consumables on every path, success and failure, so the
  store never holds an unconsumed SKU that replays into the next purchase.
- **Wrap the SDK's purchase callback in try/catch.** These callbacks are often
  `async void` lambdas (see `@unity-async-patterns`). An exception escaping
  one kills the continuation silently: the loading spinner never hides, and the
  report says "stuck" or "unknown error". This guard alone fixes a permanent
  spinner.
- **Error codes must distinguish the stages.** Use separate codes for store
  failure, store success with server verification pending, server rejected
  (already used / invalid), and network failure. Log the store transaction id,
  the intent id and the server's response. Without these, "unknown error" cannot
  be triaged. Adding diagnostics before touching logic is a legitimate first
  step when the user asks for it.

## 2. Signed requests: never timestamp from engine time

- A signed request (purchase intent, confirm) whose timestamp is derived from
  `Time.realtimeSinceStartup`, or any launch-relative clock anchored once,
  drifts by however long the app was **in the background**. The store's payment
  sheet backgrounds the app, so the request right after payment is the one that
  fails with "signature expired".
- Recommended: anchor on a **server-time offset** applied to wall-clock time
  (`serverOffset + DateTime.UtcNow`). Re-sync it on resume **with retries**. A
  single failed resync on resume otherwise poisons the rest of the session.

## 3. Server → client: one ownership source, re-derived on every grant

- A grant can come from many flows: IAP, a pack or bundle, a reward, a level-up,
  a mission, a claim button. All of them must update **one live ownership
  source**, through one `MarkOwned`-style convergence point. A fix on the one
  grant path the tester used leaves every sibling path stale.
- Every consumer derives from that source *on the grant event*: grids, filters,
  notice bars, buy-all popups, summary counts. A list that split itself into
  owned/for-sale sections **once, when the screen opened**, stays wrong until the
  user re-enters, which is the signature of this bug. A per-cell refresh
  re-renders visible cells but does not re-partition sections
  (`@unity-scrollview-recycling`). Call an explicit rebuild.
- A count shown elsewhere (a login popup, a profile summary) must be computed
  from the same live source, not from a list fetched once at boot.
- After fixing the reported screen, grep for **every other reader** of the
  stale snapshot. Consider the ones not fixed, and say which you left and why.

## 4. Predicates: say exactly what blocks a purchase

- "Never buyable directly" (bundle-only, reward-only) and "currently blocked"
  (already owned, cooldown, out of uses) are different questions. A guard such as
  `if (isBundleOnly) return;` written for the first also blocks legitimate
  repurchase once a consumable's uses run out. The button shows a price and does
  nothing.
- Encode each as a named predicate (`BlockedFromDirectBuy => isBundleOnly &&
  !isOwned`), and route **every** purchase affordance through it: the buy
  button, the notice bar, buy-all, and popups. Copies of the old inline guard are
  where the bug survives.
- Know where each counter lives. If uses-left is server-authoritative,
  reinstall is safe. If it is only local, reinstall is a bug. Confirm it before
  answering "what happens on reinstall".

## 5. Declared metadata vs. what actually rendered

- Render a count from the **same resolved collection** the UI built, not from a
  separate server field (`item_count`) that can disagree with the ids that
  actually resolve.
- Log ids that fail to resolve, instead of silently `continue`-ing past them.
  A silent skip turns a data problem into a "wrong number" UI bug with no trail.

## 6. Old builds cannot read new fields

- A field the server starts sending is **invisible to already-shipped clients**
  if their serializer does not declare it. AOT or binary serializers (generated
  formatters, for example) drop unknown fields silently. Old builds therefore
  cannot enforce new gating such as purchase type, bundle-only or reward-only.
  Protect them **on the server** (filter what old versions receive), because
  shipped code cannot be patched.
- Send the build's real **version code** (an integer build number) to the
  server, never a display string like `Application.version` ("1.1.6"). A server
  parsing "1.1.6" as `1` treats every build as ancient.
- Adding the field to the client model also means regenerating whatever
  serializer code the project generates. Otherwise the new client drops it too.

## 7. Verify honestly

- Compile plus unit tests is not verification for purchase flows. Several
  regressions in this area only showed up in a live pass through the running
  game, for example a reward grant that did not rebuild the grid while the tab
  was open. Walk the real flow after the tests pass.
- Use the store's sandbox/test accounts for anything touching the store round
  trip. If you could not, say "untested against a real store", not "fixed".
- Use a fresh account per attempt: a purchase mutates server state, so a replay
  on the same account takes a different path (`@unity-ui-spam-click`
  pre-flight).
- Keep scope: Editor-only diagnostics stay Editor-only unless the user approved
  the device path too. Propose server-side changes; do not assume them.

## Related Skills
- `@unity-bug-regression-workflow` — recall prior fixes, reproduce before fixing.
- `@unity-async-patterns` — `async void` callbacks, single-flight requests.
- `@unity-scrollview-recycling` — refresh vs. rebuilding sections in recycled lists.
- `@unity-event-safety` — show/hide loading pairing on every purchase path.
- `@unity-telemetry-analytics` — logging purchase stages and error codes.
