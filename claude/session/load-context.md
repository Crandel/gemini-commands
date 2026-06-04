---
description: Loads context from a feature directory to start or resume a work session.
---

You are the orchestrator of the `/session:load-context` command. Your goal is to load all context for a given feature and display the key, infrequently changing files as an explicit "Session Context".

The user will provide a feature directory name as an argument (e.g., `sc-12345`). It is available as `$ARGUMENTS`.

**Workflow:**

1.  **Load context:** Run the following command using the Bash tool:
    ```bash
    ai-session load-context $ARGUMENTS
    ```
    `$ARGUMENTS` is the story ID — the sole positional argument to `load-context`.

2.  **Parse output:** The command returns XML blocks: `<file name="filename">…</file>`. Parse the output directly from the tool result. Do NOT re-read any file from disk.
    *   Extract the content of `<file name="description.md">`.
    *   **CRITICAL:** The output ends at the closing `</file>` tag.

3.  **Retain Project Context:** From the same output, find `<file name="AGENTS.md">` (or `<file name="GEMINI.md">` as fallback). This is for your internal use only — retain it in your working memory for the duration of the session. **DO NOT display its content.**

4.  **Display Session Context:** Use the following Markdown format EXACTLY:

    ```markdown
    ### ✨ Session Context Loaded for `$ARGUMENTS`

    **Description:**
    > {{extracted content of description.md}}

    This context is now available for all subsequent commands.
    ```

**CRITICAL:** Only the `description.md` content goes inside the formatted block. All other file contents from the script output are for your internal memory and must not be displayed.
