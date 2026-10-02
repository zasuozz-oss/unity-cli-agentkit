---
name: unity-telemetry-analytics
description: "Use when integrating analytics/telemetry events, Firebase Analytics or Crashlytics, an event dispatcher in front of one or more analytics SDKs, offline/pending event buffering, or events fired before the SDK finished initializing."
---

# Unity Telemetry and Analytics Integration

## Overview
Guidelines for managing analytics event dispatching and telemetry tracking. This ensures all metrics and tracking events are successfully buffered offline, dispatched without locking the main thread, and properly synchronized when Firebase initialization completes.

## When to Use
- Implementing custom game telemetry trackers.
- Adding new Firebase analytics events or parameters.
- Debugging a dispatcher/buffer layer that drops or duplicates events offline.
- Handling events that occur before Firebase has fully initialized.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Event dispatcher** | One high-level entry point that fans events out to every analytics service (Firebase, AppsFlyer, …). Names below are illustrative — match your project's. |
| **Pending event buffer** | Queue holding events fired offline or before initialization, flushed once the SDK is ready. |
| **Telemetry Event** | A structured data payload with name, timestamp, and metadata. |
| **Initialize Delay** | Handling the gap when events are fired before Firebase completes initialization. |

## Best Practices
- ✅ **Always** buffer events if the analytics SDK is not yet initialized — never drop them.
- ✅ Use asynchronous patterns (`UniTask` or Task-based) for network dispatching to prevent main thread blocking.
- ✅ Implement proper CancellationToken propagation to cancel telemetry uploads on scene change.
- ✅ Standardize event parameter formats (use snake_case for event parameters and names).
- ❌ **NEVER** drop events directly if a network request fails; retry with exponential backoff or buffer them.
- ❌ **NEVER** perform blocking synchronous HTTP operations on the main thread for telemetry.
- ✅ After merges/reverts, verify the WIRING, not just the classes: the dispatcher/buffer can silently become dead code while the legacy log entry point keeps its old drop-if-not-initialized logic. Check that the central log method actually routes through the dispatcher.
- ✅ Verify events on device with Firebase DebugView, not the normal dashboard (normal pipeline lags hours): `adb shell setprop debug.firebase.analytics.app <package.name>` to enable, `adb shell setprop debug.firebase.analytics.app .none.` to disable.
- ✅ Instrument funnel checkpoints around loading-phase API calls (e.g. between login-success and world-time fetch) — these gaps are where silent user drop-off hides.

## "Does event X fire?" — map the gates, not just the call site

Finding the `LogEvent("x")` line answers nothing. For each caller, list what
stands between the user action and that line:

- the remote-config flag that enables the feature, **and its default** (a
  default of 0 means "never fires" until config loads/sets it);
- cooldowns/timers and early `return`s before the log;
- in-flight guards — no `isLoading` check means several overlapping loads,
  each logging a request;
- a wrapper method that exists but nothing calls (e.g. an attribution SDK's
  purchase method next to Firebase's automatic `in_app_purchase`).

A sibling project with the same SDK is the fastest diff for the missing guard.
Prove it with DebugView on a device, not by reading.

**Naming a new param:** "is it a valid name" and "does it collide" are two
questions — answer the second from Firebase's *Automatically collected events*
page (fetch it; don't answer from memory): SDK-logged events such as
`ad_impression` carry their own params, and a same-named custom param mixes
two meanings in one column. Prefix custom params per project. A param shows in GA4 reports only after it is registered as
a custom dimension; a high-cardinality one collapses to `(other)` — that data
lives in the BigQuery export.

## Few-Shot Examples

### Example 1: Buffer and Dispatch Pattern
**User**: "Fix the analytics dispatcher so it automatically buffers events while Firebase is not ready yet."

**Agent**:
```csharp
public class AnalyticsEventDispatcher
{
    private readonly PendingEventBuffer _pendingBuffer = new();
    private bool _isFirebaseInitialized;

    public void Initialize()
    {
        FirebaseApp.CheckAndFixDependenciesAsync().ContinueWithOnMainThread(task => {
            var dependencyStatus = task.Result;
            if (dependencyStatus == DependencyStatus.Available)
            {
                _isFirebaseInitialized = true;
                FlushPendingEvents();
            }
            else
            {
                Debug.LogError($"Could not resolve all Firebase dependencies: {dependencyStatus}");
            }
        });
    }

    public void TrackEvent(string eventName, Dictionary<string, object> parameters)
    {
        var telemetryEvent = new TelemetryEvent(eventName, parameters, DateTime.UtcNow);

        if (!_isFirebaseInitialized)
        {
            Debug.Log($"[Telemetry] Firebase not ready. Buffering event: {eventName}");
            _pendingBuffer.Add(telemetryEvent);
            return;
        }

        SendToFirebase(telemetryEvent);
    }

    private void FlushPendingEvents()
    {
        var events = _pendingBuffer.GetAllPending();
        foreach (var ev in events)
        {
            SendToFirebase(ev);
        }
        _pendingBuffer.Clear();
    }

    private void SendToFirebase(TelemetryEvent ev)
    {
        // Actual Firebase implementation
        FirebaseAnalytics.LogEvent(ev.Name, ConvertToFirebaseParams(ev.Parameters));
    }
}
```

## Related Skills
- `@unity-async-patterns` - For handling cancellation and UniTask in dispatcher.
- `@unity-qa-generator` - For writing contract verification tests for analytic dispatchers.
