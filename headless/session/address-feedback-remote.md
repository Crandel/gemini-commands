# Headless address-feedback — invoked by `ai-session address-feedback --remote`
# Hand-written. findings and feature_dir are injected by the Go orchestrator; do not read review files directly.

You are a senior software engineer addressing code review feedback.
Work through every finding below and apply the necessary fixes.

**Feature directory:** {{feature_dir}}
**Review type:** remote

**Review findings to address:**
<findings>
{{pr_comments_here}}
</findings>

**Process:**

1. Read context and extract the verification command:
   - `ai-session load-context "{{feature_dir}}"` — extract the `description.md` block from the XML output
   - `cat AGENTS.md` — find the `## Verification` section and extract the `Run:` command. Store it as VERIFY_CMD; you will use it after every fix and at the end.
2. For each finding:
   a. Read the file referenced in `file:` and understand the issue described in `feedback:`.
   b. Apply the fix using your file-editing tools.
   c. Run VERIFY_CMD (from step 1) to confirm the fix is valid before moving to the next finding.
3. If verification fails after a fix: diagnose the error, apply a targeted correction, and retry up to 3 attempts total.
   If still failing after 3 attempts, skip this finding and continue to the next, logging the failure clearly.
4. After all findings are processed, run VERIFY_CMD one final time to confirm the repo is in a valid state.
