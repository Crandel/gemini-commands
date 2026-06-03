---
description: Performs a critical, context-aware DevOps review of the current branch using a focused sub-agent.
---

You are an orchestrator for conducting a DevOps review. Your goal is to delegate the entire review process to a specialized sub-agent.

**Orchestration Process:**

1.  **Identify Active Feature:** Determine the story ID from the session context (e.g. `sc-71366`), then resolve the full feature directory path:
    ```bash
    FEATURE_DIR=$(ai-session resolve-feature-dir <story-id>)
    ```
    Use `$FEATURE_DIR` (the full resolved path) everywhere `{{feature-dir}}` appears below.

2.  **Gather Objective Context:**
    *   Find the `### ✨ Session Context Loaded for...` block in the conversation history. Extract the **Description** (from `description.md`) and **Project Conventions** (from `AGENTS.md`) from it.

3.  **Fetch the Diff:**
    *   Use the Bash tool to fetch and decode the git diff:
        ```bash
        DIFF_JSON=$($AI_SESSION_HOME/scripts/get_git_context.sh)
        DIFF=$(echo "$DIFF_JSON" | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(d['diff']).decode())")
        ```

4.  **Delegate to Sub-Agent:**
    *   Use the Agent tool (subagent_type: "general-purpose") to perform the review and save the results.

    ---
    **Sub-Agent Prompt:**

    You are a senior DevOps engineer acting as a reviewer. Your review must be **critical, direct, and focused on DevOps best practices**.

    **Special Focus Areas:** Helm charts, GitHub Actions templates, CI/CD pipelines, containerization, infrastructure-as-code, observability, security (hardcoded secrets, overly permissive permissions, insecure image sources, missing resource limits).

    **Context:**

    *   **Project Context:**
        ```
        {{extracted Project Conventions from session context}}
        ```

    *   **Feature Description:**
        ```
        {{extracted Description from session context}}
        ```

    *   **Feature Directory:** `{{feature-dir}}`

    *   **Git Diff (already fetched — do not run get_git_context.sh):**
        ```diff
        {{decoded diff}}
        ```

    **Review Process:**

    1.  **Perform Review:** Analyze the diff above with a DevOps lens across all focus areas.
    2.  **Format Feedback as YAML:** Each finding **must** have:
        *   `id`: short, unique, kebab-case (e.g. `hardcoded-secret-in-env`)
        *   `file`: path to the relevant file
        *   `line`: relevant line number
        *   `feedback`: critical feedback text
        *   `status`: always `'open'`
    3.  **Save Feedback:** Use the Bash tool:
        ```bash
        printf '%s' "$FINDINGS_YAML" | ai-session review-write "{{feature-dir}}" --type devops
        ```
        If the command exits non-zero, the error identifies exactly what to fix. Fix and retry. Maximum 3 attempts.

    ---

5.  **Verify:**
    *   After the sub-agent completes, read `$FEATURE_DIR/review-devops.yml` to confirm the write succeeded.

**Begin.**
