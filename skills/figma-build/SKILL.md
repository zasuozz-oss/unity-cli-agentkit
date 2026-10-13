---
name: figma-build
description: Use when the user asks to build a Unity screen/popup/prefab from a Figma URL or the node selected in Figma (optionally with a doc/behavior description) and the project has the FigmaImporter package — imports through the "Import Selected Node" flow (`FigmaQuickImport.ImportForAgent`, via utk exec) into a temporary scene, renames/wires in place, saves it with `FigmaQuickImport.SaveAsPrefab`, verifies node by node, then deletes the scene.
---

# figma-build — Figma node → production-ready Unity prefab

The end-to-end recipe for the importer route. Mapping rules, the manual route (no
importer) and the aspect/distortion audits live in `unity-figma-cli`; the order of work
(reuse the game's popup family, 9-slice, text fit) in `figma-unity-workflow`. Load both.

The user gives a screen name + Figma URL, or "the node selected in Figma", optionally with
a doc. Result: a prefab with a clean Unity hierarchy and scripts attached and wired. The
importer writes the import record, node folder and prefab reuse; the temp scene is deleted
at the end.

## Why not FigmaHeadlessImporter or the menu

`FigmaHeadlessImporter.Import` writes no scene-map, import record, node folder or prefab
reuse, and ignores the project's sprite/font folders and feature groups. Building into a temp
prefab and flattening/copying the wrapper has the same problem. The menu `GameObject/UI/Figma
Importer/Import Selected Node` records everything but opens modal dialogs and hangs `utk exec`.
`ImportForAgent` is the same flow, synchronous and dialog-free.

## Prerequisites — check first, stop and tell the user if one fails

1. `figma-cli status` OK (Figma Desktop over CDP; no MCP server, no plugin). `no_cdp` /
   `ambiguous_tab` / `boot_failed` need a human — report and hand back. Confirm the tab it
   names is the URL's file (or pass `--file <key>`).
2. `utk status` OK, Editor not in Play mode.
3. The API exists:
   `utk exec 'return typeof(FigmaImporter.Live.FigmaQuickImport).GetMethod("ImportForAgent") != null;'`
   False → the importer DLL predates the API; ask for an updated package. Never fall back to
   `FigmaHeadlessImporter`.
4. Importer settings (Figma Importer ▸ Dashboard): Sprite Folder, Font Folder and export scale
   set to the project's convention — read them, don't change them.
5. TMP Essentials imported (else `success:false` says so). One-time:
   `AssetDatabase.ImportPackage("Packages/com.unity.ugui/Package Resources/TMP Essential Resources.unitypackage", false)`
   (older TMP-as-package projects: `Packages/com.unity.textmeshpro/...`).

## API (`FigmaImporter.Live.FigmaQuickImport`)

- `ImportForAgent(GameObject parent = null, string nodeId = null, bool rebuild = false,
  string group = null, bool shareCommon = false)` → JSON **string**
  `{success, mode: built|rebuilt|synced, nodeId, rootPath, nodeFolder, missingFonts[],
  prefabs[], nodes[], sync, error}`. No log field — the per-node build log is in `utk console`.
  - `nodeId: null` = the node selected in Figma.
  - `group` = feature folder under the Sprite Folder; only used when the node has no record yet.
  - `nodes[]` (every mode) = `{figmaId, figmaName, path, active, sprite, sliced}`; `path: null`
    = the object was deleted, `sliced` = `sprite.border != 0`.
  - Check the `shareCommon` parameter exists on your DLL (`GetMethod` + `GetParameters`) before
    passing it.
- `SaveAsPrefab(GameObject root, string assetPath)` → `{success, prefabPath, guid,
  relinked[], error}`. Re-points the scene-map and `RootGlobalId` after the convert; saving over
  an existing prefab path keeps its GUID.

## Workflow

1. **Node id** — from the URL (`?node-id=1234-5678` → `1234:5678`; figma-cli never takes
   hyphens), or `null` for the Figma selection. Pass it explicitly when you know it.
2. **Temp scene** — create and save an empty scene, e.g. `Assets/_FigmaImportTemp.unity`
   (never committed, never in Build Settings). The root must live in a **saved** scene: records
   use `GlobalObjectId`, Prefab Mode is not supported.
   Add the shell the import goes into by **mirroring an existing popup prefab of the game**
   (same Canvas/CanvasScaler reference resolution and match, raycaster, camera binder, root
   script, dim/mask child, panel with its fit/animation components). Save the scene.
3. **Import** into the shell's panel:
   ```sh
   utk exec 'var p = GameObject.Find("<Name>/Panel");
     return FigmaImporter.Live.FigmaQuickImport.ImportForAgent(p, "<id or null>", false, "<FeatureGroup>");'
   ```
   - `group` = the **game feature** (Shop, StarterPack, Profile…), never a per-screen name.
     Sprite dedup is by content: a sprite already in `<Sprite Folder>/Shared_Common/Sprites` or
     `<group>/Shared_<group>/Sprites` is reused in place; a duplicate between two nodes of the
     same group is promoted to `Shared_<group>`. A duplicate of ANOTHER group's sprite stays a
     local copy unless `shareCommon: true` moves it to `Shared_Common` (GUID kept); the console
     lists those sprites either way — read it when the game already has a shared folder.
   - A node folder that already exists without an import record (an old headless build):
     move it aside first (`AssetDatabase.MoveAsset`), or the importer forks `<Node_Name>_2`.
   - Read `missingFonts` every time — those labels fell back to the TMP default and lost their
     effects; the importer already tried installed and Google fonts, the rest needs the font
     file from the user. Keep `nodes[]` for step 8.
4. **Clean up in place** (via `utk exec`). The sync matches by object identity, not name:
   - **Rename freely** to English PascalCase with role prefixes (`Btn`, `Img`, `Txt`, `Icon`,
     `Pnl`, `Bg`); translate non-English names by meaning.
   - **Add components/children freely** (script views, LayoutGroup for runtime lists).
   - **Never flatten or reparent** imported nodes — leave Figma wrappers in place.
   - **Hide (`SetActive(false)`) design-sample content, don't delete it** — duplicate list
     cells and mock values only, not design elements (see "Default state" below). A deleted node
     is reported "Unresolved" on every sync. Delete only what must not ship (a full-screen mock
     background) and accept the Unresolved line.
   - **Never copy the imported tree** to build the final prefab elsewhere — the copy has no
     record. Exception: a runtime list-cell prefab is made from a copy of the cleaned-up sample
     cell; it is then a plain prefab.
5. **Generate scripts** (only with a doc/description) into the nearest `Scripts/` folder;
   serialized field names follow the cleaned-up node names.
6. **Attach & wire** on the shell root: `AddComponent`, assign each `[SerializeField]` via
   `transform.Find("<path>")`. Cleanup (step 4) comes first — wiring depends on final names.
7. **Save as prefab** through the importer, never raw `SaveAsPrefabAssetAndConnect` (the convert
   changes every `GlobalObjectId`; only this call re-points the ledger):
   ```sh
   utk exec 'var r = FigmaImporter.Live.FigmaQuickImport.SaveAsPrefab(GameObject.Find("<Name>"), "<prefab path>");
     UnityEditor.SceneManagement.EditorSceneManager.SaveScene(GameObject.Find("<Name>").scene); return r;'
   ```
   Saving over an existing prefab path keeps its GUID — compare `guid` with the old one. Later
   edits on the instance: `PrefabUtility.ApplyPrefabInstance(root, InteractionMode.AutomatedAction)`.
   Sprites copied in from an older prefab still point at the old sprite folder — grep their
   GUIDs before deleting that folder.
8. **Verify before deleting the scene** (fixes can still be applied): clean compile, no new
   errors in `utk console`, EditMode tests, then a **Play-mode screenshot from the game's real
   entry flow** next to the Figma render (`figma-cli get_screenshot --node <id> --max 0`).
   - Node checklist: re-run step 3 (`mode: synced`) and walk `nodes[]` — every node has a
     non-null `path` and `active: true` unless the user decided to hide/delete it — then confirm
     shape and position on the screenshot.
   - Aspect sweep 9:16 / 9:18 / 9:23 when art reaches past the popup frame (`unity-popup-layout`).
   - "Done" only when no line is off; a line off because server data is missing is named in the
     report, never hidden.
9. **Delete the temp scene** — open the project's normal scene, then
   `AssetDatabase.DeleteAsset("Assets/_FigmaImportTemp.unity")`. Commit only the prefab, the
   sprite folder and scripts.

## After a Figma change

The temp scene is gone, so in-place diff sync is gone too: `ImportForAgent` on the same node
builds a new tree (`mode: built`). Small change (a sprite, a text, a position) → edit the prefab
directly. Large change → repeat the workflow in a new temp scene, re-wire, and `SaveAsPrefab`
over the same path (GUID kept). `rebuild: true` throws the tree away — renames and wiring with it.

## Rules

- **Default state = the design.** Every design element (model art, discount badge, compare
  price, secondary buttons) is visible by default with the design's content. Server data only
  overrides text/sprites; never hide an element because the server has no data yet. Hide only on
  an explicit user decision, recorded in the report.
- **Visuals follow the design, behaviour follows the project.** Popup show/hide animation, dim
  alpha, fit components and purchase flow use the project's existing helpers with their defaults,
  exactly like the game's other popups — never a custom mask alpha or tween to match Figma.
  Seen: a 0.9 mask alpha copied from Figma made one popup's dim differ from every other popup.
- **Reused imported sprites**: the importer writes small 9-slice crops (`sliced: true`, e.g.
  72×72). Any `Image` you point at one needs `Image.type = Sliced`, or it stretches into a blob.
- **Runtime list cells come from the imported sample node**, never from an older hand-built
  prefab with repointed sprites. Seen: a reward tile turned into a circle that way.
- **Server-fed image slots** (model art, thumbs): `Image`, not a stretched `RawImage`; size and
  anchoring per `unity-remote-image-flicker` §4.
- **Buttons**: nodes tagged "Button" (name contains button/btn) get `UnityEngine.UI.Button`
  (transition None) on build/rebuild only, not on sync. Add the project's button feedback
  components like its other popups and wire `onClick` yourself.
- **`utk exec` prefab snippets**: build the return string BEFORE `UnloadPrefabContents` —
  reading a RectTransform after unload throws `MissingReferenceException` and looks like a
  failed save. Pass snippets through a quoted heredoc (`<<'EOF'`), never `echo` with `\n`.
- `sync.Cleaned` may list other nodes' records whose sprite folder and root are both gone —
  valid housekeeping; mention it, don't undo it.
- The sync library (`.unity-figma/`) and `Library/FigmaImporter/import-records.json` are local
  state, never committed; never edit files inside `.unity-figma/<id>/`.
- Figma is read-only here: never call figma-cli write tools while building into Unity.
- Every `utk` command gets a finite timeout; after 3 failures stop and report with the output.

## Related Skills

- `unity-figma-cli` — mapping, manual route, distortion audit, verification
- `figma-unity-workflow` — order of work, reuse the game's popup family, 9-slice, text fit
- `unity-popup-layout` — aspect check and bleed art inside a popup
- `unity-remote-image-flicker` — server images replacing design placeholders
- `utk-exec-query`, `utk-playmode-driving` — the `utk exec` and Play-mode mechanics
