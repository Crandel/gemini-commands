---
description: Saves a checkpoint of the work done by updating the state files.
---

You are an assistant that helps log work-in-progress and update the session state.

**Actions:**

1.  **Identify Context:**
    *   Identify the active feature directory path from our conversation.
    *   Review our conversation since the last checkpoint to determine which tasks were completed and which questions were answered.

2.  **Update State Files:**
    *   For each completed task ID, update its status using the CLI:
        Task update: `ai-session update-task <feature-id> <task-id> --status done`
        Slice update: `ai-session update-slice <feature-id> <slice-id> --status done`
    *   For each answered question, fetch the current questions, update the relevant entries in memory, then write back atomically:
        ```bash
        ai-session plan get --questions "<feature-id>"
        # modify status to 'resolved' and set 'answer' for each answered question in memory
        printf '%s' "$UPDATED_QUESTIONS_YAML" | ai-session plan write --questions "<feature-id>"
        ```

3.  **Update Log (`log.md`):**
    *   Generate a concise Markdown summary of the work done.
    *   Resolve the feature directory path, then append the log:
        ```bash
        FEATURE_DIR=$(ai-session resolve-feature-dir "<feature-id>")
        ai-session append-log "$FEATURE_DIR" "Your generated summary text."
        ```

4.  **Confirm:** State that the checkpoint has been saved.
