package plan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/daniel-talonone/gemini-commands/internal/llm"
	"github.com/daniel-talonone/gemini-commands/internal/status"
)

const enrichScript = "scripts/enrich_tasks.sh"

// RunSkipEnrich is like Run but skips the post-plan task enrichment step.
func RunSkipEnrich(ctx context.Context, storyID, featureDir string, runner llm.Runner, progress func(string)) error {
	return runPipeline(ctx, storyID, featureDir, runner, progress, true)
}

// Run executes the plan generation pipeline for the given feature.
//
//   - storyID is the feature identifier (e.g. "sc-1234").
//   - featureDir is the absolute path to the feature directory.
//   - runner is the LLM backend to use.
//   - progress is called at each milestone with a short human-readable label.
//     It must be non-nil and is called synchronously from the pipeline goroutine.
//
// Errors are returned so the caller can publish a "failed" event.
func Run(ctx context.Context, storyID, featureDir string, runner llm.Runner, progress func(string)) error {
	return runPipeline(ctx, storyID, featureDir, runner, progress, false)
}

func runPipeline(ctx context.Context, storyID, featureDir string, runner llm.Runner, progress func(string), skipEnrich bool) error {
	progress("Checking prerequisites")
	descPath := filepath.Join(featureDir, "description.md")
	if _, err := os.Stat(descPath); os.IsNotExist(err) {
		return fmt.Errorf("description.md not found in %s — run /session:new or /session:define first", featureDir)
	}

	progress("Starting plan generation")
	if err := status.Write(featureDir, "plan", "", ""); err != nil {
		return fmt.Errorf("updating status: %w", err)
	}

	progress("Preparing prompt")
	prompt, err := PreparePrompt(storyID, featureDir)
	if err != nil {
		_ = status.Write(featureDir, "plan-failed", "", "")
		return fmt.Errorf("preparing plan prompt: %w", err)
	}

	progress("Running LLM")
	if err := runner.Run(strings.NewReader(prompt), os.Stdout, os.Stderr); err != nil {
		_ = status.Write(featureDir, "plan-failed", "", "")
		return fmt.Errorf("plan generation failed: %w", err)
	}

	progress("Validating plan")
	p, err := LoadPlan(featureDir)
	if err != nil {
		_ = status.Write(featureDir, "plan-failed", "", "")
		return fmt.Errorf("plan.yml is missing or invalid: %w", err)
	}
	if len(p) == 0 {
		_ = status.Write(featureDir, "plan-failed", "", "")
		return fmt.Errorf("plan.yml is empty — LLM did not generate any plan content")
	}

	if err := status.Write(featureDir, "plan-done", "", ""); err != nil {
		return fmt.Errorf("updating status to plan-done: %w", err)
	}

	if skipEnrich {
		return nil
	}

	progress("Enriching tasks")
	enrichScriptPath := filepath.Join(AISessionHome(), enrichScript)
	enrichCmd := exec.CommandContext(ctx, enrichScriptPath, featureDir)
	enrichCmd.Stdout = os.Stdout
	enrichCmd.Stderr = os.Stderr
	if err := enrichCmd.Run(); err != nil {
		// Non-fatal: enrichment failure does not invalidate the plan.
		fmt.Fprintf(os.Stderr, "warning: enrichment script failed: %v\n", err)
	}

	return nil
}

// PreparePrompt reads the headless plan prompt template and substitutes the
// story ID and feature directory placeholders.
func PreparePrompt(storyID, featureDir string) (string, error) {
	promptPath := filepath.Join(AISessionHome(), "headless", "session", "plan.md")
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("reading prompt file at %s: %w", promptPath, err)
	}
	out := strings.ReplaceAll(string(content), "<story-id>", storyID)
	out = strings.ReplaceAll(out, "<feature-dir>", featureDir)
	return out, nil
}

// AISessionHome returns the root of the ai-session installation. It reads the
// AI_SESSION_HOME environment variable and falls back to inferring the path
// from the current executable location (assumes bin/ is two levels below root).
func AISessionHome() string {
	if home := os.Getenv("AI_SESSION_HOME"); home != "" {
		return home
	}
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(executable), "..", "..")
}
