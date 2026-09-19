---
name: unity-qa-parser
description: Use when turning QA documents, feature specs, acceptance criteria, screenshots, or Unity code into structured feature plans and testable requirements.
---

# QA Document & Code Parser

## Overview
Parse feature documents (MD/PDF/Word/Notion/Figma) or source code (C#/PHP) into structured feature plans (markdown). Output feeds into `@unity-qa-generator`.

## When to Use
- Parse feature documents into structured plans
- Read existing C#/PHP code to extract logic, dependencies, edge cases
- Supplement documentation with insights from newly completed code
- **Do NOT use when:** feature plan already exists → use `@unity-qa-generator` directly

## Best Practices
- ✅ Include both happy paths AND edge cases from the source
- ✅ Extract API endpoints with method, path, description
- ✅ Identify platform scope (Unity client, PHP backend, or both)
- ✅ Rate complexity to guide test case count
- ❌ **NEVER** auto-save output files without explicit user request
- ❌ **NEVER** omit edge cases — they are critical for QA coverage

## Modes

| Mode | Input | Use For |
|------|-------|---------|
| **From Document** | MD/PDF/Word/Notion/Figma | Feature spec parsing |
| **From Code** | C#/PHP source files | Extracting logic + edge cases to supplement docs |

## Prompt — From Document

**System:**
```
You are a QA Analyst specializing in mobile games (Unity C# client + PHP backend).
Task: Analyze feature documents and extract structured information.
Return markdown following the template. Do NOT add explanations.
```

**User:**
```
Analyze the following feature document and return a feature plan:

DOCUMENT:
{{document_content}}

OUTPUT FORMAT (markdown):
# Feature Plan: [feature name]

## Actors
- [actor 1]

## Happy Paths
### [Flow name]
1. [step]

## Edge Cases
- [edge case]

## Business Rules
- [rule]

## API Endpoints
| Method | Path | Description |
|--------|------|-------------|

## Platforms
- [ ] Unity Client
- [ ] PHP Backend

## Complexity: low|medium|high
```

## Prompt — From Code

**System:**
```
You are a QA Analyst specializing in mobile games (Unity C# client + PHP backend).
Task: Read source code and extract logic, dependencies, edge cases
to supplement the feature plan.
Return markdown following the template. Do NOT add explanations.
```

**User:**
```
Read the following source code and extract a feature plan:

CODE:
{{source_code}}

Focus on:
- Public methods and core logic
- Input validation and error handling
- Dependencies and external calls
- Potential edge cases from code

OUTPUT FORMAT: (same template as From Document)
```

## Few-Shot Examples

### Example 1: Parse Feature Document
**User**: "Parse the login feature document."

**Agent**:
```markdown
# Feature Plan: User Login with Google OAuth

## Actors
- Player
- Auth Server
- Unity Client
- PHP Backend

## Happy Paths
### Standard login flow
1. Player opens app, sees login screen
2. Player taps Login with Google
3. Google OAuth popup appears
4. Player selects account and approves
5. App receives access token
6. PHP backend validates token and creates session
7. Unity client receives session_id and player_data
8. Player enters main menu

## Edge Cases
- Token expires after 1 hour
- Account is banned (status=banned)
- No internet connection
- First-time login vs returning player

## Business Rules
- Banned accounts cannot enter game
- New player triggers onboarding tutorial
- Session expires after 24h of inactivity

## API Endpoints
| Method | Path | Description |
|--------|------|-------------|
| POST | /api/auth/google | Validate Google token |
| GET | /api/player/profile | Fetch player data |
| POST | /api/auth/refresh | Refresh session token |

## Platforms
- [x] Unity Client
- [x] PHP Backend

## Complexity: medium
```

## Related Skills
- `@unity-qa-generator` — Generate TCs from feature plan
- `@unity-qa-verifier` — Classify test results
- `@unity-qa-scorer` — Score test case quality
