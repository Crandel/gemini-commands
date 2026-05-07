#!/usr/bin/env bash
# audit_skills.sh — Audit skill files for stale CLI references, raw yq writes, gap comments,
# and direct feature file reads that should go through the ai-session CLI.
#
# Scans: claude/session/*.md and headless/session/*.md
#
# Checks:
#   1. Every `ai-session <subcommand>` invocation — verifies the subcommand still exists in the CLI.
#   2. Gap comments ("# No CLI command yet") — surfaces stale ones for upgrade.
#   3. `yq` write-mode usage (`yq -i`, `yq e ... -i`) — should use ai-session CLI instead.
#   4. Direct feature file references (review.yml, plan.yml, etc.) — potential CLI bypasses.
#
# Usage:
#   ./scripts/audit_skills.sh           # run + interpret with claude
#   ./scripts/audit_skills.sh --raw     # mechanical report only (no LLM)
#   ./scripts/audit_skills.sh --verbose # also report OK invocations, then interpret

set -euo pipefail

RAW=0
VERBOSE=0
for arg in "$@"; do
    case "$arg" in
        --raw)     RAW=1 ;;
        --verbose) VERBOSE=1 ;;
    esac
done

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLAUDE_DIR="$REPO_DIR/claude/session"
HEADLESS_DIR="$REPO_DIR/headless/session"
SCAN_DIRS=("$CLAUDE_DIR" "$HEADLESS_DIR")
AI_SESSION="${AI_SESSION_HOME}/go-session/bin/ai-session"

if [ ! -x "$AI_SESSION" ]; then
    echo "Error: ai-session binary not found at $AI_SESSION" >&2
    exit 1
fi

if [ "$RAW" -eq 0 ] && ! command -v claude &>/dev/null; then
    echo "Warning: 'claude' CLI not found — falling back to raw output." >&2
    RAW=1
fi

# Collect available subcommands (one per line)
available="$("$AI_SESSION" --help 2>&1 | awk '/^  [a-z]/{print $1}')"

subcommand_exists() {
    echo "$available" | grep -qx "$1"
}

# Relative path from REPO_DIR for consistent labels across all sections
label_for() {
    echo "${1#"$REPO_DIR/"}"
}

report=""
stale_count=0
gap_count=0
yq_write_count=0
direct_count=0

# --- SUBCOMMAND AUDIT ---
report+=$'=== SUBCOMMAND AUDIT ===\n\n'

for scan_dir in "${SCAN_DIRS[@]}"; do
    [ -d "$scan_dir" ] || continue
    for md_file in "$scan_dir"/*.md; do
        [ -f "$md_file" ] || continue
        name="$(label_for "$md_file")"

        while IFS= read -r match; do
            lineno="${match%%:*}"
            line="${match#*:}"
            subcommand="$(echo "$line" | sed -n 's/.*ai-session[[:space:]]\{1,\}\([a-z][a-z-]*\).*/\1/p')"
            [ -z "$subcommand" ] && continue

            if ! subcommand_exists "$subcommand"; then
                report+="[STALE] $name:$lineno — unknown subcommand '$subcommand'\n"
                report+="        $(echo "$line" | sed 's/^[[:space:]]*//')\n"
                stale_count=$((stale_count + 1))
            elif [ "$VERBOSE" -eq 1 ]; then
                report+="[OK]    $name:$lineno — ai-session $subcommand\n"
            fi
        done < <(grep -n 'ai-session ' "$md_file" 2>/dev/null || true)
    done
done

[ "$stale_count" -eq 0 ] && report+=$'(no stale references found)\n'

# --- GAP COMMENTS ---
report+=$'\n=== GAP COMMENTS ===\n\n'

for scan_dir in "${SCAN_DIRS[@]}"; do
    [ -d "$scan_dir" ] || continue
    for md_file in "$scan_dir"/*.md; do
        [ -f "$md_file" ] || continue
        name="$(label_for "$md_file")"

        while IFS= read -r match; do
            lineno="${match%%:*}"
            line="${match#*:}"
            report+="[GAP] $name:$lineno\n"
            report+="      $(echo "$line" | sed 's/^[[:space:]]*//')\n"
            gap_count=$((gap_count + 1))
        done < <(grep -n '# No CLI command yet' "$md_file" 2>/dev/null || true)
    done
done

[ "$gap_count" -eq 0 ] && report+=$'(none found)\n'

# --- DIRECT YQ WRITES ---
report+=$'\n=== DIRECT YQ WRITES ===\n\n'
report+=$'Lines using `yq` in write mode — should use ai-session CLI instead.\n\n'

for scan_dir in "${SCAN_DIRS[@]}"; do
    [ -d "$scan_dir" ] || continue
    for md_file in "$scan_dir"/*.md; do
        [ -f "$md_file" ] || continue
        name="$(label_for "$md_file")"

        while IFS= read -r match; do
            lineno="${match%%:*}"
            line="${match#*:}"
            report+="[YQ-WRITE] $name:$lineno\n"
            report+="           $(echo "$line" | sed 's/^[[:space:]]*//')\n"
            yq_write_count=$((yq_write_count + 1))
        done < <(grep -nE 'yq( e)? .* -i\b|yq -i\b' "$md_file" 2>/dev/null || true)
    done
done

[ "$yq_write_count" -eq 0 ] && report+=$'(none found)\n'

# --- DIRECT FEATURE FILE READS ---
report+=$'\n=== DIRECT FEATURE FILE READS ===\n\n'
report+=$'Lines referencing feature files (review.yml, plan.yml, etc.) directly — may bypass the CLI.\n\n'

for scan_dir in "${SCAN_DIRS[@]}"; do
    [ -d "$scan_dir" ] || continue
    for md_file in "$scan_dir"/*.md; do
        [ -f "$md_file" ] || continue
        name="$(label_for "$md_file")"

        while IFS= read -r match; do
            lineno="${match%%:*}"
            line="${match#*:}"
            report+="[DIRECT-READ] $name:$lineno\n"
            report+="              $(echo "$line" | sed 's/^[[:space:]]*//')\n"
            direct_count=$((direct_count + 1))
        done < <(grep -n '\b\(review\|plan\|questions\|log\|description\|pr\)\.yml\b\|\blog\.md\b\|\bdescription\.md\b\|\bpr\.md\b' "$md_file" 2>/dev/null || true)
    done
done

[ "$direct_count" -eq 0 ] && report+=$'(none found)\n'

# --- SUMMARY ---
report+=$'\n=== SUMMARY ===\n'
report+="Stale subcommand references : $stale_count\n"
report+="Gap comments                : $gap_count\n"
report+="Direct yq writes            : $yq_write_count\n"
report+="Direct feature file reads   : $direct_count\n"
report+=$'\nAvailable subcommands:\n'
report+="$(echo "$available" | sed 's/^/  /')\n"

if [ "$RAW" -eq 1 ]; then
    printf '%b' "$report"
    exit 0
fi

# Pipe report through claude for interpretation and fix suggestions
INTERPRETATION_PROMPT="You are a skill auditor for the ai-session ecosystem.

## Context

ai-session is a session-based workflow framework for AI assistants. Its /session:* skill files (Markdown prompts) allow the author to interact with persistent feature state across sessions, LLMs, and environments — interactive or headless. The key architectural principle is separation of concerns:

- The \`ai-session\` CLI handles all deterministic, structured file operations (reads, writes, status updates). This keeps those operations reliable, testable, and LLM-agnostic.
- The skill files handle creative, reasoning-heavy tasks (planning, reviewing, interpreting). They delegate all data mutations to the CLI — never touching files directly.
- Gap comments (\`# No CLI command yet\`) mark places where a skill currently accesses files directly because no CLI command exists yet. As the CLI grows, these gaps close and the skill can be upgraded.

This separation is intentional: it decouples the storage layer from the prompts, so the author can evolve either side independently and trust that the deterministic parts behave consistently.

## Your Job

The report lists:
- [STALE] entries: invocations of \`ai-session <subcommand>\` where the subcommand no longer exists in the CLI.
- [GAP] entries: comments marking operations that had no CLI command when the skill was written.
- [YQ-WRITE] entries: lines using \`yq\` in write mode — direct file mutations that should use the CLI instead.
- [DIRECT-READ] entries: lines referencing feature files (review.yml, plan.yml, etc.) directly — potential bypasses of the CLI.
- Available subcommands: the current list of valid subcommands.

1. For each [STALE] entry: check the available subcommands list and propose the most likely replacement (rename vs. removal). Be concrete — show the exact invocation fix.
2. For each [GAP] entry: check whether any available subcommand now covers the described operation. Consider semantic equivalence, not just name matching. If yes, mark as upgradeable and show the replacement. If no, mark as still pending.
3. For each [YQ-WRITE] entry: identify which ai-session subcommand should replace it and show the exact fix.
4. For each [DIRECT-READ] entry: judge whether the reference is an actual direct file read that bypasses the CLI, or benign (e.g. a comment, a CLI argument placeholder like \`{{feature-dir}}/review.yml\`). If it's a real bypass, flag it and suggest the correct CLI replacement (\`ai-session load-context <feature-id>\`).
5. Output a prioritized fix list grouped as:
   - Stale references (must fix — broken)
   - Direct yq writes (must fix — violates architectural principle)
   - Direct file reads bypassing CLI (should fix — violates architectural principle)
   - Upgradeable gaps (optional improvement)
   - Pending gaps (no action needed)
6. For each fix, show the exact change: old line → new line.

Be concise and direct. No preamble."

printf '%b' "$report" | claude -p --system-prompt "$INTERPRETATION_PROMPT" 2>/dev/null
