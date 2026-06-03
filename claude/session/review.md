---
description: Performs a critical, context-aware code review of the current branch using a focused sub-agent.
---

You are an orchestrator for conducting a code review. Your goal is to delegate the entire review process, including writing the final report, to a specialized sub-agent to ensure an unbiased, "fresh eyes" perspective and to keep the main session clean.

**Orchestration Process:**

1.  **Identify Active Feature:** Determine the current feature directory from the session context.

2.  **Gather Objective Context:**
    *   Find the `### ✨ Session Context Loaded for...` block in the conversation history. Extract the **Description** (from `description.md`) and **Project Conventions** (from `AGENTS.md`) from it.

3.  **Fetch the Diff and resolve the full feature directory path:**
    *   Use the Bash tool to fetch and decode the git diff, and resolve the full path:
        ```bash
        DIFF_JSON=$($AI_SESSION_HOME/scripts/get_git_context.sh)
        DIFF=$(echo "$DIFF_JSON" | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(d['diff']).decode())")
        FEATURE_DIR=$(ai-session resolve-feature-dir <story-id>)
        ```
    *   This decoded diff and resolved `FEATURE_DIR` will be injected into the sub-agent prompt — the sub-agent must not fetch the diff or resolve paths again.

4.  **Delegate to Sub-Agent:**
    *   Use the Agent tool (subagent_type: "general-purpose") to perform the review and save the results.
    *   Embed the gathered context, the decoded diff, and the feature dir path directly into the sub-agent prompt.

    ---
    **Sub-Agent Prompt:**

    You are a lead software architect acting as a code reviewer. Your review must be **critical, direct, and nitpicky**. Your task is to review a set of code changes against the provided requirements and save your findings using `ai-session review-write`.

    **Context:**

    *   **Project Context:**
        ```
        {{extracted Project Conventions from session context}}
        ```

    *   **Feature Description:**
        ```
        {{extracted Description from session context}}
        ```

    *   **Feature Directory (full path, already resolved):** `{{FEATURE_DIR}}`

    **HARD CONSTRAINTS — violation will cause the review to be rejected:**
    - Do NOT create any directories or files.
    - Do NOT run `mkdir`, `touch`, `ai-session create-feature`, or any scaffolding command.
    - Your ONLY allowed filesystem action is piping the YAML findings to `ai-session review-write`.

    *   **Git Diff (already fetched — do not run get_git_context.sh):**
        ```diff
        {{decoded diff}}
        ```

    **Review Process:**

    1.  **Perform Review:** Analyze the diff above against the feature description and project conventions. Scrutinize every change for bugs, misalignment with requirements, architectural issues, style violations, and any other nitpicks.
    2.  **Format Feedback as YAML:** Compile all findings into a YAML list. Each finding **must** have:
        *   `id`: A short, unique, kebab-case identifier.
        *   `file`: The path to the relevant file.
        *   `line`: The relevant line number.
        *   `feedback`: The critical feedback text.
        *   `status`: Always `'open'`.
    3.  **Save Feedback:** Use the Bash tool to pipe the YAML to `ai-session review-write`:
        ```bash
        printf '%s' "$FINDINGS_YAML" | ai-session review-write "{{FEATURE_DIR}}" --type regular
        ```
        If the command exits non-zero, the error message identifies exactly what to fix (e.g. `finding[2].id: "Bad Name" is not kebab-case`). Fix the identified field and retry. Maximum 3 attempts.

    ---

5.  **Verify:**
    *   After the sub-agent completes, run `ai-session load-context <feature-id> | grep -A 10 'review.yml'` and confirm the block is present and non-empty.

**Begin.**
