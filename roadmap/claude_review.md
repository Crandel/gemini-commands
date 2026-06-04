# Session Workflow Commands

> Written after performing the full migration of these commands to Claude Code.
> This gives me a perspective the previous reviews didn't have: I had to reason about
> every command deeply enough to translate it, which surfaces friction that's invisible
> from the outside.

---

## What's genuinely strong

The core architectural instinct here is correct. Moving from chat-history-driven to
state-file-driven is the right bet — LLM context windows are unreliable over long
sessions, and structured files are debuggable in a way that conversation history never is.

The script extraction pattern (Phase 4) is the most important architectural decision in
the project. Pushing deterministic operations into shell scripts and leaving only
orchestration and synthesis to the LLM is exactly the right division of labor. The
`append_to_log.sh`, `create_feature_dir.sh`, and `load_context_files.sh` scripts are
the most valuable artifacts here, not the commands themselves.

The subagent pattern for reviews is well-reasoned. The "fresh eyes" motivation is real
and the implementation (passing only the diff + requirements, not the implementation
history) correctly operationalizes it.

---

## Structural risks worth addressing

### 1. The session context block is a single point of failure

Every command after `/session:load-context` depends on the LLM finding the
`### ✨ Session Context Loaded for...` block in conversation history. In long sessions,
this block gets pushed out of the active context window or lost to compression.

There is no recovery path. The session silently degrades — the LLM starts hallucinating
context or asking the user to re-provide information that was already established.

**Concrete suggestion:** Add a `/session:context` command that re-runs the context
loading step without resetting anything. Treat it as a "refresh" — cheap to call
anytime the user feels the session has drifted.

---

### ~~2. Scripts are coupled to a Gemini-namespaced path~~ ✅ Fixed

The repo now lives at `~/.ai-session/` and all commands reference scripts via
`$AI_SESSION_HOME/scripts/`. `setup.sh` symlinks each subdirectory of `gemini/` and
`claude/` into the respective tool's commands directory — no Gemini path dependency
remains. Both tools point at the same neutral location.

---

### 3. Script failures are silent

If `load_context_files.sh` is called on a directory that doesn't exist, it exits with
an error — but the LLM receives empty output and has no explicit signal that something
went wrong. In practice it either hallucinates context or asks the user a confusing
question.

The scripts do write to stderr, but the prompts don't instruct the model to check the
exit code or handle errors.

**Concrete suggestion:** Have the scripts emit a structured signal on failure, e.g.:
```
ERROR: Directory not found at '.vscode/sc-99999'
```
And update the command prompts to check for this prefix and stop with a clear message
to the user.

---

### ~~4. `yq` sub-agent delegation is over-engineered for simple cases~~ ✅ Fixed

The `yq-skill` (500+ lines) has been removed from `/session:checkpoint` and
`/session:end`. Both commands now inline the exact `yq` commands needed directly in
the sub-agent prompt. The `yq-skill` install step has been removed from `setup.sh`,
`README.md`, `CLAUDE.md`, `AGENTS.md`, and all spec documentation. The `tdd-skill`
no longer loads it either.

The sub-agent delegation itself is kept (for context isolation), but the skill
indirection is gone.

---

### 5. `log.md` grows without structure

The log is an append-only flat Markdown file. Each checkpoint, research entry, and
session end adds a block. After a few weeks of active use, it becomes too long for
the LLM to load usefully, and the `/session:summary` command's value degrades.

`append_to_log.sh` adds timestamps, which is good, but the structure is inconsistent
across commands (checkpoint generates a different format than `log-research`).

**Concrete suggestion:** Define a schema for log entries with a consistent header:
```markdown
## [checkpoint] 2026-03-23T14:32
## [research] 2026-03-23T15:01
## [session-end] 2026-03-23T17:45
```
This makes the log parseable — and opens the door to a future `/session:log-summary`
command that loads only entries of a specific type.

---

## Smaller but real issues

**`migration.toml` prompt is a bash script, not a prompt.** The prompt field contained
`#!/bin/bash` and raw shell code. This likely worked because Gemini was lenient about
it, but it's a latent bug. Fixed in the Claude version, but worth fixing in the
Gemini original too.

**`verify-release.toml` uses `{{.Args}}` (Go template syntax)** instead of `{{args}}`
like every other command. Inconsistency that suggests it was written separately and
never caught. Worth standardizing.

**`questions.yml` is created by `/session:plan` but answered as a side effect of
checkpoints.** There's no dedicated command to work through open questions
interactively. A `/session:questions` command that presents each open question and
prompts for an answer would make this workflow more intentional.

**No way to list active sessions.** There's no `/session:list` command. Users have to
manually browse `.vscode/`. For someone with 5+ active features this is friction.

---

## On the roadmap items already proposed

The ChatGPT roadmap (2.1–2.10) is directionally correct but very abstract. The most
actionable near-term item — cross-LLM compatibility — is now partially done. The
script path coupling issue above is what remains.

The "Session State Engine" (2.2) with formal states (`defined`, `planned`, etc.) is
genuinely valuable and not that far away. A `status` field in a `session.yml` file at
the root of the feature directory would be a small change with high payoff — it would
let `/session:list` show meaningful state at a glance.

The "Feature Knowledge Graph" (2.6) is interesting but years away from being useful.
I'd deprioritize it in favor of making the existing workflow more reliable first.

---

## Summary assessment

This is a well-designed system with a clear architectural philosophy that has been
applied consistently. The main risks are reliability (silent session context loss,
silent script failures) and portability (Gemini-namespaced paths). Both are fixable
without changing the core design.

The most impactful next investments, in order:

1. ~~Neutral script location (portability, one-time fix)~~ ✅ Done
2. Script error signaling (reliability)
3. `/session:context` refresh command (reliability)
4. ~~`yq-skill` removal from checkpoint/end (simplicity)~~ ✅ Done
5. Log entry schema (long-term usability)
