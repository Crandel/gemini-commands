package implement

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/daniel-talonone/gemini-commands/internal/description"
	"github.com/daniel-talonone/gemini-commands/internal/llm"
	"github.com/daniel-talonone/gemini-commands/internal/log"
	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/daniel-talonone/gemini-commands/internal/status"
	"gopkg.in/yaml.v3"
)

// RunJob executes job up to maxRetries times: invokes the AI, then calls
// OnSuccess or OnFailure depending on the exit code. Rate-limit errors are
// retried on a separate budget without consuming the main attempt count.
func RunJob(featureDir string, maxRetries int, retryDelay time.Duration, logger *slog.Logger, runner llm.Runner, job Job) error {
	const maxRateLimitRetries = 20
	attempt := 1
	rateLimitRetries := 0
	var lastErr error

	for attempt <= maxRetries {
		if attempt > 1 && retryDelay > 0 {
			logger.Info("Waiting before retry", "delay", retryDelay, "attempt", attempt)
			time.Sleep(retryDelay)
		}

		promptContent, err := job.Prompt()
		if err != nil {
			return fmt.Errorf("building prompt (attempt %d): %w", attempt, err)
		}

		var llmOutput bytes.Buffer
		var llmErr error
		if os.Getenv("IN_TEST_MODE") != "true" {
			appendLog(logger, featureDir, fmt.Sprintf("Invoking LLM (attempt %d)", attempt))
			logger.Info("Invoking LLM", "attempt", attempt)
			llmErr = runner.Run(strings.NewReader(promptContent), io.MultiWriter(os.Stdout, &llmOutput), io.MultiWriter(os.Stderr, &llmOutput))
		} else {
			logger.Info("Skipping LLM invocation (test mode)", "attempt", attempt)
		}

		if llmErr != nil {
			appendLog(logger, featureDir, fmt.Sprintf("LLM exited with error (attempt %d): %v", attempt, llmErr))
			if isRateLimitError(llmOutput.String()) {
				rateLimitRetries++
				if rateLimitRetries >= maxRateLimitRetries {
					return fmt.Errorf("exceeded rate-limit retry budget (%d retries): %w", maxRateLimitRetries, llmErr)
				}
				continue
			}
			lastErr = job.OnFailure(attempt)
			if lastErr == nil {
				return nil
			}
		} else {
			lastErr = job.OnSuccess(attempt)
			if lastErr == nil {
				return nil
			}
		}

		attempt++
	}

	if lastErr != nil {
		return fmt.Errorf("job failed after %d attempts: %w", maxRetries, lastErr)
	}
	return fmt.Errorf("job failed after %d attempts", maxRetries)
}

// runShellAndCaptureOutput executes a shell command and captures its stdout/stderr.
func runShellAndCaptureOutput(cmdStr string) (string, error) {
	var output bytes.Buffer
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

// KnownStrategies returns a map of strategy name → Strategy instance.
// Use for O(1) validation and direct strategy instantiation.
func KnownStrategies() map[string]Strategy {
	return map[string]Strategy{
		"task":  &PerTaskStrategy{},
		"slice": &PerSliceStrategy{},
	}
}

// KnownStrategyNames returns the sorted list of valid strategy names derived from KnownStrategies().
// Use for flag help text, template dropdowns, and error messages.
func KnownStrategyNames() []string {
	m := KnownStrategies()
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

type Strategy interface {
	ExecuteSlice(ctx SliceContext) error
}

type SliceContext struct {
	FeatureDir      string
	WorkDir         string
	AISessionHome   string
	Story           string
	Architecture    string
	Slice           plan.Slice
	VerificationCmd string
	MaxRetries      int
	RetryDelay      time.Duration
	ContextPattern  []string
	Logger          *slog.Logger
	Runner          llm.Runner
}

type PerTaskStrategy struct{}

func (s *PerTaskStrategy) ExecuteSlice(ctx SliceContext) error {
	for j := range ctx.Slice.Tasks {
		t := &ctx.Slice.Tasks[j]
		if t.Status == "done" {
			continue
		}

		resuming := t.Status == "in-progress"
		if resuming {
			appendLog(ctx.Logger, ctx.FeatureDir, fmt.Sprintf("Resuming task: %s (was in-progress from a prior run)", t.ID))
		} else {
			appendLog(ctx.Logger, ctx.FeatureDir, fmt.Sprintf("Starting task: %s...", t.ID))
		}
		ctx.Logger.Info("Starting task", "slice", ctx.Slice.ID, "task", t.ID, "resuming", resuming)

		if err := plan.UpdateTask(ctx.FeatureDir, t.ID, "in-progress"); err != nil {
			return fmt.Errorf("updating task %s to in-progress: %w", t.ID, err)
		}

		if err := executeTaskWithRetry(ctx.Logger, ctx.FeatureDir, ctx.AISessionHome, ctx.WorkDir, ctx.Story, ctx.Architecture, ctx.Slice.Description, t.Task, ctx.VerificationCmd, ctx.MaxRetries, ctx.RetryDelay, ctx.ContextPattern, ctx.Runner); err != nil {
			appendLog(ctx.Logger, ctx.FeatureDir, fmt.Sprintf("Task %s FAILED after all retries: %v", t.ID, err))
			ctx.Logger.Error("Task failed", "task", t.ID, "error", err)
			return fmt.Errorf("task %s in slice %s failed: %w", t.ID, ctx.Slice.ID, err)
		}

		if err := plan.UpdateTask(ctx.FeatureDir, t.ID, "done"); err != nil {
			return fmt.Errorf("updating task %s to done: %w", t.ID, err)
		}
		t.Status = "done"
		appendLog(ctx.Logger, ctx.FeatureDir, fmt.Sprintf("Task %s completed successfully.", t.ID))
		ctx.Logger.Info("Task completed", "task", t.ID)
	}
	return nil
}

type PerSliceStrategy struct{}

// sliceJob implements the Job interface for executing a single slice.
type sliceJob struct {
	ctx       SliceContext
	lastError string
}

// newSliceJob creates a new sliceJob.
func newSliceJob(ctx SliceContext) *sliceJob {
	return &sliceJob{ctx: ctx}
}

// Prompt assembles the slice prompt by reading the template and plan from disk.
// It is called on every attempt so task statuses and the codebase diff are fresh.
func (j *sliceJob) Prompt() (string, error) {
	promptPath := filepath.Join(j.ctx.AISessionHome, "headless", "session", "execute_slice.md")
	promptTemplate, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("reading execute_slice.md: %w", err)
	}

	// Re-read tasks from disk — picks up status changes the LLM made in prior attempts.
	currentPlan, err := plan.LoadPlan(j.ctx.FeatureDir)
	if err != nil {
		return "", fmt.Errorf("reloading plan for slice %s: %w", j.ctx.Slice.ID, err)
	}
	currentSlice, found := currentPlan.FindSlice(j.ctx.Slice.ID)
	if !found {
		return "", fmt.Errorf("slice %s not found in plan", j.ctx.Slice.ID)
	}

	tasksBytes, err := yaml.Marshal(currentSlice.Tasks)
	if err != nil {
		return "", fmt.Errorf("serializing tasks for slice %s: %w", j.ctx.Slice.ID, err)
	}

	changesSoFar := getSourceFilesDiff(j.ctx.WorkDir, j.ctx.ContextPattern)

	promptContent := strings.ReplaceAll(string(promptTemplate), "{{story_description_here}}", j.ctx.Story)
	promptContent = strings.ReplaceAll(promptContent, "{{architecture_description_here}}", j.ctx.Architecture)
	promptContent = strings.ReplaceAll(promptContent, "{{slice_description_here}}", currentSlice.Description)
	promptContent = strings.ReplaceAll(promptContent, "{{tasks_here}}", string(tasksBytes))
	promptContent = strings.ReplaceAll(promptContent, "{{changes_so_far_here}}", changesSoFar)
	promptContent = strings.ReplaceAll(promptContent, "{{verification_command_here}}", j.ctx.VerificationCmd)
	promptContent = strings.ReplaceAll(promptContent, "{{feature_dir_here}}", j.ctx.FeatureDir)

	if j.lastError != "" {
		promptContent = strings.ReplaceAll(promptContent, "{{error_message_here}}", j.lastError)
		promptContent = strings.ReplaceAll(promptContent, "{{#if error_message}}", "")
		promptContent = strings.ReplaceAll(promptContent, "{{/if}}", "")
	} else {
		promptContent = errorBlockRe.ReplaceAllString(promptContent, "")
	}
	return promptContent, nil
}

// checkGates validates task completeness (Gate 1) and runs the verification
// command (Gate 2). Called by both OnSuccess and OnFailure — file changes may
// land even when Gemini exits non-zero.
func (j *sliceJob) checkGates(attempt int) error {
	reloadedPlan, err := plan.LoadPlan(j.ctx.FeatureDir)
	if err != nil {
		return fmt.Errorf("reloading plan after slice %s execution: %w", j.ctx.Slice.ID, err)
	}
	reloadedSlice, found := reloadedPlan.FindSlice(j.ctx.Slice.ID)
	if !found {
		return fmt.Errorf("slice %s not found in reloaded plan", j.ctx.Slice.ID)
	}
	var incomplete []string
	for _, t := range reloadedSlice.Tasks {
		if t.Status != "done" {
			incomplete = append(incomplete, fmt.Sprintf("%s (status: %s)", t.ID, t.Status))
		}
	}
	if len(incomplete) > 0 {
		j.lastError = fmt.Sprintf("not all tasks marked done: %s", strings.Join(incomplete, ", "))
		appendLog(j.ctx.Logger, j.ctx.FeatureDir, fmt.Sprintf("Gate 1 failed for slice %s (attempt %d): %s", j.ctx.Slice.ID, attempt, j.lastError))
		return errors.New(j.lastError)
	}

	verificationOutput, verifyErr := runShellAndCaptureOutput(j.ctx.VerificationCmd)
	if verifyErr != nil {
		j.lastError = fmt.Sprintf("verification failed: %v\nOutput:\n%s", verifyErr, verificationOutput)
		appendLog(j.ctx.Logger, j.ctx.FeatureDir, fmt.Sprintf("Gate 2 failed for slice %s (attempt %d): %v", j.ctx.Slice.ID, attempt, verifyErr))
		return errors.New(j.lastError)
	}

	appendLog(j.ctx.Logger, j.ctx.FeatureDir, fmt.Sprintf("Slice %s: all gates passed (attempt %d).", j.ctx.Slice.ID, attempt))
	j.ctx.Logger.Info("All gates passed", "slice", j.ctx.Slice.ID)
	return nil
}

func (j *sliceJob) OnSuccess(attempt int) error {
	return j.checkGates(attempt)
}

func (j *sliceJob) OnFailure(attempt int) error {
	// Gemini exited non-zero. File changes may still have landed — check gates anyway.
	appendLog(j.ctx.Logger, j.ctx.FeatureDir, fmt.Sprintf("Gemini exited with error for slice %s (attempt %d); checking gates — file changes may have landed.", j.ctx.Slice.ID, attempt))
	return j.checkGates(attempt)
}

func (s *PerSliceStrategy) ExecuteSlice(ctx SliceContext) error {
	return RunJob(ctx.FeatureDir, ctx.MaxRetries, ctx.RetryDelay, ctx.Logger, ctx.Runner, newSliceJob(ctx))
}

// errorBlockRe strips the entire {{#if error_message}}...{{/if}} block (including
// its XML wrapper) when there is no error to inject.
var errorBlockRe = regexp.MustCompile(`(?s)\{\{#if error_message\}\}.*?\{\{/if\}\}`)

// agentsVerifyRe extracts the verification command from an AGENTS.md "## Verification" section.
var agentsVerifyRe = regexp.MustCompile(`(?m)^## Verification\s*\nRun: (.+)$`)

// agentsContextFilesRe extracts the source file glob pattern from an AGENTS.md
// "## Context files" section.
var agentsContextFilesRe = regexp.MustCompile(`(?m)^## Context files\s*\nPattern: (.+)$`)

// defaultContextExcludes are git pathspec exclusions applied when no explicit
// "## Context files" pattern is configured in AGENTS.md. They strip known
// non-source files (config, docs, lock files, binaries) that produce formatting
// noise without containing any structural information the LLM needs.
var defaultContextExcludes = []string{
	":!*.yaml", ":!*.yml", ":!*.json", ":!*.toml",
	":!*.md", ":!*.txt", ":!*.lock", ":!*.sum",
	":!*.svg", ":!*.png", ":!*.jpg", ":!*.gif", ":!*.ico",
}

// appendLog writes a timestamped entry to log.md. Logging is best-effort — a
// failure is reported via the logger but never stops the orchestration.
func appendLog(logger *slog.Logger, featureDir, msg string) {
	if err := log.AppendLog(featureDir, msg); err != nil {
		logger.Warn("failed to append log entry", "error", err)
	}
}

// Run orchestrates the full implementation loop for a feature: reads the plan,
// runs the verification gate, iterates slices/tasks, invokes the LLM per task,
// and marks statuses as it progresses.
//
// workDir must be the target project root (the directory containing AGENTS.md).
// aiSessionHome must be the resolved AI_SESSION_HOME path.
// maxRetries is the maximum number of LLM+verification attempts per task.
// retryDelay is the pause between attempts (helps avoid LLM rate limits).
func Run(logger *slog.Logger, featureID, featureDir, workDir, aiSessionHome string, maxRetries int, retryDelay time.Duration, strategy Strategy, runner llm.Runner) (err error) {
	// Set the initial pipeline_step. If the previous run left "implement-failed"
	// (i.e. a human manually intervened and is restarting), use "implement-restarted"
	// so the dashboard reflects the context. Any other state is treated as a first run.
	currentStep, _ := status.ReadStep(featureDir)
	initialStep := "implement"
	if currentStep == "implement-failed" {
		initialStep = "implement-restarted"
	}
	if writeErr := status.Write(featureDir, initialStep, "", ""); writeErr != nil {
		logger.Warn("failed to write initial implement status", "error", writeErr)
	}

	// On any failure, mark the run as failed so the dashboard and the next invocation
	// can distinguish between "in progress", "failed", and "done".
	defer func() {
		if err != nil {
			if writeErr := status.Write(featureDir, "implement-failed", "", ""); writeErr != nil {
				logger.Warn("failed to write implement-failed status", "error", writeErr)
			}
		}
	}()

	appendLog(logger, featureDir, fmt.Sprintf("--- Starting implementation orchestration for feature: %s ---", featureID))
	logger.Info("Starting implementation orchestration", "feature_id", featureID)

	// Extract verification command from the project's AGENTS.md.
	verificationCmd, err := ExtractVerificationCommand(workDir)
	if err != nil {
		return fmt.Errorf("extracting verification command: %w", err)
	}

	// Optional source-file filter for the codebase context diff (see AGENTS.md
	// "## Context files" section). Falls back to defaultContextExcludes if absent.
	contextPattern := extractContextPattern(workDir)
	appendLog(logger, featureDir, fmt.Sprintf("Using verification command: %s", verificationCmd))
	logger.Info("Using verification command", "command", verificationCmd)

	// Initial verification gate — codebase must be passing before we start.
	appendLog(logger, featureDir, "Running initial verification gate...")
	logger.Info("Running initial verification gate")
	if err := runShell(verificationCmd, workDir); err != nil {
		msg := fmt.Sprintf("Initial verification failed — codebase must be in a passing state to begin: %v", err)
		appendLog(logger, featureDir, msg)
		return errors.New(msg)
	}
	appendLog(logger, featureDir, "Initial verification gate passed.")
	logger.Info("Initial verification gate passed.")

	// Read story description for prompt context.
	storyDescription, err := description.LoadDescription(featureDir)
	if err != nil {
		return fmt.Errorf("reading description.md: %w", err)
	}

	// Load architecture if present — gives the LLM design constraints and pattern refs.
	architectureDescription, err := plan.LoadArchitecture(featureDir)
	if err != nil {
		return fmt.Errorf("loading architecture: %w", err)
	}

	// Load plan.
	p, err := plan.LoadPlan(featureDir)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}

	// Main loop: process slices in dependency order using a progress-made pattern
	// to handle slices whose dependencies become satisfied mid-run.
	for {
		progressMade := false
		allDone := true

		for i := range p {
			s := &p[i]
			if s.Status == "done" {
				continue
			}
			allDone = false

			// Check all depends_on slices are done (consult in-memory Plan only).
			if !depsMet(p, s.DependsOn) {
				continue
			}

			appendLog(logger, featureDir, fmt.Sprintf("Starting slice: %s", s.ID))
			logger.Info("Starting slice", "id", s.ID)

			if err := plan.UpdateSlice(featureDir, s.ID, "in-progress"); err != nil {
				return fmt.Errorf("updating slice %s to in-progress: %w", s.ID, err)
			}

			sliceCtx := SliceContext{
				Logger:          logger,
				FeatureDir:      featureDir,
				AISessionHome:   aiSessionHome,
				WorkDir:         workDir,
				Story:           storyDescription,
				Architecture:    architectureDescription,
				VerificationCmd: verificationCmd,
				MaxRetries:      maxRetries,
				RetryDelay:      retryDelay,
				ContextPattern:  contextPattern,
				Slice:           *s,
				Runner:          runner,
			}

			if err := strategy.ExecuteSlice(sliceCtx); err != nil {
				return fmt.Errorf("slice %s failed: %w", s.ID, err)
			}

			if err := plan.UpdateSlice(featureDir, s.ID, "done"); err != nil {
				return fmt.Errorf("updating slice %s to done: %w", s.ID, err)
			}
			s.Status = "done"
			appendLog(logger, featureDir, fmt.Sprintf("Slice %s completed.", s.ID))
			logger.Info("Slice completed", "id", s.ID)
			progressMade = true
		}

		if allDone {
			break
		}
		if !progressMade {
			return fmt.Errorf("cannot make further progress — check for unsatisfiable or circular dependencies")
		}
	}

	// Final integration check: catches cross-cutting issues (template↔struct mismatches,
	// sort key inconsistencies, dead code, always-passing tests) that individual task
	// verification gates miss because each task only validates its own local scope.
	appendLog(logger, featureDir, "Running final integration check...")
	logger.Info("Running integration check")
	if err := runIntegrationCheck(logger, aiSessionHome, storyDescription, workDir, contextPattern, runner); err != nil {
		appendLog(logger, featureDir, fmt.Sprintf("Integration check found blockers: %v", err))
		return fmt.Errorf("integration check failed: %w", err)
	}
	appendLog(logger, featureDir, "Integration check passed.")

	if err := status.Write(featureDir, "implement-done", "", ""); err != nil {
		return fmt.Errorf("updating status to implement-done: %w", err)
	}
	appendLog(logger, featureDir, "--- IMPLEMENT COMPLETE ---")
	logger.Info("IMPLEMENT COMPLETE")
	return nil
}

// extractContextPattern reads the optional "## Context files" section
// from AGENTS.md and returns the space-separated glob list. Returns nil when the section
// is absent so callers can fall back to defaultContextExcludes.
func extractContextPattern(dir string) []string {
	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		return nil
	}
	matches := agentsContextFilesRe.FindSubmatch(content)
	if len(matches) < 2 {
		return nil
	}
	return strings.Fields(strings.TrimSpace(string(matches[1])))
}

// getSourceFilesDiff returns a git diff scoped to source files only.
//
// If includePatterns is non-empty (read from AGENTS.md "## Context files"), they are
// passed as include pathspecs: `git diff HEAD -- <patterns>`.
// Otherwise, defaultContextExcludes are used to strip noise (config, docs, lock files)
// while keeping all source languages: `git diff HEAD -- :!*.yaml :!*.md …`.
//
// A 32 KB cap prevents runaway context sizes. When truncated, the most recent hunks
// are kept (most likely to contain the task-relevant additions) and a header notes it.
func getSourceFilesDiff(workDir string, includePatterns []string) string {
	args := []string{"diff", "HEAD", "--"}
	if len(includePatterns) > 0 {
		args = append(args, includePatterns...)
	} else {
		args = append(args, defaultContextExcludes...)
	}

	var out bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}

	result := out.String()
	const maxBytes = 32 * 1024
	if len(result) <= maxBytes {
		return result
	}

	// Keep the tail of the diff and align to the next file header so we never
	// emit a partial hunk.
	tail := result[len(result)-maxBytes:]
	if idx := strings.Index(tail, "\ndiff --git"); idx >= 0 {
		tail = tail[idx+1:]
	}
	return fmt.Sprintf("[diff truncated: showing last %d of %d bytes — earliest changes omitted]\n\n%s",
		len(tail), len(result), tail)
}

// runIntegrationCheck invokes the LLM with integration_check.md to catch
// cross-cutting issues (template↔struct mismatches, dead code, broken tests)
// that per-task verification gates miss. Returns an error if the LLM reports
// any BLOCKERs (exits non-zero).
func runIntegrationCheck(logger *slog.Logger, aiSessionHome, storyDescription, workDir string, contextPattern []string, runner llm.Runner) error {
	if os.Getenv("IN_TEST_MODE") == "true" {
		logger.Info("Skipping integration check (test mode)")
		return nil
	}

	promptPath := filepath.Join(aiSessionHome, "headless", "session", "integration_check.md")
	promptTemplate, err := os.ReadFile(promptPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Prompt file is optional; skip gracefully rather than failing the run.
			logger.Warn("integration_check.md not found — skipping integration check", "path", promptPath)
			return nil
		}
		return fmt.Errorf("reading integration_check.md: %w", err)
	}

	diff := getSourceFilesDiff(workDir, contextPattern)
	prompt := strings.ReplaceAll(string(promptTemplate), "{{story_description_here}}", storyDescription)
	prompt = strings.ReplaceAll(prompt, "{{codebase_diff_here}}", diff)

	if err := runner.Run(strings.NewReader(prompt), os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("integration check reported blockers (gemini exit: %w)", err)
	}
	return nil
}

// isRateLimitError reports whether the combined stdout+stderr output of a failed
// Gemini invocation indicates a transient rate-limit or quota error. These errors
// should be retried without consuming the task's attempt budget.
func isRateLimitError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "ratelimitexceeded") ||
		strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "resource exhausted") ||
		strings.Contains(lower, "too many requests")
}

// executeTaskWithRetry invokes the LLM for a single task and retries on failure.
// In --yolo mode Gemini executes its own tool calls (run_shell_command, write_file, replace)
// live during the process — file changes happen inside the gemini call.
//
// A non-zero Gemini exit code is treated as a soft failure: verification still runs
// because file changes may have landed on disk. If verification passes, the task
// succeeds regardless of the Gemini exit code. If either fails, both errors are
// injected into the next attempt's prompt so the LLM can recover.
//
// retryDelay is applied before each retry to avoid LLM rate-limit errors that
// otherwise cause immediate failure on the following attempt.
func executeTaskWithRetry(logger *slog.Logger, featureDir, aiSessionHome, workDir, storyDescription, architectureDescription, sliceDescription, taskDescription, verificationCmd string, maxRetries int, retryDelay time.Duration, contextPattern []string, runner llm.Runner) error {
	promptPath := filepath.Join(aiSessionHome, "headless", "session", "execute_task.md")

	// Read prompt template once; it does not change between retries.
	promptTemplate, err := os.ReadFile(promptPath)
	if err != nil {
		return fmt.Errorf("reading execute_task.md: %w", err)
	}

	// attempt counts real failures against the budget. rateLimitRetries tracks
	// transient quota/rate-limit errors separately — they never consume the budget
	// but are capped to prevent an infinite loop.
	const maxRateLimitRetries = 20
	attempt := 1
	rateLimitRetries := 0
	var lastError error

	for attempt <= maxRetries {
		if (attempt > 1 || rateLimitRetries > 0) && retryDelay > 0 {
			logger.Info("Waiting before retry", "delay", retryDelay, "attempt", attempt, "rate_limit_retries", rateLimitRetries)
			time.Sleep(retryDelay)
		}

		// Capture the current diff before building the prompt so the LLM can see
		// all changes made by previous tasks (prevents field-name mismatches when
		// a later task references types introduced by an earlier one).
		changesSoFar := getSourceFilesDiff(workDir, contextPattern)

		promptContent := strings.ReplaceAll(string(promptTemplate), "{{story_description_here}}", storyDescription)
		promptContent = strings.ReplaceAll(promptContent, "{{architecture_description_here}}", architectureDescription)
		promptContent = strings.ReplaceAll(promptContent, "{{slice_description_here}}", sliceDescription)
		promptContent = strings.ReplaceAll(promptContent, "{{task_description_here}}", taskDescription)
		promptContent = strings.ReplaceAll(promptContent, "{{changes_so_far_here}}", changesSoFar)
		promptContent = strings.ReplaceAll(promptContent, "{{verification_command_here}}", verificationCmd)
		promptContent = strings.ReplaceAll(promptContent, "{{feature_dir_here}}", featureDir)
		if lastError != nil {
			promptContent = strings.ReplaceAll(promptContent, "{{error_message_here}}", lastError.Error())
			promptContent = strings.ReplaceAll(promptContent, "{{#if error_message}}", "")
			promptContent = strings.ReplaceAll(promptContent, "{{/if}}", "")
		} else {
			promptContent = errorBlockRe.ReplaceAllString(promptContent, "")
		}

		var geminiErr error
		var geminiOutput bytes.Buffer // captures stdout+stderr for rate-limit detection
		if os.Getenv("IN_TEST_MODE") != "true" {
			appendLog(logger, featureDir, fmt.Sprintf("Invoking LLM for task execution (attempt %d).", attempt))
			logger.Info("Invoking Gemini", "attempt", attempt)
			if err := runner.Run(strings.NewReader(promptContent), io.MultiWriter(os.Stdout, &geminiOutput), io.MultiWriter(os.Stderr, &geminiOutput)); err != nil {
				geminiErr = err
				appendLog(logger, featureDir, fmt.Sprintf("Gemini exited with error (attempt %d): %v — running verification anyway", attempt, err))
				logger.Warn("Gemini exited with error; running verification", "attempt", attempt, "error", err)
			}
		} else {
			logger.Info("Skipping Gemini invocation (test mode)", "attempt", attempt)
		}

		// Always run verification — file changes may have landed on disk even when
		// Gemini exited non-zero (e.g. due to a rate-limit or transient API error).
		var verificationOutput bytes.Buffer
		verifyCmd := exec.Command("sh", "-c", verificationCmd)
		verifyCmd.Dir = workDir
		verifyCmd.Stdout = &verificationOutput
		verifyCmd.Stderr = &verificationOutput
		verifyErr := verifyCmd.Run()

		if verifyErr == nil {
			appendLog(logger, featureDir, fmt.Sprintf("Verification passed (attempt %d).", attempt))
			logger.Info("Verification passed", "attempt", attempt)
			return nil
		}

		// Rate-limit / quota errors do not count against the attempt budget.
		if geminiErr != nil && isRateLimitError(geminiOutput.String()) {
			rateLimitRetries++
			msg := fmt.Sprintf("Rate-limit error on attempt %d (rate-limit retry %d/%d) — not counting against budget.", attempt, rateLimitRetries, maxRateLimitRetries)
			appendLog(logger, featureDir, msg)
			logger.Warn(msg)
			if rateLimitRetries >= maxRateLimitRetries {
				return fmt.Errorf("exceeded rate-limit retry budget (%d retries): %w", maxRateLimitRetries, geminiErr)
			}
			continue // attempt unchanged
		}

		// Real failure: build combined error for the next prompt and consume one attempt.
		var errParts []string
		if geminiErr != nil {
			errParts = append(errParts, geminiErr.Error())
		}
		errParts = append(errParts, verificationOutput.String())
		lastError = errors.New(strings.Join(errParts, "\n"))

		appendLog(logger, featureDir, fmt.Sprintf("Verification failed (attempt %d): %s", attempt, verificationOutput.String()))
		logger.Error("Verification failed", "attempt", attempt)

		if attempt == maxRetries {
			return lastError
		}
		attempt++
	}

	return lastError
}

// depsMet reports whether all slices in deps are marked "done" in the plan.
func depsMet(p plan.Plan, deps []string) bool {
	for _, depID := range deps {
		found := false
		for _, s := range p {
			if s.ID == depID {
				found = true
				if s.Status != "done" {
					return false
				}
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// extractVerificationCommand reads AGENTS.md from dir and extracts the verification
// command from a "## Verification" section.
func ExtractVerificationCommand(dir string) (string, error) {
	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		return "", fmt.Errorf("reading AGENTS.md: %w", err)
	}
	matches := agentsVerifyRe.FindSubmatch(content)
	if len(matches) < 2 {
		return "", fmt.Errorf("verification command not found in AGENTS.md (expected '## Verification\\nRun: <command>')")
	}
	return strings.TrimSpace(string(matches[1])), nil
}

// runShell executes a shell command in the given directory, streaming output to stdout/stderr.
func runShell(shellCmd, dir string) error {
	cmd := exec.Command("sh", "-c", shellCmd)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
