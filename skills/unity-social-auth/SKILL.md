---
name: unity-social-auth
description: "Use when implementing or debugging social login (Google, Facebook OAuth), managing account linking states, localizing authentication error popups, or checking internet status before authenticating."
---

# Unity Social Authentication & Popup Management

## Overview
Guidelines for managing Google and Facebook OAuth logins, checking network states prior to auth requests, localizing authentication errors, and handling link/unlink state displays on UI buttons.

## When to Use
- Adding or modifying social authentication buttons/scenes.
- Implementing logic for linking/unlinking social media accounts to a guest profile.
- Handling popup displays when OAuth fails or if the account is already bound to another profile (`LinkAccountFailedAlreadyLinked`).
- Resolving network timeouts or "no internet connection" alerts during auth.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Link/Unlink State** | Displaying whether an account is currently linked (e.g. grayed out, checkmark, or change button label). |
| **LinkAccountFailedAlreadyLinked** | A core error code signifying that the social account is already attached to another game profile. |
| **Social Login Popup** | Unified UI component that presents login options (Google, FB, Guest) and handles callbacks. |
| **Social Auth Error Handler** | System that converts raw OAuth/Firebase exceptions into localized user-friendly popups. |
| **Focus Helper Timeout** | Fallback MonoBehaviour that fires a cancel/failure callback when the native SDK callback is lost after app switching. |

## Best Practices
- ✅ **Always** check `Application.internetReachability` or ping the API before attempting OAuth to avoid long timeouts.
- ✅ Localize error texts dynamically based on the error code (e.g., Google/FB account already linked to another player).
- ✅ Update the button interactability and text labels immediately when link status changes.
- ✅ Provide a fallback Guest Login option in case external OAuth services are blocked or slow.
- ❌ **NEVER** leave a loading/waiting overlay visible indefinitely if an authentication request fails.
- ❌ **NEVER** allow double-clicking or spamming the Google/FB button while login is in progress (block UI raycasts).
- ✅ Handle the lost-callback case: the Facebook SDK callback can never fire when the user backs out of the Facebook app without logging in (especially FB app installed but no account). Use a Focus Helper: on `OnApplicationFocus(true)` start a ~1.5s timer that triggers the cancel/failure callback if the SDK callback hasn't arrived; cancel the timer when the real callback fires or focus is lost again.
- ✅ Distinguish auth failure from network failure in error routing — collapsing them produces false "no internet" popups on OAuth errors.
- ✅ If a post-login popup shows player stats, capture its data snapshot BEFORE queueing a cross-scene pending trigger; capturing after the scene load shows zeros.
- ❌ **NEVER** read a locally cached list (owned items, submitted entries, …) immediately after a server write — the local cache only refreshes on the next fetch, so counts are stale; re-fetch or compensate explicitly.

## Few-Shot Examples

### Example 1: Check Connection and Handle Auth Errors
**User**: "Fix the Google/FB button hanging when there is no internet, or when the account is already linked to another account."

**Agent**:
```csharp
public class SocialLoginController : MonoBehaviour
{
    [SerializeField] private Button _googleButton;
    [SerializeField] private Button _facebookButton;
    [SerializeField] private CanvasGroup _loadingOverlay;

    public async UniTaskVoid OnClickGoogleLogin(CancellationToken ct)
    {
        if (Application.internetReachability == NetworkReachability.NotReachable)
        {
            PopupManager.ShowError("No Internet Connection. Please check your network and try again.");
            return;
        }

        SetLoading(true);
        try
        {
            var authResult = await AuthManager.Instance.LoginWithGoogleAsync(ct);
            if (authResult.Success)
            {
                SceneLoader.LoadScene("MainMenu");
            }
        }
        catch (AuthException ex)
        {
            HandleAuthError(ex.ErrorCode);
        }
        finally
        {
            SetLoading(false);
        }
    }

    private void HandleAuthError(AuthErrorCode code)
    {
        string message = code switch
        {
            AuthErrorCode.LinkAccountFailedAlreadyLinked => "This Google/Facebook account is already linked to another game profile.",
            AuthErrorCode.NetworkTimeout => "Network timeout. Please try again.",
            _ => "An unexpected error occurred during login. Please try again."
        };
        PopupManager.ShowError(message);
    }

    private void SetLoading(bool isLoading)
    {
        _loadingOverlay.alpha = isLoading ? 1f : 0f;
        _loadingOverlay.blocksRaycasts = isLoading;
        _googleButton.interactable = !isLoading;
        _facebookButton.interactable = !isLoading;
    }
}
```

## Related Skills
- `@unity-ugui-layout` - For popup canvas and loading overlay positioning.
- `@unity-event-safety` - For cleaning up OAuth callback subscriptions.
