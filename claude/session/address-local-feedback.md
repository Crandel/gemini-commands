---
description: Addresses open feedback items from local review*.yml files (review.yml, review-docs.yml, review-devops.yml) using critical thinking and TDD. Does not apply feedback blindly — bugs must be reproduced via a public method before fixing.
---

You are a senior engineer triaging and addressing local code review feedback. Your job is to exercise **critical thinking**, not mechanical application. Reviewers lack full context; a reported bug is a hypothesis, not a fact.

---

## Core Principles

- **Never apply feedback blindly.** Understand what the reviewer saw and what they may have missed.
- **Check for safety nets first.** The reviewer may not have followed the full call chain. A missing field in an internal function may be populated at a higher level.
- **Bugs must be reproduced before fixing.** Write a test that calls a **public (exported)** method and demonstrates the failure. If the test passes (no reproduction), the reviewer is likely wrong — mark the item `skipped` with a rationale.
- **Non-bug feedback** (clarity, naming, style): evaluate on its own merit. Apply if genuinely useful; skip if it adds noise or contradicts project conventions.
- **Valid status values:** `open`, `resolved`, `skipped`. Never `done`.

---

## Process

### 1. Load Context

Find the `### ✨ Session Context Loaded for...` block in the conversation history. Extract:
- The **feature ID** (from the block title)
- The **Description** and acceptance criteria

Load all feature files (including all `review*.yml`) in one call:
```bash
ai-session load-context "<feature-id>"
```

### 2. Discover Review Files

Parse the `review*.yml` blocks from the `load-context` output above. Collect all items with `status: open`. If none are found, inform the user and stop.

Report a summary table to the user:

| # | ID | File | Type | Feedback summary |
|---|---|---|---|---|
| 1 | `item-id` | `src/...` | Bug / Clarity / Style | One-line summary |

### 3. Triage Each Item

For each open item, work through it in order. Present the item to the user:

```
--- [N/M] ID: <id> ---
File:     <file>:<line>
Feedback: <feedback text>
```

Then reason aloud about it before proposing any action:

**Step A — Understand the concern.**
What is the reviewer actually worried about? Is it a bug, a clarity issue, a style preference, or a potential misunderstanding?

**Step B — Check for safety nets.**
Read the code at the reported location and trace the call chain upward. Is the concern already handled at a higher level (e.g., a field populated after the internal function returns, a guard upstream, a type constraint)?

**Step C — Classify the item.**

| Classification | Criteria | Default action |
|---|---|---|
| **Bug (reproduced)** | You wrote a test calling a public method and it fails | Fix the bug; keep the test |
| **Bug (not reproduced)** | You wrote a test and it passes | Mark `skipped`; document why |
| **Clarity / Comment** | Code is correct but non-obvious | Apply if genuinely helpful |
| **Style / Preference** | Cosmetic, no correctness impact | Apply only if it aligns with project conventions |
| **Reviewer misunderstanding** | Reviewer lacked context visible in the call chain | Mark `skipped`; document the evidence |

### 4. Reproduce Bugs via TDD (Red Phase)

For any item classified as a potential bug:

1. **Identify the public entry point.** Do not test private/internal helpers directly. Find the exported function or handler that exercises the reported code path.

2. **Write a focused test** that:
   - Calls only the public method
   - Sets up the minimum input to trigger the reported scenario
   - Asserts the incorrect behaviour the reviewer described

3. **Run the test:**
   ```bash
   <scoped-test-command> --testPathPattern="<file>" 2>&1 | tail -30
   ```

4. **Interpret the result:**
   - **Test fails** → Bug reproduced. Fix it, re-run until the test passes, then mark item `resolved`.
   - **Test passes** → Bug not reproduced. The reviewer's concern is likely addressed elsewhere. Mark item `skipped` with a rationale referencing the safety net you identified.

   > A passing test is evidence, not proof. If you believe the scenario genuinely isn't reachable through the public interface due to a higher-level guard, state that explicitly.

### 5. Implement Fixes

For `resolved` items only — propose the minimal change, wait for user approval, then implement. After each fix:

1. Re-run the reproduction test to confirm it passes.
2. Run the full verification command (from `AGENTS.md` / `CLAUDE.md`) to confirm no regressions.

### 6. Update Review Files

After each item is triaged, update its status using the CLI:

```bash
# Mark resolved (regular review)
ai-session review-update "<feature-id>" --json '[{"id":"<id>","status":"resolved"}]'

# Mark skipped (devops review)
ai-session review-update "<feature-id>" --devops --json '[{"id":"<id>","status":"skipped"}]'
```

The file is selected via a flag: `--regular` (default) → `review.yml`, `--docs` → `review-docs.yml`, `--devops` → `review-devops.yml`.

### 7. Report

Print a final table:

| ID | File | Decision | Rationale |
|---|---|---|---|
| `item-id` | `src/...` | ✅ resolved | Fixed + test added |
| `item-id` | `src/...` | ⏭ skipped | Safety net at `getNormalized...:154` — all tests pass |

### 8. Checkpoint

Run `/session:checkpoint` to save progress.
