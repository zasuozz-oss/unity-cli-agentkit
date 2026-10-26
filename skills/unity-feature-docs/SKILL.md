---
name: unity-feature-docs
description: "Use when writing or revising a document about a feature of an existing Unity game — a backend/API spec or proposal for the server team, an in-game flow diagram (which screen or popup opens what, in what order), a plan or overview that must match the current code, or when a product decision was just made and the related specs, plans, sheets and diagrams must be brought in line."
---

# Feature Docs for an Existing Game — evidence first

Readers act on these docs: the backend builds what the spec says, and the
designer approves the flow they see. Most rework came from one root cause:
writing from the design text or from memory instead of from the code and the
live server.

## 1. Collect evidence before writing a line

- **Client:** grep the real entry points and callers, and keep `file:line` for
  every claim ("the shop opens from HomePanel, TopMenu and the fail popup").
- **Server:** probe the live endpoints (`unity-live-api-probe`). The docs'
  schema is not proof.
- Split every statement into three buckets and keep them visibly separate:

  | Bucket | Wording |
  |---|---|
  | Current behaviour, verified | cite the source (`file:line`, endpoint + date) |
  | Unknown | "unknown — ask BE/PM", never phrased as fact |
  | New requirement | what changes, and who builds it |

- **Never state a server guarantee that you only saw the client enforce**
  (idempotency, "no double credit", validation).
- Do not propose changes to a flow the user says already works.

## 2. API proposals for the backend

1. Restate the user's constraint in one line before answering ("server decides,
   client only displays, no client update needed for new variants").
2. Build on the existing endpoint that already carries similar data. Do not
   invent a new endpoint first.
3. Give **one** response JSON with only additive fields, plus a table:
   `field → who creates the value (CMS / AB assignment / server) → how the
   client uses it`.
4. Use the names the team already uses in the sheet or code (`group_id`, not
   a new `variant_id`).
5. If you change your recommendation, say so and give the reason in one line.
   Do not silently flip between two designs.
6. Discuss in chat until the user says to write the file. When asked to resend
   the overview, resend the same document, not a new version.

## 3. In-game flow diagrams

1. Ask who reads it. A game designer wants screens, choices and rewards, not
   APIs or data flows.
2. **Enumerate the surfaces from code before drawing.** For each popup or
   pack: every place that can open it, the order it shows in, and what hides
   it. Grep the panels, top menu, gameplay popups, shop tabs and popup-queue
   rules. Show this list to the user once and get it approved.
3. Draw one diagram per entry area. A shared destination (the shop) gets its
   own flow, and the other flows end at it. "Close" counts as "decline".
4. Render and look at every version before sending. A recipe that works:
   Mermaid via `npx -y -p @mermaid-js/mermaid-cli mmdc`, hidden links
   (`A ~~~ B`) to steer the layout, one diagram per page, and Chrome headless
   print to PDF (A3 when A4 text is too small).

## 4. After every decision, propagate in one pass

- List every derived doc: spec, plan, overview, flow HTML/SVG, CMS spec, sheet.
  Update them together. The real API response breaks ties.
- Grep all docs for the retired terms (old field names, dropped features) and
  fix every hit. Delete docs that are now obsolete, so they cannot mislead the
  next session.
- Do not edit or commit a doc the user has uncommitted edits in. List the
  retired-term hits in it for the user, and commit only the other files.

## 5. Provenance

- **Never attribute a decision or a promise to a person** ("dev will add X",
  "named by the dev team") unless the user said so. A value found in a PM's
  sheet belongs to the PM. When a value has no source in code or docs, write
  "no source found".
- Label AI-derived items "(đề xuất)" or "(proposal)".
- Re-read the live sheet or doc before claiming a cell is empty or missing. A
  downloaded copy or an earlier dump is stale.

## Related Skills

- `unity-live-api-probe` — getting the real server response
- `unity-analytics-tracking-plan-sync` — tracking-plan sheets specifically
- `unity-server-driven-feature-testing` — planning server-driven features
