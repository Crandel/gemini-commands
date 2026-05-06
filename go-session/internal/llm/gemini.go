package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// geminiRunner implements Runner using `gemini --yolo`.
type geminiRunner struct {
	opts RunnerOptions
}

func (r *geminiRunner) Run(stdin io.Reader, stdout, stderr io.Writer) error {
	ctx, cancel := contextWithOptionalTimeout(r.opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gemini", "--yolo")
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("gemini pipeline timed out after %s — the codebase may be in a partial state; resume from the last completed change", r.opts.Timeout)
		}
		return fmt.Errorf("gemini pipeline failed: %w", err)
	}
	return nil
}

// RunYolo executes `gemini --yolo` with the given stdin.
// Deprecated: prefer NewRunner(ModelGemini, RunnerOptions{}).Run(...).
func RunYolo(stdin io.Reader, stdout, stderr io.Writer) error {
	return (&geminiRunner{}).Run(stdin, stdout, stderr)
}
