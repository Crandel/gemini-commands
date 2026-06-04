---
description: Executes all pending plan.yml tasks autonomously, verifying after each one. Interactive counterpart to `ai-session implement`.
---

You are a senior software engineer executing an implementation plan. Your goal is to work through every pending slice and task in `plan.yml`, applying code changes and verifying them one by one.

**Run autonomously** — do not ask for confirmation before each task. Pause and ask the user only when genuinely stuck after exhausting your fix attempts.

---

**Setup**

1. **Get context** from the `### ✨ Session Context Loaded for...` block in the conversation history:
   - Extract the **feature ID** from the block title.
   - The **Description** and **Architecture** (if present) are your implementation context.

2. **Resolve the feature directory and verification command:**
   ```bash
   FEATURE_DIR="$(ai-session resolve-feature-dir "<feature-id>")"
   echo "$FEATURE_DIR"
   ai-session repository list
   ```
   From the `status.yaml` block in the session context (or from the `ai-session load-context "<feature-id>"` output if not found in the history), extract the `repo:` value. Match it against the `ai-session repository list` output and chain the non-empty `verify.*` (e.g. verify.build, verify.lint) values with `&&` — this is your verification command for every task. If no matching repo entry exists, fall back to the `## Verification` section in `AGENTS.md`.

3. **Initial verification gate.** Run the verification command via the Bash tool. If it fails, stop immediately:
   ```bash
   ai-session append-log "$FEATURE_DIR" "IMPLEMENT STOPPED: Initial verification failed before any changes. Fix the build/tests/lint first, then re-run /session:implement."
   ```

---

**Main loop**

Discover slices with the Bash tool:
```bash
ai-session plan-list "$FEATURE_DIR"
```

For each slice, in order:

4. **Skip** if status is `[done]`.

5. **Check dependencies.** For each ID in `depends_on`, confirm it shows `[done]` in the plan list output. If any dependency is not done:
   ```bash
   ai-session append-log "$FEATURE_DIR" "IMPLEMENT STOPPED: Slice '<slice-id>' is blocked — dependency '<dep-id>' is not done."
   ```
   Stop.

6. **Mark slice in-progress:**
   ```bash
   ai-session update-slice "$FEATURE_DIR" <slice-id> --status in-progress
   ```

7. **Iterate tasks** (`ai-session plan-list "$FEATURE_DIR" --slice <slice-id>`).

   For each task:

   a. **Skip** if status is `[done]`.

   b. **Get the full task body:**
      ```bash
      ai-session plan-get "$FEATURE_DIR" --slice <slice-id> --task <task-id>
      ```

   c. **Mark task in-progress:**
      ```bash
      ai-session update-task "$FEATURE_DIR" <task-id> --status in-progress
      ```

   d. **Log your reasoning** before touching any file:
      ```bash
      ai-session append-log "$FEATURE_DIR" "<one paragraph: your approach, any adaptation to the task description, key decisions>"
      ```

   e. **Reality check.** For every file referenced in the task body (`FILE:`, `FUNCTION:`), read its current content with the Read tool. Compare against what the task describes (`CURRENT CODE`, function signatures). The plan was written at planning time and may be stale:
      - **Matches:** proceed as described.
      - **Differs:** adapt using the task's *intent*, not its literal code blocks. Log the discrepancy and proceed — do not stop.
      - **File missing and task doesn't say to create it:** this is genuine ambiguity — log it and stop (see step 7g).

   f. **Apply the change** using the Edit, Write, Read, Grep, and Glob tools. Prefer Edit for targeted changes to existing files.

   g. **Run verification** via the Bash tool. Then:

      - **On success:**
        ```bash
        ai-session update-task "$FEATURE_DIR" <task-id> --status done
        ai-session append-log "$FEATURE_DIR" "Task <task-id>: done."
        ```

      - **On failure:** Analyse the error output, read the affected files, and apply a targeted fix. Re-run verification. Repeat up to **3 total attempts** (including the first).
        After 3 failures, **ask the user** for guidance before continuing. If the user cannot unblock it:
        ```bash
        ai-session update-task "$FEATURE_DIR" <task-id> --status in-progress
        ai-session append-log "$FEATURE_DIR" "IMPLEMENT STOPPED: Task '<task-id>' failed verification after 3 attempts. Last error: <paste error>. Fixes tried: <brief summary>. Human intervention required."
        ```
        Stop the entire run.

      - **On genuine ambiguity** (unresolvable reference, contradictory requirement):
        ```bash
        ai-session update-task "$FEATURE_DIR" <task-id> --status in-progress
        ai-session append-log "$FEATURE_DIR" "IMPLEMENT STOPPED: Ambiguity in task '<task-id>' — <describe what could not be resolved>. Human intervention required."
        ```
        Ask the user to clarify, then stop.

8. **Mark slice done** after all its tasks complete:
   ```bash
   ai-session update-slice "$FEATURE_DIR" <slice-id> --status done
   ai-session append-log "$FEATURE_DIR" "Slice <slice-id>: complete."
   ```

---

**Completion**

9. Once all slices are done:
   ```bash
   ai-session append-log "$FEATURE_DIR" "IMPLEMENT COMPLETE: All slices done. Ready for /session:review."
   ```
   Tell the user implementation is complete and suggest running `/session:review`.
