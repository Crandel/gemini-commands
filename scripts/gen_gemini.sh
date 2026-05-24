#!/usr/bin/env bash
# gen_gemini.sh — Generate gemini/session/*.toml from claude/session/*.md
#
# Run this whenever a Claude command is added or modified.
# The gemini/session/ directory is fully generated — do not edit .toml files directly.
#
# Each prompt body is adapted from Claude to Gemini conventions by passing it
# through `gemini -p` with scripts/gemini_adapter_prompt.md as the instruction.
#
# Usage: ./scripts/gen_gemini.sh

set -euo pipefail

FORCE=0
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=1 ;;
    esac
done

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLAUDE_DIR="$REPO_DIR/claude/session"
GEMINI_DIR="$REPO_DIR/gemini/session"
ADAPTER_PROMPT_FILE="$REPO_DIR/scripts/gemini_adapter_prompt.md"
CHECKSUMS_FILE="$GEMINI_DIR/.checksums"

if ! command -v gemini &>/dev/null; then
    echo "Error: 'gemini' CLI not found in PATH. Install Gemini CLI to use this script." >&2
    exit 1
fi

if [ ! -f "$ADAPTER_PROMPT_FILE" ]; then
    echo "Error: Adapter prompt not found at $ADAPTER_PROMPT_FILE" >&2
    exit 1
fi

mkdir -p "$GEMINI_DIR"

# Use a temp file instead of associative array (bash 3.2 compat — macOS default)
UPDATED_CHECKSUMS="$(mktemp)"
trap 'rm -f "$UPDATED_CHECKSUMS"' EXIT
if [ -f "$CHECKSUMS_FILE" ]; then
    cp "$CHECKSUMS_FILE" "$UPDATED_CHECKSUMS"
else
    touch "$UPDATED_CHECKSUMS"
fi

# Return stored checksum for a given name, or empty string
get_stored_checksum() {
    grep " $1$" "$UPDATED_CHECKSUMS" 2>/dev/null | awk '{print $1}' || true
}

adapter_prompt="$(cat "$ADAPTER_PROMPT_FILE")"

echo "DEBUG: CLAUDE_DIR=$CLAUDE_DIR"
echo "DEBUG: CHECKSUMS_FILE=$CHECKSUMS_FILE (exists=$([ -f "$CHECKSUMS_FILE" ] && echo yes || echo no))"
echo "DEBUG: UPDATED_CHECKSUMS=$UPDATED_CHECKSUMS"
echo "DEBUG: FORCE=$FORCE"
echo "DEBUG: md files: $(ls "$CLAUDE_DIR"/*.md 2>/dev/null | wc -l | tr -d ' ') found"

count=0
skipped=0
for md_file in "$CLAUDE_DIR"/*.md; do
    [ -f "$md_file" ] || { echo "DEBUG: no .md files matched glob"; continue; }

    name="$(basename "$md_file" .md)"
    toml_file="$GEMINI_DIR/$name.toml"

    current_hash="$(shasum -a 256 "$md_file" | awk '{print $1}')"
    stored_hash="$(get_stored_checksum "$name")"
    echo "DEBUG: $name — stored='$stored_hash' current='$current_hash' toml=$([ -f "$toml_file" ] && echo exists || echo missing)"
    if [ "$FORCE" -eq 0 ] && [ -n "$stored_hash" ] && [ "$stored_hash" = "$current_hash" ] && [ -f "$toml_file" ]; then
        skipped=$((skipped + 1))
        continue
    fi

    # Extract description value from YAML frontmatter
    description="$(awk '
        BEGIN { f=0 }
        /^---/ { f++; next }
        f==1 && /^description:/ { sub(/^description: */, ""); print; exit }
    ' "$md_file")"

    echo "DEBUG: $name — description='$description'"
    if [ -z "$description" ]; then
        echo "  ⚠ Skipping $name.md — no description found in frontmatter"
        continue
    fi

    # Extract body — everything after the closing --- of the frontmatter
    body="$(awk 'BEGIN{f=0} /^---/{f++; next} f>=2{print}' "$md_file")"

    # Adapt the prompt body from Claude conventions to Gemini conventions via LLM.
    # gemini -p sets the instruction prompt; stdin is appended to it as content.
    # stderr is suppressed to hide Gemini CLI startup noise (MCP loading, keychain, etc.)
    printf "  Adapting %s.md..." "$name"
    adapted_body="$(printf '%s' "$body" | gemini -p "$adapter_prompt" 2>/dev/null)"
    gemini_exit=$?
    if [ $gemini_exit -ne 0 ] || [ -z "$adapted_body" ]; then
        printf " ✗ (gemini exited %d or returned empty — skipping %s)\n" "$gemini_exit" "$name" >&2
        continue
    fi
    printf " ✓\n"

    # Escape backslashes in the prompt body to prevent TOML parsing errors.
    adapted_body="$(printf '%s' "$adapted_body" | sed 's/\\/\\\\/g')"

    # Escape description for a TOML double-quoted string (backslash, then double-quote)
    description_escaped="$(printf '%s' "$description" | sed 's/\\/\\\\/g; s/"/\\"/g')"

    # Write TOML — triple-quoted strings are used for the prompt.
    # Leading newline after """ is trimmed by the TOML spec.
    {
        printf '# Generated from claude/session/%s.md — do not edit directly.\n' "$name"
        printf '# Run scripts/gen_gemini.sh to regenerate.\n'
        printf 'description = "%s"\n' "$description_escaped"
        printf 'prompt = """\n'
        printf '%s\n' "$adapted_body"
        printf '"""\n'
    } > "$toml_file"

    # Validate the generated TOML file.
    if ! yq . "$toml_file" >/dev/null 2>&1; then
        printf " ✗ Invalid TOML generated for %s.md. Check for syntax errors.\n" "$name" >&2
        rm "$toml_file" # Clean up the invalid file
        continue
    fi

    # Update checksum in temp file: remove old entry (grep -v exits 1 on empty file — suppress)
    { grep -v " $name$" "$UPDATED_CHECKSUMS" || true; } > "${UPDATED_CHECKSUMS}.tmp"
    mv "${UPDATED_CHECKSUMS}.tmp" "$UPDATED_CHECKSUMS"
    printf '%s %s\n' "$current_hash" "$name" >> "$UPDATED_CHECKSUMS"
    count=$((count + 1))
done

# Persist updated checksums
sort "$UPDATED_CHECKSUMS" > "$CHECKSUMS_FILE"

echo ""
if [ "$skipped" -gt 0 ]; then
    echo "Generated $count Gemini command(s), skipped $skipped unchanged."
else
    echo "Generated $count Gemini command(s) in gemini/session/."
fi
