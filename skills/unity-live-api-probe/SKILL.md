---
name: unity-live-api-probe
description: "Use when a Unity game's real server response is needed and the API docs (Apidog/OpenAPI/Swagger) show no response schema or may be stale — the backend says it added fields, \"check whether the server fixed it\", a login/API error must be triaged, a UI element that depends on server data does not show, or a spec must cite current server behaviour. Covers calling the live endpoint with the session the Editor is already logged in with, without ever exposing the token."
---

# Live API Probe — ask the server, not the user

When the docs have no response schema, the live endpoint is the only truth.
Do not ask the user for a sample response and do not guess fields: call the
endpoint yourself with the account the Editor is already logged in with.

## 1. Find the base URL and the token source

Grep the game's own API client once:

```sh
grep -rn "Bearer\|Authorization" Assets --include=*.cs | grep -v Plugins | head
```

That shows where the base URL comes from (often a static field set at startup)
and where the token is read (often `PlayerPrefs.GetString("<key>")`).

- If the base URL or the token is only set at runtime, probe **in Play mode
  after login**, through the game's own request method. It also adds version
  headers and query params the server may require.
- Static config + a PlayerPrefs token → edit mode is fine for plain GETs; use
  `scripts/api_probe.cs`. It sends no version headers or extra params, so
  replay a failing or version-gated request in Play mode instead.
- Probing another account: check in the client caller whether the endpoint
  takes the id from the query or from the token.

## 2. Probe

Fill in `scripts/api_probe.cs` (BASE, PATH, token expression), then:

```sh
utk exec --file AgentScripts/api_probe.cs   # → "200 1532 bytes -> /tmp/api_probe/<name>.json token.len=180"
```

- The token stays inside the Editor process. **Print only its length**, never
  its value. Do not write it to a file, the repo, a report or memory.
- **GET only.** A POST can be irreversible (claim, buy, submit). Use it only
  when the user asked for that exact action.
- `rm -rf /tmp/api_probe` when done, then `ls` it before saying so.
- Never claim cleanup you did not verify.

## 3. Read the result like evidence

| Situation | Do |
|---|---|
| Backend "added fields" | Diff the JSON keys against the client's data class, add exactly the new keys, and list them for the user ("3 new fields, nothing else") |
| Test account returns `0`, empty or `null` | Zeros prove nothing. Find an account with data from a list endpoint (feed, leaderboard → `user_id`) and probe that id |
| UI element missing | Check **this account's server state** first: AB group, flags, owned items. The missing label may be correct for this group |
| User toggles a server setting mid-session (AB on/off, default group) | Re-fetch every time; bypass any cache param the client uses. Never answer from the previous dump |
| Login/API error | Order: `utk console --type error` → token length in PlayerPrefs → replay the same request → only then read client code. A failure on a replay with the client out of the loop is a server bug: report method, path, status and the body scrubbed of tokens/PII. Write the header as `Bearer <redacted len=N>` and give BE a repro with `$TOKEN`, or point them at their logs by time and user id. Never paste the token, even when asked |
| Writing a spec | Cite the probe as "observed <date>, <endpoint>". It outranks the docs. Mark unverified server behaviour "unknown — ask BE" |

## 4. Wire it into the client

- Add only the fields the response actually has. Keep JSON names identical to
  the server's.
- Reset the UI to its empty state when the shown entity changes, so a slow
  response never shows the previous entity's numbers.
- Verify on the account with real data. Use the same entry path the user
  takes, and take a screenshot.

## Related Skills

- `utk-exec-query` — the exec snippet contract (`using` is invalid, full type names)
- `utk-playmode-driving` — entering Play and waiting for login before a runtime probe
- `unity-server-driven-feature-testing` — planning features from the real API
- `unity-feature-docs` — turning probe results into a spec, an API proposal or a flow diagram
