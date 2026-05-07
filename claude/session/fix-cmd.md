---
description: Diagnoses and fixes the last session command that failed or behaved incorrectly.
---

You are a skill repair agent. Your job is to identify what went wrong in the last `/session:` command and fix its skill file with surgical edits.

---

## Core Principle: Verify Before Fixing

Never assume what the correct invocation is — verify it first. For every broken reference found in the skill, check the actual tool before writing the fix.

---

## Example

`/session:checkpoint` called `$AI_SESSION_HOME/scripts/append_to_log.sh "path/to/log.md" "message"` but that script no longer exists. Running `ai-session append-log --help` revealed the correct replacement: `ai-session append-log <feature-dir> "<message>"`. The skill was updated with a surgical Edit.

---

## Process

### 1. Identify the Broken Command

From the conversation history, find the last `/session:<command>` that failed or produced incorrect behaviour. Extract:
- The command name (e.g. `checkpoint`)
- The exact error or wrong behaviour observed

### 2. Read the Skill File

```bash
cat ~/.claude/commands/session/<command-name>.md
```

### 3. Diagnose All Issues

For each shell invocation in the skill file, verify it against reality:

- **`ai-session` CLI calls:** Run `ai-session <subcommand> --help` to confirm argument count, names, and format.
- **Shell scripts (`*.sh`):** Run `ls $AI_SESSION_HOME/scripts/` to check the script still exists.
- **External tools:** Run `which <tool>` or `<tool> --help` to confirm availability.
- **Env variables and paths:** Check that referenced variables (`$AI_SESSION_HOME`, etc.) expand to what the skill expects.
- **Any other pattern** that could cause the observed failure (wrong flags, hardcoded paths, outdated syntax).

Build a diagnosis list: for each issue, note what the skill currently says vs. what is correct.

### 4. Present Diagnosis

Show the user a concise list:
- **Issue:** what the skill says
- **Fix:** what it should say
- **Verified by:** the command you ran to confirm

### 5. Apply Fixes

After presenting (no need to wait for approval unless the fix is ambiguous), apply the changes using the Edit tool. Make surgical edits — do not rewrite the whole file.