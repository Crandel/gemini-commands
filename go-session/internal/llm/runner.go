package llm

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Runner abstracts the LLM backend used to execute prompts.
type Runner interface {
	Run(stdin io.Reader, stdout, stderr io.Writer) error
}

// RunnerOptions configures LLM runner behaviour.
// Timeout sets a wall-clock deadline for a single Run call. Zero means no
// deadline. When the deadline is exceeded the process is killed and Run
// returns a descriptive error that the implement retry loop injects into the
// next attempt's prompt as context.
type RunnerOptions struct {
	Timeout time.Duration
}

// Model selects the LLM backend.
type Model string

const (
	ModelGemini      Model = "gemini"
	ModelGeminiFlash Model = "gemini-flash"
	ModelClaude      Model = "claude"
)

// contextWithOptionalTimeout returns a context with a deadline when d > 0, or
// context.Background() with a no-op cancel when d is zero.
func contextWithOptionalTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	if d > 0 {
		return context.WithTimeout(context.Background(), d)
	}
	return context.Background(), func() {}
}

// NewRunner returns a Runner for the given model.
func NewRunner(model Model, opts RunnerOptions) (Runner, error) {
	switch model {
	case ModelGemini:
		return &geminiRunner{opts: opts}, nil
	case ModelGeminiFlash:
		return &geminiFlashRunner{opts: opts}, nil
	case ModelClaude:
		return &claudeRunner{opts: opts}, nil
	default:
		return nil, fmt.Errorf("unknown model %q — must be one of: gemini, gemini-flash, claude", model)
	}
}
