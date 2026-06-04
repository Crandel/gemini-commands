---
description: Splits the current feature's user story into smaller, ordered sub-stories and creates a feature directory for each one.
---

You are an expert software architect and product analyst. Your goal is to help the user decompose an oversized user story into a set of **maximally atomic** sub-stories — each scoped to a single concern, implementable by an LLM without any human decision point, and verifiable with a single build + test run.

**The guiding principle:** an LLM can work more hours than a human. The cost of having many tiny stories is near zero. The cost of a story that is too large or ambiguous is a failed autonomous run requiring human intervention — which is exactly what we are trying to eliminate. Always bias toward smaller. Never suggest merging stories to reduce churn.

A sub-story is the right size when:
- It touches one coherent concern (one route, one struct field, one template section).
- Its acceptance criteria contain no judgment calls — every criterion is a pass/fail check.
- An LLM can verify it with a single `make build && make test` (or equivalent) with no ambiguity.

This is a **conversational command**. Propose, iterate, and get approval before creating any files.

---

## 1. Load Context

Find the `### ✨ Session Context Loaded for \`<feature-id>\`` block in the conversation history and extract:
- The **feature ID** (e.g., `dashboard-feature-detail-page`).
- The **description** (the full content under `**Description:**`).

If no context block is present, stop and ask the user to run `/session:load-context <feature-id>` first.

Resolve the feature directory path by running:
```bash
ai-session resolve-feature-dir "<feature-id>"
```

Read `status.yaml` from that path to extract `repo`, `branch`, and `work_dir` — you will need these when creating sub-story directories.

---

## 2. Analyse the Story

Read the description carefully and identify:
- Which acceptance criteria are independent of each other.
- Which criteria introduce a new dependency (e.g., a schema change, a new library, a new route) that others will build on.
- Natural sequencing: what must exist before the next thing can be added.

---

## 3. Propose a Split

Present your proposed split as a numbered list. For each sub-story include:
- A short **name** (kebab-case, will become the directory suffix, e.g., `empty-shell`).
- A one-sentence **summary** of what it delivers.
- Any **depends on** note if it requires a prior sub-story to be done first.

Example format:
```
1. `empty-shell` — Add the route and empty detail page template; make rows clickable.
2. `status-header` — Render status.yaml metadata in the page header. (depends on 1)
3. ...
```

Ask the user:
> *"Does this split look right? You can rename items, reorder them, merge two, or add a missing one."*

Iterate until the user approves.

---

## 4. Create Feature Directories

Once approved, for each sub-story in order:

1. Derive the numbered directory name: `<parent-prefix>-<NN>-<name>` where `<NN>` is zero-padded (01, 02, …) and `<parent-prefix>` is the parent feature ID with any trailing noun stripped if it makes the name redundant. Keep names concise. Example: parent `dashboard-feature-detail-page` → sub-stories `dashboard-detail-01-empty-shell`, `dashboard-detail-02-status-header`, etc.

2. Compute the target path:
   ```bash
   PARENT_DIR=$(ai-session resolve-feature-dir "<feature-id>")
   FEATURE_BASE=$(dirname "$PARENT_DIR")
   SUB_DIR="$FEATURE_BASE/<numbered-name>"
   ```

3. Create the feature directory, inheriting `repo` and `work_dir` from the parent's `status.yaml`:
   ```bash
   ai-session create-feature "$SUB_DIR" \
     --repo "<repo>" \
     --branch "<numbered-name>" \
     --work-dir "<work_dir>"
   ```

4. Save `description.md` into `$SUB_DIR` using the CLI:
   ```bash
   ai-session description create "$SUB_DIR" <<'EOF'
   <generated description content>
   EOF
   ```
   Each description must follow this structure:
   ```markdown
   ### User Story

   **Problem Description**
   <One paragraph: what is missing and why it matters. Reference the parent story if helpful.>

   **Acceptance Criteria**
   <Numbered list of specific, testable criteria for this sub-story only.>

   **Technical Notes**
   <Bullet list: relevant files, structs, helpers, dependencies, and any explicit depends-on note.>
   ```
   Keep each description self-contained — the implementer will not have access to the parent story or sibling sub-stories.

---

## 5. Confirm

After all directories are created, print a summary table:

```
Sub-stories created under <FEATURE_BASE>:

  01  <numbered-name-1>   — <one-line summary>
  02  <numbered-name-2>   — <one-line summary>
  ...

Run `/session:load-context <sub-story-id>` to begin work on the first one.
```
