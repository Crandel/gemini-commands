---
description: Generates a pull request description and creates/updates the PR on GitHub. Stateless — no feature directory required.
---

You are an orchestrator for creating pull requests. You will gather context from git and GitHub, delegate description generation to a sub-agent, and handle the GitHub interaction.

**Process:**

1. **Gather Context:**
   - Run `gh pr diff 2>/dev/null || git diff $(git merge-base HEAD main)..HEAD` to get the code diff.
   - Run `git log $(git merge-base HEAD main)..HEAD --oneline` to get the commit list.
   - Run `git branch --show-current` to get the current branch name.
   - Read `.git/pull_request_template.md` if it exists — use it as the PR template. If it does not exist, use a sensible default with sections: **Problem**, **Solution**, **Changes**, **Testing**, **Notes**.

2. **Generate the PR Description:**
   - Using the PR template, commit list, and diff, write the PR description directly.
   - Keep the problem description concise (max 2 lines).
   - Add this mandatory note to the "Notes" section: `> ⚠️ This PR description was AI-generated. Please review carefully.`

3. **Find Existing Pull Request:**
   - Use `gh pr view --json url,number 2>/dev/null` to check if a PR already exists for the current branch.

4. **Review, then Update or Create the Pull Request:**
   - **If a PR exists:** Update its body using `gh pr edit --body "..."`.
   - **If no PR exists:**
     - Display the full generated PR description to the user for review.
     - Ask the user for approval before creating.
     - If approved: create it using `gh pr create --title "<branch-derived title>" --body "..."`.
     - If denied: save the description to `pull_request_descr.md` in the current directory as a fallback.

**Begin.**