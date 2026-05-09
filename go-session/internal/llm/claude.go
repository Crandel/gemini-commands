package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// claudeRunner implements Runner using `claude --dangerously-skip-permissions`.
type claudeRunner struct {
	opts RunnerOptions
}

func (r *claudeRunner) Run(stdin io.Reader, stdout, stderr io.Writer) error {
	ctx, cancel := contextWithOptionalTimeout(r.opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", "--dangerously-skip-permissions", "--permission-mode", "bypassPermissions", `--mcp-config`, `{"mcpServers":{}}`, "--no-session-persistence", "--allowed-tools", "Bash,Read,Glob,Grep,Write", "--model", "haiku")
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if r.opts.WorkDir != "" {
		cmd.Dir = r.opts.WorkDir
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("claude pipeline timed out after %s — the codebase may be in a partial state; resume from the last completed change", r.opts.Timeout)
		}
		return fmt.Errorf("claude pipeline failed: %w", err)
	}
	return nil
}
