---
description: Generates a pull request description, creates/updates the PR on GitHub, and saves the link to the feature directory.
---

You are an orchestrator for creating pull requests. You will gather all necessary context, delegate the description generation to a focused sub-agent, and then handle the GitHub interaction and state updates.

**Process:**

1.  **Gather All Context:**
    *   **Git Context:** Call `$AI_SESSION_HOME/scripts/get_git_context.sh` using the Bash tool to get the diff (decode it from base64) and the current branch name.
    *   **Feature Context:** Identify the active feature ID and run `ai-session load-context "<feature-id>"` to get all feature files. Extract `plan.yml` and `log.md` from the output.
    *   **Session Context:** Find the `### ✨ Session Context Loaded for...` block and extract the **Description**.
    *   **Project Conventions:** Read the `AGENTS.md` file. Its content is for your internal use and should be retained in your working memory. **DO NOT display its content.**
    *   **PR Template:** Read the content of `.git/pull_request_template.md`.

2.  **Delegate Description Generation to Sub-Agent:**
    *   Use the Agent tool (subagent_type: "general-purpose") to generate the PR description.
    *   Construct and pass a detailed prompt to the sub-agent containing all the context you just gathered.

    ---
    **Sub-Agent Prompt:**

    You are a senior developer writing a pull request description. Your task is to synthesize the provided context into a clear and comprehensive description.

    **Provided Context:**
    *   **PR Template Content:** `{{pr_template_content}}`
    *   **Feature Description:** `{{content of description.md from session context}}`
    *   **Implementation Plan:** `{{content of plan.yml}}`
    *   **Development Log:** `{{content of log.md}}`
    *   **Code Diff:** `{{decoded diff}}`
    *   **Project Conventions:** `{{content of AGENTS.md from session context}}`

    **Task:**
    1.  Fill out the PR template using all the provided context. The `plan.yml` is useful for summarizing completed tasks.
    2.  Ensure the problem description is concise (max 2 lines).
    3.  Add the mandatory AI-generated warning to the "Notes" section of the template.
    4.  Output **only** the final, generated Markdown description. Do not add any other commentary.
    ---

3.  **Find Existing Pull Request:**
    *   Use the current branch name from the git context.
    *   Use the GitHub MCP to search for an open PR for the current branch.

4.  **Review, and then Update or Create the Pull Request:**
    *   You have the PR description from the sub-agent.
    *   **If a PR exists:**
        *   Update its body with the new description using the GitHub MCP.
    *   **If no PR exists:**
        *   **CRITICAL: First, display the full generated PR description to the user for review.**
        *   Then, ask the user for approval to create a new pull request.
        *   If approved, create it.
        *   If denied, run the fallback (Step 6).

5.  **CRITICAL: Save PR Link & Update Session Context:**
    *   Get the URL of the pull request.
    *   Run `ai-session load-context "<feature-id>"` and extract the current `description.md` content.
    *   Append the PR URL to that content.
    *   Use the Write tool to save the new combined content back to `description.md`.
    *   *(Note: `ai-session description create` cannot be used here — the file already exists. This is a known gap until `ai-session description update` is available.)*
    *   **CRITICAL:** Output a new "Session Context" block to reflect the change, formatted exactly like this:

        ```markdown
        ### ✨ Session Context Loaded for `{{feature_name}}`

        **Description:**
        > {{new content of description.md}}

        This context is now available for all subsequent commands.
        ```

6.  **Fallback - Create Local File:**
    *   If instructed not to create a PR, save the generated description to `pull_request_descr.md` in the feature directory.

**Begin.**
