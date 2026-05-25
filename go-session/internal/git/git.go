package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RemoteURL returns the git remote origin URL, or "" if not in a git repo.
// exec.Command is intentionally kept in the CLI layer, not in internal/commands.
func RemoteURL() string {
	return RemoteURLImpl()
}

// remoteURLImpl is the actual implementation of RemoteURL, allowing it to be swapped out for testing.
var RemoteURLImpl = func() string {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// OrgRepo returns "org/repo" derived from git remote origin, or "" if unavailable.
func OrgRepo() string {
	remoteURL := RemoteURL()
	if remoteURL == "" {
		return ""
	}
	return ParseOrgRepo(remoteURL)
}

// WorkDir returns the repo root path from git rev-parse --show-toplevel, or "" if unavailable.
func WorkDir() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CurrentBranch returns the current git branch name, or "" if unavailable.
func CurrentBranch() string {
	return CurrentBranchImpl()
}

// currentBranchImpl is the actual implementation of CurrentBranch, allowing it to be swapped out for testing.
var CurrentBranchImpl = func() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" { // detached HEAD state
		return ""
	}
	return branch
}

// DefaultBranch returns the remote default branch name (e.g. "main") by
// resolving refs/remotes/origin/HEAD locally without a network call.
// Falls back to "main" on any error.
func DefaultBranch() string {
	return DefaultBranchImpl()
}

// defaultBranchImpl is the actual implementation of DefaultBranch, allowing it to be swapped out for testing.
var DefaultBranchImpl = func() string {
	out, err := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD", "--short").Output()
	if err != nil {
		return "main"
	}
	ref := strings.TrimSpace(string(out))
	// ref is "origin/main" — strip the "origin/" prefix.
	if idx := strings.Index(ref, "/"); idx != -1 {
		return ref[idx+1:]
	}
	return ref
}

// untrackedExcludes lists pathspec exclusions passed to `git ls-files --others`.
// These cover common dependency directories for Go, JS/TS, and similar ecosystems.
// They complement .gitignore (via --exclude-standard) for directories that may not
// be ignored in every project.
var untrackedExcludes = []string{
	":(exclude)vendor",       // Go modules vendor dir
	":(exclude)node_modules", // JS/TS dependencies
}

// appendUntracked returns a unified diff for every untracked file (files not yet
// added to the index) found under workDir. Common dependency directories are
// excluded via pathspecs. Errors are non-fatal — a non-repo directory or missing
// files produce empty output.
func appendUntracked(workDir string) string {
	args := append([]string{"ls-files", "--others", "--exclude-standard"}, untrackedExcludes...)
	lsCmd := exec.Command("git", args...)
	lsCmd.Dir = workDir
	lsOut, _ := lsCmd.Output()

	var b strings.Builder
	for _, f := range strings.Split(strings.TrimSpace(string(lsOut)), "\n") {
		if f == "" {
			continue
		}
		// Use the absolute path so the diff is correct regardless of the process cwd.
		// git diff --no-index exits 1 when files differ (always for new files) — ignore exit code.
		noIndexCmd := exec.Command("git", "diff", "--no-index", "--", "/dev/null", filepath.Join(workDir, f))
		out, _ := noIndexCmd.Output()
		b.Write(out)
	}
	return b.String()
}

// Diff returns a unified diff of all changes on the current branch relative to
// origin/<default-branch>. workDir must be the repo root; passing a subdirectory
// scopes the diff to that subtree only.
//
// The diff covers three categories:
//   - committed changes not yet on the default branch
//   - staged and unstaged changes to tracked files
//   - untracked files (new files never added to the index)
//
// vendor/ and node_modules/ are excluded. git fetch origin runs first to
// ensure the remote ref is current; fetch failure is non-fatal.
func Diff(workDir string) (string, error) {
	branch := DefaultBranch()

	fetchCmd := exec.Command("git", "fetch", "origin")
	fetchCmd.Dir = workDir
	_ = fetchCmd.Run() // non-fatal

	diffCmd := exec.Command("git", "diff", "origin/"+branch, "--",
		".", ":(exclude)vendor", ":(exclude)node_modules")
	diffCmd.Dir = workDir
	tracked, err := diffCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff origin/%s: %w", branch, err)
	}
	return string(tracked) + appendUntracked(workDir), nil
}

// DiffLastCommit returns a unified diff of all uncommitted changes relative to
// HEAD (the last commit on the current branch). workDir must be the repo root.
//
// The diff covers:
//   - staged and unstaged changes to tracked files (git diff HEAD)
//   - untracked files (new files never added to the index)
//
// vendor/ and node_modules/ are excluded.
func DiffLastCommit(workDir string) (string, error) {
	diffCmd := exec.Command("git", "diff", "HEAD", "--",
		".", ":(exclude)vendor", ":(exclude)node_modules")
	diffCmd.Dir = workDir
	tracked, err := diffCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff HEAD: %w", err)
	}
	return string(tracked) + appendUntracked(workDir), nil
}

// ParseOrgRepo extracts "org/repo" from SSH and HTTPS git remote URLs.
// SSH:   git@github.com:org/repo.git  → org/repo
// HTTPS: https://github.com/org/repo.git → org/repo
func ParseOrgRepo(remoteURL string) string {
	url := strings.TrimSuffix(remoteURL, ".git")
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return ""
	}
	idx := strings.Index(url, "://")
	if idx == -1 {
		return ""
	}
	rest := url[idx+3:]
	slash := strings.Index(rest, "/")
	if slash == -1 {
		return ""
	}
	return rest[slash+1:]
}
