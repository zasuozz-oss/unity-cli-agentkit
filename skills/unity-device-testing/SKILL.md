---
name: unity-device-testing
description: Use when a Unity change must be checked on a real phone/tablet instead of the Editor — "test trên device thật", "test trên máy thật", "cài lên điện thoại", "chạy thử trên Android/iOS", reading logcat or the iOS device console, device screenshots, crashes that only happen on device, or verifying analytics/Firebase events with DebugView. Covers build → install → launch → log → screenshot → report for Android (adb) and iOS (xcrun devicectl).
---

# Test a Unity build on a real device

Editor green is not device green: IL2CPP stripping, platform SDKs (ads,
IAP, Firebase, sign-in), permissions, safe areas, performance and native
crashes only show up on hardware. The job is: **build a debuggable player,
put it on a named device, drive it, capture evidence, report what was and
was not seen.**

Related: `utk-test-runner` (Editor tests first), `unity-android-build`
(Gradle/export problems), `unity-analytics-tracking-plan-sync` (what the
events should be), `utk-console-triage` (reading Unity errors).

## Rules

- **Never block.** `logcat`, `devicectl --console`, and builds run in the
  background or with a timeout, writing to a file you read afterwards.
- **Always name the device** (`adb -s <SERIAL>`, `--device <UDID>`). Two
  connected devices + no serial = the command hits the wrong one or fails.
- **Human steps are the user's.** Unlocking the phone, "Allow USB
  debugging", "Trust this computer", signing in to a store/Google/Apple
  account, real purchases, iOS signing/provisioning — ask and wait.
- **Destructive ≠ routine.** `adb uninstall`, `pm clear`, wiping app data
  deletes the tester's progress/save. Confirm first; say why (e.g. signature
  mismatch on install).
- **Don't test against production by accident.** Check which config the
  build uses (ad test IDs, Firebase project, backend URL) before installing.
  Test purchases/ads only with sandbox/test accounts and test ad units.
- **Report what the device showed**, not what the code implies. Not
  observed = say "not observed", never "works".

## Phase checklist

1. **Preflight** — device connected and authorized; package/bundle id;
   build config (dev/test vs prod).
2. **Build** — development build of the current code.
3. **Install + launch** on the named device, log capture already running.
4. **Drive** the scenario (agent taps for simple flows, user for human steps).
5. **Capture** logs, screenshots, crash traces; for analytics, the event log.
6. **Report** — table below.

## 1. Preflight

```bash
# Android
adb devices -l                         # "device" = ok; "unauthorized" = user must tap Allow
adb -s <SERIAL> shell getprop ro.product.model
adb -s <SERIAL> shell pm list packages | grep <PACKAGE_ID>   # already installed?

# iOS (Xcode 15+)
xcrun devicectl list devices           # needs "available (paired)"; else user trusts the Mac
```

Find `<PACKAGE_ID>` / `<BUNDLE_ID>` in `ProjectSettings/ProjectSettings.asset`
(`applicationIdentifier`) — don't guess.

## 2. Build

```bash
utk build --confirm true --target Android --outputPath Builds/Android/<NAME>.apk
utk build --confirm true --target iOS     --outputPath Builds/iOS
```

Run in the background (up to 30 min). Enable **Development Build** (and
Script Debugging only if you need it) so `Debug.Log` and full stack traces
reach the device log. Check it in Build Settings before building; if the
project builds through its own menu/script, use that instead.

iOS output is an Xcode project. Build + sign it:

```bash
xcodebuild -project Builds/iOS/Unity-iPhone.xcodeproj -scheme Unity-iPhone \
  -configuration Debug -destination 'id=<UDID>' -allowProvisioningUpdates \
  -derivedDataPath Builds/iOS/dd build
```

Signing error (no team, no profile) → stop, hand to the user: signing needs
their Apple account in Xcode.

## 3. Install + launch (start logs first)

```bash
# Android — clear, then capture in the background to a file
adb -s <SERIAL> logcat -c
adb -s <SERIAL> logcat -v time Unity:V AndroidRuntime:E DEBUG:V FA:V FA-SVC:V *:S \
  > Builds/logs/device.log &          # or run_in_background; stop it when done
adb -s <SERIAL> install -r Builds/Android/<NAME>.apk
adb -s <SERIAL> shell monkey -p <PACKAGE_ID> -c android.intent.category.LAUNCHER 1

# iOS — install, then launch with the console attached (background + timeout)
xcrun devicectl device install app --device <UDID> Builds/iOS/dd/Build/Products/Debug-iphoneos/<APP>.app
xcrun devicectl device process launch --device <UDID> --console <BUNDLE_ID> \
  > Builds/logs/device.log 2>&1
```

`INSTALL_FAILED_UPDATE_INCOMPATIBLE` = signature differs from the installed
build → uninstalling wipes data: ask first.
`.aab` can't be installed directly — build an `.apk` for device testing.

## 4. Drive the scenario

- Agent-driven taps (Android): `adb -s <SERIAL> shell input tap <x> <y>`,
  `input swipe …`, `input keyevent KEYCODE_BACK`. Coordinates are device
  pixels — take a screenshot first and read them off it
  (`adb shell wm size`). iOS has no stock tap CLI → the user drives.
- For human steps, give the user a numbered script ("1. open shop, 2. watch
  a rewarded ad, 3. close"), ask them to say when each step is done, and
  note the time so you can slice the log.
- Background/foreground, kill/relaunch, airplane mode, rotation, low
  memory — only if the change touches them; list which ones you ran.
- **Xiaomi/MIUI/HyperOS blocks `adb shell input`** (`SecurityException:
  INJECT_EVENTS`) unless the user turns on *USB debugging (Security
  settings)*. Ask once; otherwise the user drives.
- **Is the installed build the one with your change?** Before reading any
  log, compare `adb shell dumpsys package <pkg> | grep lastUpdateTime` with
  your last source edit, and look for a log line only the new code prints.
  An old install looks exactly like "the fix doesn't work".

## Performance on device

Editor numbers mislead for hitches and FPS — measure on the device:

- Development build with *Autoconnect Profiler* (or connect the Profiler
  window to the device over USB/adb), record **1000+ frames** across the slow
  flow, then name the top markers by ms. Don't guess from code.
- Cold-start timing: `adb shell am force-stop <pkg>`, `logcat -c`, launch,
  then timestamp each boot phase from the log; repeat ≥ 3 runs.
- Temporary timing logs you add for this come out again before the report.

## 5. Capture

```bash
adb -s <SERIAL> exec-out screencap -p > Builds/logs/shot_<step>.png   # then Read the png
adb -s <SERIAL> logcat -d -b crash > Builds/logs/crash.log            # Java + native crashes
grep -nE "Exception|Error|FATAL|SIGSEGV|SIGABRT" Builds/logs/device.log
```

iOS: screenshots — ask the user (or `idevicescreenshot` if libimobiledevice
is installed); crash reports — Xcode → Devices and Simulators → View Device
Logs, or `idevicecrashreport`. IL2CPP native stacks need the build's symbols
to be readable — keep the build folder until the report is done.

## Analytics on device (Firebase)

Without debug mode Firebase batches uploads (~1 h), so DebugView stays empty
and you conclude "not firing" wrongly.

```bash
# Android: debug mode + verbose FA logs
adb -s <SERIAL> shell setprop debug.firebase.analytics.app <PACKAGE_ID>
adb -s <SERIAL> shell setprop log.tag.FA VERBOSE
adb -s <SERIAL> shell setprop log.tag.FA-SVC VERBOSE
# relaunch the app, drive, then:
grep -n "Logging event" Builds/logs/device.log
# done — turn it off again
adb -s <SERIAL> shell setprop debug.firebase.analytics.app .none.
```

iOS: pass the flag as a launch argument —
`xcrun devicectl device process launch --device <UDID> --console <BUNDLE_ID> -FIRDebugEnabled`
(or add it to the Xcode scheme's *Arguments Passed On Launch*; it persists
until a launch with `-FIRDebugDisabled`), then read the console log /
DebugView.

- The log line holds event name and params (`name=…, params=Bundle[{…}]`).
  Compare each against the tracking plan: fired at the right step, every
  param, right type (a Number shows as long/double, not a quoted string).
- **Count per action.** Two `Logging event` lines for one tap = double-fire.
- Firebase console → DebugView shows the same stream; it needs the user's
  signed-in browser (the agent never signs in). If you can't see DebugView,
  say the verification is logcat-only.
- Mark an event verified on device only when you saw its log line from this
  build. Everything else stays "Editor-tested only".

## Report

```
Device: <model> / <OS version> / <SERIAL or UDID>   Build: <dev|prod config>, <commit>
| Step | Expected | Observed (log line / screenshot) | Result |
|------|----------|-----------------------------------|--------|
Events (if analytics): | Event | Params seen | Count per action | OK? |
Crashes/errors: <none | excerpt + file>
Not tested on device: <iOS, purchases, ad fill, offline…> and why
Evidence: Builds/logs/device.log, Builds/logs/shot_*.png
```
