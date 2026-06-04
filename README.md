> [!IMPORTANT]
> ## BREAKING CHANGE: Repo Location & Multi-Tool Support
>
> Previously, this repo was cloned directly into `~/.gemini/commands/` and only supported Gemini CLI.
>
> It now lives in its own neutral location (`~/.ai-session/`) and supports **both Gemini CLI and Claude Code**.
> A `setup.sh` script handles all wiring automatically.
>
> **If you cloned this into `~/.gemini/commands/` before, migrate with:**
> ```sh
> mv ~/.gemini/commands ~/.ai-session
> ~/.ai-session/setup.sh
> source ~/.zshrc
> ```

# ai-session

An interactive and autonomous AI-powered software engineering system. Instead of relying on fragile chat history or basic prompts, `ai-session` provides a structured, state-driven workflow. It empowers AI assistants to autonomously implement codebases, run test suites, verify changes, address feedback, and create PRs using persistent state files (features, plans, open questions, logs) that remain consistent across tools and sessions.

Compatible with **Gemini CLI**, **Claude Code**, or both simultaneously. Each tool gets its own set of commands (`gemini/` and `claude/`) that implement the same workflow concepts in that tool's native format, orchestrating deterministic file operations and automated loop-execution via a dedicated Go-based CLI.

## Prerequisites

> **Platform note:** these instructions are written for macOS. `setup.sh` works on Linux too, but the install commands below use Homebrew — substitute your distro's package manager (`apt`, `dnf`, etc.) as needed.

Before running setup, make sure you have:

- **[Go](https://go.dev/doc/install)** (v1.21+)
- **[Gemini CLI](https://github.com/google-gemini/gemini-cli)** and/or **[Claude Code](https://github.com/anthropics/claude-code)** — install whichever tools you plan to use
- **[yq](https://github.com/mikefarah/yq) v4+** — used to update YAML state files (`brew install yq`)
- **[Node.js](https://nodejs.org/)** — required for `npx` (used by MCP servers)
- **[uv](https://docs.astral.sh/uv/)** — required for `uvx` (used by the Git MCP server)
- **git**

## Configuration & Environment Variables

`ai-session` can be configured using the following environment variables. Ensure they are exported in your shell profiles (`~/.zshrc` and `~/.zshenv`) if you wish to override defaults:

| Environment Variable | Description | Default |
| --- | --- | --- |
| `AI_SESSION_HOME` | The home directory where this repository is installed. Set automatically by `setup.sh`. | `$HOME/.ai-session` |
| `AI_SESSION_FEATURES_DIR` | The central directory where all your feature folders, execution plans, logs, and registered repository configurations are saved. | `$HOME/.features` |

To use a custom directory for your features, set `AI_SESSION_FEATURES_DIR` before running `setup.sh`:
```bash
export AI_SESSION_FEATURES_DIR="$HOME/my-custom-features-path"
```

## Setup

1. **Clone the repository:**
```bash
git clone git@github.com:daniel-talonone/gemini-commands.git ~/.ai-session
```

2. **Run the setup script:**
```bash
chmod +x ~/.ai-session/setup.sh
~/.ai-session/setup.sh
```

The idempotent `setup.sh` script does the following automatically:
- Registers `AI_SESSION_HOME` in both `~/.zshrc` and `~/.zshenv` (supporting both interactive and headless CLI sessions).
- Adds necessary tool directories (`scripts/` and Go binary `go-session/bin/`) to your shell `PATH`.
- Symlinks each subcommand group in `gemini/` and `claude/` to their respective tool's active commands directory.
- Creates the centralized features directory (`AI_SESSION_FEATURES_DIR` or the default `$HOME/.features`).
- Automatically compiles the Go CLI (`go-session/bin/ai-session`) so it's ready to use instantly.

3. **Reload your shell profile:**
```bash
source ~/.zshenv && source ~/.zshrc
```

> **Not using zsh?** Manually add `export AI_SESSION_HOME="$HOME/.ai-session"` and add `$AI_SESSION_HOME/go-session/bin` & `$AI_SESSION_HOME/scripts` to your shell's config file (`.bashrc`, `.bash_profile`, `config.fish`, etc.).

## Getting Started

Get up and running with `ai-session` in three quick steps:

### 1. Register Your Repository

For `ai-session` to run background verifications (build, test, lint), compile code, and load workspace context, you must register each target project repository with the central config registry. Add the repository definition:

```bash
ai-session repository add --config-json '{
  "repo_name": "my-org/my-project",
  "work_dir": "/Users/username/src/my-project",
  "is_worktree": false,
  "agents_path": "/Users/username/src/my-project/AGENTS.md",
  "verify_config": {
    "build": "yarn build",
    "test": "yarn test:unit",
    "lint": "yarn lint --fix"
  }
}'
```

To list all configured repositories:
```bash
ai-session repository list
```

### 2. Launch the Web Dashboard

Run the built-in HTTP server to start the real-time visual web dashboard:

```bash
ai-session serve
```
Open `http://localhost:1004` to monitor active feature development, view structured execution plans, append logs, and switch between multiple types of AI-driven code reviews (Regular, Docs, DevOps).

### 3. Initiate a Feature Session

Inside your registered repository's directory, boot your AI assistant (Gemini CLI or Claude Code) and run:
```
/session:new sc-12345
```
This automatically scaffolds a dedicated directory in your features folder (`~/.features/my-org/my-project/sc-12345/`), initializes execution state files (`plan.yml`, `status.yaml`), and loads the context block to begin planning, implementation, and review.

## Structure

```
.
├── spec/                  # LLM-agnostic: documentation, schemas, examples
│   └── session/
├── go-session/            # Go CLI binary (ai-session)
│   ├── cmd/ai-session/    # Cobra subcommands
│   └── internal/
│       ├── commands/      # File operations (plan, log, dir, load-context, …)
│       ├── dashboard/     # Feature scanner + state derivation (for serve)
│       ├── git/           # Git helpers (remote URL, diff, untracked files)
│       ├── pr/            # pr.md create, read, and write
│       ├── review/        # review*.yml CRUD and open-findings access
│       └── server/        # HTTP server + HTML template (for serve)
├── gemini/                # Gemini CLI implementation (*.toml)
│   └── session/
├── claude/                # Claude Code implementation (*.md)
│   └── session/
├── headless/
│   └── session/           # LLM-agnostic headless pipeline variants (generated via gen_headless.sh)
├── scripts/               # Shared helper scripts used by both tools
├── roadmap/               # Project roadmap and design docs
├── AGENTS.md              # Project-wide AI context
├── setup.sh               # Idempotent setup script
└── README.md
```

## How it works

`setup.sh` symlinks each subdirectory of `gemini/` and `claude/` into the respective
tool's commands directory:

- `~/.gemini/commands/<group>/` → `~/.ai-session/gemini/<group>/`
- `~/.claude/commands/<group>/` → `~/.ai-session/claude/<group>/`

Each tool's `commands/` directory remains a real folder, so you can add personal
commands alongside the repo-managed ones without touching this repo.

Adding a new command group (e.g. `gemini/transaction/`) is automatically picked up
the next time `setup.sh` is run — no script changes needed.

All commands reference shared scripts via `$AI_SESSION_HOME/scripts/`.

## Dashboard

`ai-session serve` starts a local read-only web dashboard that shows all features across all repos:

```bash
ai-session serve           # http://localhost:1004
ai-session serve --port 8080
```

Filter by repo (`?repo=org/name`) or status (`?status=running|idle|done`). The page scans `~/.features/` on every refresh — no caching, no background process.

### Detail View

The detail view for a feature provides a comprehensive overview of its state. It now supports the discovery of multiple review files (e.g., `review.yml`, `review-docs.yml`). If more than one review file is found, a dropdown selector is displayed, allowing you to switch between the different sets of review findings. This is useful for separating different types of reviews, such as documentation, DevOps, and regular code reviews.

## Go CLI

The `ai-session` binary provides deterministic file operations used by prompts and orchestrators:

```bash
ai-session serve                    # dashboard
ai-session load-context sc-1234     # load feature files as XML blocks for LLM context
ai-session create-feature sc-1234   # scaffold feature directory (includes status.yaml)
ai-session resolve-feature-dir sc-1234
ai-session append-log sc-1234 "msg"

ai-session update-task sc-1234 task-id --status done
ai-session update-slice sc-1234 slice-id --status in-progress
ai-session plan-list sc-1234
ai-session plan-get sc-1234 --slice s --task t
ai-session plan-write sc-1234       # validate + atomically write plan.yml (stdin)
ai-session plan-write-architecture sc-1234  # atomically write architecture.md (stdin)
ai-session plan-enrich-task sc-1234 --slice s --task t
ai-session plan-split-task sc-1234 --slice s --task t
ai-session implement sc-1234        # headless LLM implementation loop (gemini --yolo per task)
ai-session review sc-1234 [--regular] [--docs] [--devops] [--strategy=branch|last-commit]
ai-session review-write sc-1234 --type regular   # write findings from stdin (YAML)
ai-session review-update sc-1234 --json '[...]' [--regular] [--docs] [--devops]  # atomically update finding statuses
ai-session address-feedback sc-1234 [--regular] [--docs] [--devops] [--remote] # Address findings with retry/verification
ai-session create-pr-description <feature-name> # Generates a PR description from feature context and writes it to `pr.md`.
ai-session submit-pr <feature-name>             # Creates a GitHub PR for the feature's branch. The PR title is `feat: <branch-name>`. The PR body is read from `pr.md`. The base branch is detected automatically. If a PR already exists for the branch, the command exits with an error. On success, the PR URL is written to `status.yaml` and the `pipeline_step` is set to `pr-submitted`.
```

Build the binary: `cd go-session && make build` (output: `go-session/bin/ai-session`, added to `$PATH` by `setup.sh`).

## Session workflow

See [`spec/session/README.md`](spec/session/README.md) for full documentation of the
session commands, workflow lifecycle, and core concepts.
