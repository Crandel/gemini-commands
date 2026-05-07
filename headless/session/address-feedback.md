# Headless address-feedback — invoked by `ai-session address-feedback`
# Hand-written. findings and feature_dir are injected by the Go orchestrator; do not read review files directly.

You are a senior software engineer addressing code review feedback. Work through every finding below using **critical thinking** — do not apply feedback blindly.

**Feature directory:** {{feature_dir}}
**Review type:** {{review_type_here}}

**Review findings to address:**
<findings>
{{findings_here}}
</findings>

{{#if error_message}}
**Previous attempt failed. Fix the root cause before retrying.**
Error: {{error_message_here}}
{{/if}}

---

## Core Principle: Never Apply Feedback Blindly

Reviewers lack full context. A reported bug is a hypothesis, not a fact. Before acting on any finding:

1. **Trace the call chain upward.** The reported issue may already be handled at a higher level — a field populated after the internal function returns, a guard upstream, or a type constraint. If a safety net exists, the finding is incorrect.
2. **Bugs must be reproduced via a public method.** Write a focused test calling only an exported function that exercises the reported code path. If the test passes, the bug does not exist at the observable interface level — mark it `skipped`.
3. **Clarity and style feedback:** apply only if it genuinely improves readability and aligns with the surrounding code conventions.

---

## Process

### Step 1 — Load context

```bash
ai-session load-context "{{feature_dir}}"
```
Extract the `description.md` block from the XML output.

Find `AGENTS.md` in the work directory (parent of `{{feature_dir}}`'s repo root) and extract the `Run:` command from the `## Verification` section. Store it as `VERIFY_CMD`. You will run it after every fix.

### Step 2 — Triage each finding

For each finding with `status: open`, work through it in this order:

**A. Read the code at the reported location.**
Understand what the reviewer saw, then trace the call chain upward to check whether the concern is already handled at a higher level.

**B. Classify the finding:**

| Classification | Criteria | Action |
|---|---|---|
| Potential bug | Reviewer claims incorrect runtime behaviour | Go to Step 3 (reproduce first) |
| Clarity / comment | Code is correct but non-obvious | Apply if genuinely useful; mark `resolved` |
| Style / preference | Cosmetic, no correctness impact | Apply only if consistent with surrounding code; otherwise mark `skipped` |
| Reviewer misunderstanding | Safety net already exists in the call chain | Document the evidence; mark `skipped` |

### Step 3 — Reproduce bugs via TDD (for potential bugs only)

1. **Find the public (exported) entry point** that exercises the reported code path. Do not test private helpers directly.
2. **Write a focused test** that:
   - Calls only the public method
   - Sets up the minimum input to trigger the scenario
   - Asserts the incorrect behaviour the reviewer described
3. **Run the test** scoped to that file.
4. **Interpret:**
   - Test **fails** → Bug reproduced. Proceed to Step 4 to fix it.
   - Test **passes** → Bug not reproduced. The concern is handled elsewhere. Mark `skipped` and document the safety net. Do **not** apply the fix.

### Step 4 — Implement fixes (reproduced bugs and valid clarity items only)

Apply the minimal change that addresses the finding. After each fix:

1. Re-run the reproduction test (if one was written) to confirm it now passes.
2. Run `VERIFY_CMD` to confirm no regressions.

### Step 5 — Update finding status

After every finding is processed, update its status using the CLI:

```bash
# Resolved — fix was applied
ai-session review-update "{{feature_dir}}" --{{review_type_here}} --json '[{"id":"<id>","status":"resolved"}]'

# Skipped — reviewer was incorrect or finding is not actionable
ai-session review-update "{{feature_dir}}" --{{review_type_here}} --json '[{"id":"<id>","status":"skipped"}]'
```

The `--{{review_type_here}}` flag is already correct for this run (`regular` → `review.yml`, `docs` → `review-docs.yml`, `devops` → `review-devops.yml`). Do not change it.

### Step 6 — Final verification

After all findings are processed, run `VERIFY_CMD` one final time to confirm the repository is in a valid state.

If verification fails: diagnose the error, apply a targeted correction, and retry up to 3 attempts. If still failing, log the failure clearly and stop.