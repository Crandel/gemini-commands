package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// geminiFlashRunner implements Runner using `gemini --yolo --model=gemini-2.5-flash`.
type geminiFlashRunner struct {
	opts RunnerOptions
}

func (r *geminiFlashRunner) Run(stdin io.Reader, stdout, stderr io.Writer) error {
	ctx, cancel := contextWithOptionalTimeout(r.opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gemini", "--yolo", "--model=gemini-2.5-flash")
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if r.opts.WorkDir != "" {
		cmd.Dir = r.opts.WorkDir
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("gemini-flash pipeline timed out after %s — the codebase may be in a partial state; resume from the last completed change", r.opts.Timeout)
		}
		return fmt.Errorf("gemini-flash pipeline failed: %w", err)
	}
	return nil
}
