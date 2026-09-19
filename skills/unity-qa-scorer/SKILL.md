---
name: unity-qa-scorer
description: Use when scoring QA test quality, coverage, risk, ambiguity, or readiness before accepting a Unity feature test suite.
---

# QA Test Scorer

## Overview
Score test case quality using 4 criteria and generate comprehensive score reports. Identifies TCs needing review, failed TCs requiring fixes, and manual test queue.

## When to Use
- Score a generated test case set
- Generate sprint-end score report
- Identify low-quality TCs needing review
- Combine verify results + scores into PM report
- **Do NOT use when:** only need pass/fail classification → use `@unity-qa-verifier`

## Best Practices
- ✅ Score every TC on all 4 criteria
- ✅ Flag TCs below threshold (< 6.0) for review
- ✅ Include actionable recommendations in report
- ✅ Separate failed TCs and manual queue clearly
- ❌ **NEVER** auto-save output files without explicit user request
- ❌ **NEVER** skip low-score TCs — they indicate coverage gaps

## Scoring Formula

```
Score = (coverage × 0.30) + (severity × 0.30) + (automation × 0.20) + (manual × 0.20)

Threshold: ≥ 6.0 = ✅ Good
           < 6.0 = ⚠️ Needs Review
```

## Score Guide (1-10)

| Score | Coverage | Severity | Automation | Manual |
|-------|----------|----------|------------|--------|
| **10** | happy + all edges | crash/data loss/security | pure unit test | UX/animation/platform |
| **7** | happy + some edges | core feature broken | needs mock | UX verify recommended |
| **4** | happy only | wrong result | complex integration | optional |
| **1** | incomplete | cosmetic only | manual only | skip if auto passes |

## Scorer Implementation

```python
def score_tc(tc: dict, weights=None) -> dict:
    w = weights or {"coverage": 0.30, "severity": 0.30, "automation": 0.20, "manual": 0.20}
    total = round(
        tc.get("score_coverage", 0) * w["coverage"] +
        tc.get("score_severity", 0) * w["severity"] +
        tc.get("score_automation", 0) * w["automation"] +
        tc.get("score_manual", 0) * w["manual"], 1
    )
    return {
        "tc_id": tc["id"],
        "total": total,
        "quality": "good" if total >= 6.0 else "needs_review",
    }
```

## Few-Shot Examples

### Example 1: Score Report
**User**: "Generate score report for login feature TCs."

**Agent**:
```markdown
# Score Report — User Login
**Date**: 2026-03-17 | **Tool**: QA Agent-Tester

## Summary

| Metric | Count | % |
|--------|-------|---|
| Total TCs | 10 | 100% |
| ✅ Passed | 7 | 70% |
| ❌ Failed | 1 | 10% |
| ⚠️ Manual | 2 | 20% |

**Average Score**: 7.8/10 *(threshold: 6.0)*

## Scoring Breakdown

| TC ID | Coverage | Severity | Automation | Manual | Total | Quality |
|-------|----------|----------|------------|--------|-------|---------|
| TC-AUTH-F-001 | 9 | 10 | 8 | 7 | 8.7 | ✅ |
| TC-AUTH-F-002 | 8 | 10 | 9 | 4 | 8.3 | ✅ |
| TC-AUTH-F-005 | 5 | 4 | 4 | 8 | 5.2 | ⚠️ |

**Formula**: `(Coverage×0.30) + (Severity×0.30) + (Automation×0.20) + (Manual×0.20)`

## Failed TCs — Action Required

| ID | Title | Priority |
|----|-------|----------|
| TC-AUTH-F-002 | Login banned account — wrong response | P0 |

## Manual Test Queue

| ID | Title | Reason | Est. time |
|----|-------|--------|-----------|
| TC-AUTH-E-003 | Login offline | Physical device test | 5 min |
| TC-AUTH-U-006 | Loading animation | UI/UX verify | 3 min |

## Recommendations
- [ ] **Block release**: TC-AUTH-F-002 — P0 bug
- [ ] **Manual test before demo**: TC-AUTH-E-003, TC-AUTH-U-006
- [ ] **Review**: TC-AUTH-F-005 (score 5.2 < threshold 6.0)
```

## Related Skills
- `@unity-qa-parser` — Parse documents/code into feature plans
- `@unity-qa-generator` — Generate TCs from feature plans
- `@unity-qa-verifier` — Classify test results before scoring
