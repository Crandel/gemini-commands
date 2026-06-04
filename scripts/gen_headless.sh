#!/bin/zsh
# gen_headless.sh — Generate claude/session/headless/*.md from claude/session/*.md
#
# Run this whenever a Claude command is added or modified.
# The claude/session/headless/ directory is fully generated — do not edit .md files directly.
# Exception: plan.md is hand-written and excluded via the deny list.
#
# Usage: ./scripts/gen_headless.sh [--force]

set -euo pipefail

FORCE=0
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=1 ;;
    esac
done

REPO_DIR="$(cd "${0:A:h}/.." && pwd)"
CLAUDE_DIR="$REPO_DIR/claude/session"
HEADLESS_DIR="$REPO_DIR/headless/session"
ADAPTER_PROMPT_FILE="$REPO_DIR/scripts/headless_adapter_prompt.md"
CHECKSUMS_FILE="$HEADLESS_DIR/.checksums"

# Commands that have no headless equivalent — skip entirely.
# Hand-written headless prompts (not generated): plan, implement, review, review-docs, review-devops,
# address-remote-feedback, address-feedback-remote, create-pr-description.
# Inherently interactive commands (no headless variant): define, start, end, get-familiar, log-research,
# migration, checkpoint.
DENY_LIST=("address-local-feedback" "audit" "define" "load-context" "end" "get-familiar" "log-research" "migration" "plan" "checkpoint" "implement" "review" "review-docs" "review-devops" "address-remote-feedback" "address-feedback-remote" "create-pr-description" "first-test" "fix-cmd" "new-skill" "split-story")

if ! command -v gemini &>/dev/null; then
    echo "Error: 'gemini' CLI not found in PATH. Install Gemini CLI to use this script." >&2
    exit 1
fi

if [ ! -f "$ADAPTER_PROMPT_FILE" ]; then
    echo "Error: Adapter prompt not found at $ADAPTER_PROMPT_FILE" >&2
    exit 1
fi

mkdir -p "$HEADLESS_DIR"

# Load existing checksums into an associative array
declare -A stored_checksums
if [ -f "$CHECKSUMS_FILE" ]; then
    while IFS=' ' read -r hash filename; do
        stored_checksums["$filename"]="$hash"
    done < "$CHECKSUMS_FILE"
fi

adapter_prompt="$(cat "$ADAPTER_PROMPT_FILE")"

count=0
skipped=0
denied=0
for md_file in "$CLAUDE_DIR"/*.md; do
    [ -f "$md_file" ] || continue

    name="$(basename "$md_file" .md)"
    out_file="$HEADLESS_DIR/$name.md"

    # Check deny list
    denied_flag=0
    for denied_name in "${DENY_LIST[@]}"; do
        if [ "$name" = "$denied_name" ]; then
            denied_flag=1
            break
        fi
    done
    if [ "$denied_flag" -eq 1 ]; then
        denied=$((denied + 1))
        continue
    fi

    current_hash="$(shasum -a 256 "$md_file" | awk '{print $1}')"
    if [ "$FORCE" -eq 0 ] && [ "${stored_checksums[$name]+set}" = "set" ] && [ "${stored_checksums[$name]}" = "$current_hash" ] && [ -f "$out_file" ]; then
        skipped=$((skipped + 1))
        continue
    fi

    # Extract body — everything after the closing --- of the frontmatter
    body="$(awk 'BEGIN{f=0} /^---/{f++; next} f>=2{print}' "$md_file")"

    # Adapt the prompt body via LLM: translate tool names + strip interactivity + inline sub-agents.
    # gemini -p sets the instruction prompt; stdin is appended to it as content.
    # stderr is suppressed to hide Gemini CLI startup noise.
    printf "  Adapting %s.md..." "$name"
    adapted_body="$(printf '%s' "$body" | gemini -p "$adapter_prompt" 2>/dev/null)"
    printf " ✓\n"

    {
        printf '# Generated from claude/session/%s.md — do not edit directly.\n' "$name"
        printf '# Run scripts/gen_headless.sh to regenerate.\n'
        printf '\n'
        printf '%s\n' "$adapted_body"
    } > "$out_file"

    stored_checksums["$name"]="$current_hash"
    count=$((count + 1))
done

# Persist updated checksums
{
    for name in "${(k)stored_checksums[@]}"; do
        printf '%s %s\n' "${stored_checksums[$name]}" "$name"
    done
} | sort > "$CHECKSUMS_FILE"

echo ""
if [ "$skipped" -gt 0 ]; then
    echo "Generated $count headless command(s), skipped $skipped unchanged, denied $denied."
else
    echo "Generated $count headless command(s), denied $denied."
fi
