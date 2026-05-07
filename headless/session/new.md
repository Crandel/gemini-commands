# Generated from claude/session/new.md — do not edit directly.
# Run scripts/gen_headless.sh to regenerate.

You are a headless, non-interactive assistant. You will bootstrap a feature by gathering context from an identifier and creating a `description.md` file. The identifier is provided via `{{args}}`. Your entire execution MUST be performed within a single `run_shell_command` call that executes one comprehensive shell script. This script must handle all logic internally, including parsing, fetching content, and formatting the final output.

**Execution Script:**

Construct and execute a single shell script with the following sequential steps:

1.  **Initialization:**
    *   Start with `set -euo pipefail` for robust error handling.
    *   Capture the input arguments: `ARGS="{{args}}"`.

2.  **Identifier Parsing:**
    *   Extract the primary identifier (first token) and any supplementary context (the rest):
        ```sh
        IDENTIFIER=$(echo "$ARGS" | cut -d' ' -f1)
        SUPPLEMENTARY_CONTEXT=$(echo "$ARGS" | cut -d' ' -f2-)
        ```
    *   Determine the `FEATURE_NAME` based on the `IDENTIFIER`'s format:
        ```sh
        if [[ "$IDENTIFIER" == sc-* ]]; then
            FEATURE_NAME="$IDENTIFIER"
        elif [[ "$IDENTIFIER" == *notion.so* ]]; then
            SLUG_WITH_UUID=$(basename "$IDENTIFIER")
            FEATURE_NAME=${SLUG_WITH_UUID%-*}
        else
            echo "Error: Unsupported identifier format. Must be a Shortcut story ID (sc-*) or a Notion URL." >&2
            exit 1
        fi
        ```

3.  **Directory Scaffolding:**
    *   Create the feature directory structure: `ai-session create-feature "$FEATURE_NAME"`.

4.  **Content Fetching and Synthesis (Inline):**
    *   **Fetch Primary Content:** Based on the identifier, use a hypothetical MCP command-line tool to fetch the main content and store it in a variable.
        ```sh
        PRIMARY_CONTENT=""
        if [[ "$IDENTIFIER" == sc-* ]]; then
            PRIMARY_CONTENT=$(shortcut-mcp fetch "$IDENTIFIER" "$SUPPLEMENTARY_CONTEXT")
        elif [[ "$IDENTIFIER" == *notion.so* ]]; then
            PRIMARY_CONTENT=$(notion-mcp fetch "$IDENTIFIER" "$SUPPLEMENTARY_CONTEXT")
        fi
        ```
    *   **Extract and Fetch Linked Resources:** Scan the primary content for URLs. Loop through them, fetch their content using the appropriate tool, and aggregate the results into a single variable. Limit recursion to one level.
        ```sh
        LINKED_RESOURCES_CONTENT="## Linked Resources\n\n"
        LINKS=$(echo "$PRIMARY_CONTENT" | grep -oE 'https?://[a-zA-Z0-9./?=-_]+' || true)

        for LINK in $LINKS; do
            LINK_CONTENT=""
            if [[ "$LINK" == *shortcut.com* ]]; then
                LINK_CONTENT=$(shortcut-mcp fetch "$LINK")
            elif [[ "$LINK" == *notion.so* ]]; then
                LINK_CONTENT=$(notion-mcp fetch "$LINK")
            elif [[ "$LINK" == *github.com* ]]; then
                LINK_CONTENT=$(github-mcp fetch "$LINK") # Hypothetical tool
            else
                LINK_CONTENT=$(web-fetch "$LINK") # Hypothetical tool
            fi
            LINKED_RESOURCES_CONTENT+=$(printf '\n### %s\n\n%s\n' "$LINK" "$LINK_CONTENT")
        done
        ```
    *   **Synthesize and Save Description:** Combine all fetched content into a final Markdown string. Then, pipe this string into the standard input of the `ai-session description create` command. **You must not use a heredoc (`<<EOF`)**.
        ```sh
        SYNTHESIZED_CONTENT=$(printf 'Source: %s\n\n%s\n\n%s' "$IDENTIFIER" "$PRIMARY_CONTENT" "$LINKED_RESOURCES_CONTENT")
        printf '%s' "$SYNTHESIZED_CONTENT" | ai-session description create "$FEATURE_NAME"
        ```

5.  **Final Context Output:**
    *   Load all context files for the created feature: `CONTEXT_XML=$(ai-session load-context "$FEATURE_NAME")`.
    *   Extract the `description.md` content from the XML output using a stream editor like `awk` or `sed`.
        ```sh
        DESCRIPTION=$(echo "$CONTEXT_XML" | awk '/<file name="description.md">/,/<\/file>/' | sed '1d;$d')
        ```
    *   Read the project's `AGENTS.md` file, falling back to `GEMINI.md` if it doesn't exist.
        ```sh
        AGENTS_CONTENT=$(cat AGENTS.md 2>/dev/null || cat GEMINI.md 2>/dev/null)
        ```
    *   Format and print the final session context to standard output. This is the only output the user will see.
        ```sh
        printf '### ✨ Session Context Loaded for `%s`\n\n' "$FEATURE_NAME"
        printf '**Description:**\n'
        echo "$DESCRIPTION" | sed 's/^/> /'
        printf '\nThis context is now available for all subsequent commands.\n'
        ```
