---
name: unity-qa-generator
description: Use when generating QA test cases, edge cases, regression checks, or Unity feature test matrices from a parsed feature plan or requirements.
---

# QA Test Case Generator

## Overview
Generate high-quality test cases + developer test code (NUnit/PHPUnit) from feature plans or source code. Each TC is scored on 4 criteria for quality assessment.

## When to Use
- Generate TCs from feature plan (output of `@unity-qa-parser`)
- Generate TCs directly from source code
- Produce developer test code (Unity C# NUnit, PHP PHPUnit)
- **Do NOT use when:** writing 1-2 specific unit tests → use `@automated-unit-testing`

## Best Practices
- ✅ Cover both happy paths AND edge cases
- ✅ Follow TC ID convention strictly
- ✅ Include test data for reproducibility
- ✅ Score every TC on all 4 criteria
- ✅ Generate code for automation-ready TCs only
- ❌ **NEVER** auto-save output files without explicit user request
- ❌ **NEVER** skip edge case TCs — they catch real bugs

## Config

| Param | Options | Default |
|-------|---------|---------|
| Platform | `unity_csharp` \| `php` \| `both` | `both` |
| Priority focus | `all` \| `p0_only` \| `p0_p1` | `all` |
| Team size | `1-3` \| `5-10` \| `10+` | `5-10` |
| TC count | number | `10` |

## Prompt

**System:**
```
You are a Senior QA Engineer with 5+ years of mobile game experience.
Expert in Unity C# (NUnit) and PHP (PHPUnit).
Task: Generate high-quality test cases covering happy paths and edge cases.
Score each TC on 4 criteria: coverage, severity, automation_ready, manual_priority.
Return markdown following the template. Do NOT add explanations.
```

**User:**
```
Generate {{count}} test cases for the following feature:

FEATURE PLAN:
{{feature_plan_content}}

CONFIG:
- Platform: {{platform}}
- Priority focus: {{priority_focus}}
- Team size: {{team_size}}

For each TC, provide:
- ID following convention TC-[FEATURE]-[TYPE]-[NUMBER]
- Title, Type, Priority (P0-P3)
- Preconditions, Steps, Expected Result, Test Data
- Scores: coverage (1-10), severity (1-10), automation (1-10), manual (1-10)
- Unity C# test code (if automation_ready)
- PHP test code (if automation_ready)
```

## TC ID Convention

```
TC-[FEATURE_CODE]-[TYPE_CODE]-[NUMBER]
```

| Feature Code | Domain |
|---|---|
| AUTH | Authentication / Login |
| INV | Inventory |
| SHOP | Shop / IAP |
| MATCH | Matchmaking / Game session |
| UI | UI components |
| API | Backend API |

| Type Code | Type |
|---|---|
| F | Functional |
| U | UI / Visual |
| I | Integration |
| P | Performance |
| S | Security |
| E | Edge case |

**Examples:** `TC-AUTH-F-001`, `TC-SHOP-I-003`, `TC-API-S-007`

## Scoring Guide

| Score | Coverage | Severity | Automation | Manual |
|-------|----------|----------|------------|--------|
| 10 | happy + all edges | crash/data loss/security | pure unit test | UX/animation/platform-specific |
| 7 | happy + some edges | core broken | needs mock | UX verify recommended |
| 4 | happy only | wrong result | complex integration | optional |
| 1 | incomplete | cosmetic | manual only | skip if auto passes |

**Formula:** `(coverage×0.30) + (severity×0.30) + (automation×0.20) + (manual×0.20)`
**Threshold:** ≥ 6.0 = good, < 6.0 = needs_review

## Few-Shot Examples

### Example 1: Generate TC with NUnit + PHPUnit
**User**: "Generate test cases for login feature."

**Agent**:
```markdown
## TC-AUTH-F-001: Successful login with Google OAuth

- **Priority**: P0
- **Type**: Functional
- **Score**: 8.5/10
- **Automation Ready**: Yes
- **Preconditions**: App installed, internet connected

### Steps
1. Open app
2. Tap "Login with Google" button
3. Select account and approve

### Expected Result
- Enters Main Menu screen
- session_id is saved
- API returns HTTP 200

### Test Data
- Account: test_valid@gmail.com
```

```csharp
[Test]
public void LoginSuccess_ValidToken_EntersMainMenu()
{
    var mockToken = TestData.VALID_GOOGLE_TOKEN;
    var result = AuthManager.LoginWithGoogle(mockToken);

    Assert.IsTrue(result.Success);
    Assert.IsNotNull(result.SessionId);
    Assert.AreEqual(GameState.MainMenu, GameManager.CurrentState);
}
```

```php
/** @test */
public function post_auth_google_valid_token_returns_session(): void
{
    $this->mockGoogleOAuth(['email' => 'valid@gmail.com']);
    $response = $this->postJson('/api/auth/google', [
        'google_token' => 'valid_mock_token',
    ]);

    $response->assertStatus(200);
    $response->assertJsonStructure(['session_id', 'player_data']);
}
```

## Related Skills
- `@unity-qa-parser` — Parse documents/code before generating TCs
- `@unity-qa-verifier` — Classify test results
- `@unity-qa-scorer` — Detailed TC quality scoring
- `@automated-unit-testing` — Write specific unit tests (NUnit)
