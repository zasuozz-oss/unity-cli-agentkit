---
name: unity-qa-verifier
description: Use when classifying QA execution results, comparing expected versus actual behavior, or turning Unity test evidence into pass/fail/blocked findings.
---

# QA Test Verifier

## Overview
Classify test results from log text (NUnit, PHPUnit, or manual test notes) into 5 statuses. Output feeds into `@unity-qa-scorer`.

## When to Use
- Classify results from NUnit/PHPUnit log output
- Classify manual test notes
- Identify flaky tests (inconsistent pass/fail)
- **Do NOT use when:** need to write tests → use `@unity-qa-generator` or `@automated-unit-testing`

## Best Practices
- ✅ Map every TC ID to exactly one status
- ✅ Include error details for failed TCs
- ✅ Suggest fixes for failures when possible
- ✅ Flag flaky tests explicitly — they need investigation
- ❌ **NEVER** auto-save output files without explicit user request
- ❌ **NEVER** mark a TC as `pass` if the log shows warnings related to it

## Status Definitions

| Status | Description | Example |
|--------|-------------|---------|
| `pass` | Test succeeded, no errors | Assert passed, response 200 |
| `fail` | Assert failed or exception thrown | Assert failed, NullRef, timeout |
| `manual` | Cannot be auto-tested | UI animation, haptic, UX flow |
| `skip` | Dependency not ready | API not deployed, mock data missing |
| `flaky` | Inconsistent pass/fail | Race condition, timing-dependent |

## Prompt

**System:**
```
You are a QA Engineer specializing in test result classification.
Task: Read test logs and classify each test case by status.
Return markdown following the template. Do NOT add explanations.
```

**User:**
```
Read the following test log and classify each test case:

TEST LOG:
{{test_log_output}}

TEST CASE IDs to classify:
{{tc_id_list}}

Possible statuses: pass, fail, manual, skip, flaky

OUTPUT FORMAT:
# Verify Results — {Feature Name}

| TC ID | Status | Notes |
|-------|--------|-------|
| TC-XXX-X-001 | pass/fail/manual/skip/flaky | Short note |

## Summary
- ✅ Passed: X
- ❌ Failed: X
- ⚠️ Manual: X
- ⏭️ Skipped: X
- 🔄 Flaky: X

## Failed Details
### TC-XXX-X-00N
- **Error:** [error message]
- **Log:** [relevant log snippet]
- **Suggested fix:** [remediation approach]
```

## Few-Shot Examples

### Example 1: Classify NUnit Test Log
**User**: "Classify these test results."

**Input:**
```
[PASS] LoginSuccess_ValidToken_EntersMainMenu (0.12s)
[FAIL] LoginBannedAccount_ShowsErrorAndBlocksEntry (0.08s)
  Expected: "ACCOUNT_BANNED"
  But was: null
  at AuthManagerTests.cs:line 42
[PASS] SessionExpired_AutoRefreshesToken (0.45s)

TC IDs: TC-AUTH-F-001, TC-AUTH-F-002, TC-AUTH-E-003
```

**Agent**:
```markdown
# Verify Results — User Login

| TC ID | Status | Notes |
|-------|--------|-------|
| TC-AUTH-F-001 | pass | Login succeeded (0.12s) |
| TC-AUTH-F-002 | fail | Assert ErrorCode null, expected ACCOUNT_BANNED |
| TC-AUTH-E-003 | pass | Token auto-refresh OK (0.45s) |

## Summary
- ✅ Passed: 2
- ❌ Failed: 1

## Failed Details
### TC-AUTH-F-002
- **Error:** Expected "ACCOUNT_BANNED" but was null
- **Log:** AuthManagerTests.cs:line 42
- **Suggested fix:** Check AuthManager.LoginWithGoogle — ErrorCode not set for banned accounts
```

## Related Skills
- `@unity-qa-parser` — Parse documents/code into feature plans
- `@unity-qa-generator` — Generate TCs from feature plans
- `@unity-qa-scorer` — Score TCs after verification
