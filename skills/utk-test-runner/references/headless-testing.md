# Headless Testing & Compile Checks (Unity 6)

**Verified against official docs: 2026-08-21.** Wrapper: `scripts/unity-test.sh`
(offline / compile / editmode / playmode). This page corrects an older assumption
that Unity tests "cannot be run headlessly" — they can, via the Unity Test
Framework CLI.

## Sources

- Test Framework CLI arguments:
  <https://docs.unity3d.com/Packages/com.unity.test-framework@2.0/manual/reference-command-line.html>
- Editor command-line arguments (Unity 6):
  <https://docs.unity3d.com/6000.2/Documentation/Manual/EditorCommandLineArguments.html>

## Offline first (no Unity at all)
Order of checks: (a) `unity-test.sh offline <repo>` first, always; (b) batchmode/Editor only when
offline cannot cover it. `offline` = `dotnet build` on Unity's generated solution (~3 s);
`offline <repo> --tests` additionally runs the EditMode test assemblies (built to `Temp/bin/Debug`)
with NUnitLite 3.14 on .NET, resolving Unity DLLs from the Editor install's `Managed` folder and
`Library/ScriptAssemblies`. Measured on a 29-project game: 1085 tests ran in ~8 s total, 112 passed,
971 reported SKIPPED-NEEDS-EDITOR (native UnityEngine/UnityEditor ECall `SecurityException`, "No log
scope" from `LogAssert`, NUnit API mismatch with Unity's custom NUnit), 2 FAIL (a JsonUtility-based
fixture decode and an asmdef type lookup, both Unity-runtime dependent: confirm in the Editor).
Limits: logic-only tests run; anything touching `AssetDatabase`, `GameObject`, scenes, Texture, JsonUtility
and similar needs the Editor. Tests run with the repo as cwd, so a test that writes tracked files will
write them. The runner is built once into `~/.unity-cli-agentkit/offline-runner` (needs NuGet access once).

## Running tests headlessly

```bash
Unity -batchmode -nographics -projectPath <path> \
      -runTests -testPlatform EditMode -testResults results.xml -logFile -
```

- `-testPlatform` accepts `EditMode`, `PlayMode`, or any `BuildTarget` enum
  value (runs tests in a built player for that platform).
- Results are written as NUnit-format XML (`-testResults <file>`).
- Filtering: `-testFilter` (semicolon-separated names or regex),
  `-testCategory`, `-assemblyNames`.
- `-nographics`: skips graphics-device init so runs work on GPU-less machines.
  Some PlayMode tests that genuinely render may need it removed.

## Creating a project from the CLI

```bash
Unity -createProject <pathname> -quit -batchmode -logFile -
```

`-createProject <pathname>` — "Create an empty project at the given path"
(verified 2026-08-21, Unity 6000.2 EditorCommandLineArguments). **Empty
means no template**: no URP/2D template packages — the docs list no
template-selection argument.

`-buildTarget <name>` — "Select an active build target to launch the Editor
in"; documented values include `android`, `ios`, `webgl`, `win64`,
`osxuniversal` (same page, verified 2026-08-21). Lowercase names; the build
support module must be installed. The bundled `unity-editor-open.sh`
(utk-cli-core) passes `$BUILD_TARGET`, default `android`. Add packages afterwards by editing
`Packages/manifest.json` (plain JSON). For UI-only games the default modules
(uGUI, UI Toolkit) are enough.

## Compile check (no tests)

```bash
Unity -batchmode -nographics -quit -projectPath <path> -accept-apiupdate -logFile -
```

Unity exits with code **1** on exceptions/compile failures. `-accept-apiupdate`
lets APIUpdater run in batch mode — omitting it "might lead to compiler errors"
(Unity docs).

## Gotchas (from the official docs)

- **Never combine `-quit` with `-runTests`** — "-quit causes the Editor to quit
  immediately, before in-progress tests have chance to complete."
- Exit codes for test runs are not uniformly defined ("no common definition for
  exit codes reported by individual Unity components under test") — parse the
  results XML instead of trusting the exit code. `scripts/unity-test.sh` does this.
- Batch mode runs cannot share a project with an open Editor instance — close
  the Editor (or use a separate clone) before running.
- Always pass `-logFile` (a file, or `-` for stdout) or failures are invisible.
