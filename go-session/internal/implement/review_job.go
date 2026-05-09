package implement

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniel-talonone/gemini-commands/internal/review"
)

// ReviewJob implements the Job interface for addressing review findings.
type ReviewJob struct {
	featureDir      string
	workDir         string
	aiSessionHome   string
	reviewType      review.Type
	findings        string
	verificationCmd string
	logger          *slog.Logger
	lastError       string
}

// NewReviewJob creates a new ReviewJob.
func NewReviewJob(featureDir, workDir, aiSessionHome string, reviewType review.Type, findings, verificationCmd string, logger *slog.Logger) *ReviewJob {
	return &ReviewJob{
		featureDir:      featureDir,
		workDir:         workDir,
		aiSessionHome:   aiSessionHome,
		reviewType:      reviewType,
		findings:        findings,
		verificationCmd: verificationCmd,
		logger:          logger,
	}
}

// Prompt assembles the prompt for addressing review findings.
func (j *ReviewJob) Prompt() (string, error) {
	var promptPath string
	if j.reviewType == review.TypeRemote {
		promptPath = filepath.Join(j.aiSessionHome, "headless", "session", "address-feedback-remote.md")
	} else {
		promptPath = filepath.Join(j.aiSessionHome, "headless", "session", "address-feedback.md")
	}
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("reading prompt: %w", err)
	}
	promptTemplate := string(promptBytes)

	reviewTypeName, err := review.TypeName(j.reviewType)
	if err != nil {
		return "", err
	}

	var promptContent string
	if j.reviewType == review.TypeRemote {
		// address-feedback-remote.md uses {{pr_comments_here}} for the thread content.
		promptContent = strings.ReplaceAll(promptTemplate, "{{pr_comments_here}}", j.findings)
	} else {
		promptContent = strings.ReplaceAll(promptTemplate, "{{findings_here}}", j.findings)
	}
	promptContent = strings.ReplaceAll(promptContent, "{{feature_dir}}", j.featureDir)
	promptContent = strings.ReplaceAll(promptContent, "{{review_type_here}}", reviewTypeName)

	if j.lastError != "" {
		promptContent = strings.ReplaceAll(promptContent, "{{error_message_here}}", j.lastError)
		promptContent = strings.ReplaceAll(promptContent, "{{#if error_message}}", "")
		promptContent = strings.ReplaceAll(promptContent, "{{/if}}", "")
	} else {
		promptContent = errorBlockRe.ReplaceAllString(promptContent, "")
	}

	return promptContent, nil
}

// OnSuccess logs success and runs the verification gate.
// A non-nil return triggers a retry.
func (j *ReviewJob) OnSuccess(attempt int) error {
	reviewTypeName, err := review.TypeName(j.reviewType)
	if err != nil {
		return err
	}
	appendLog(j.logger, j.featureDir, fmt.Sprintf("Addressed %s review feedback successfully (attempt %d).", reviewTypeName, attempt))
	j.logger.Info("Review feedback addressed", "type", reviewTypeName, "attempt", attempt)
	return j.runVerification(attempt)
}

// OnFailure is called when the LLM exits non-zero. File changes may have landed
// anyway, so it checks the verification gate and returns the result. A non-nil
// return triggers a retry; nil means verification passed despite the LLM error.
func (j *ReviewJob) OnFailure(attempt int) error {
	reviewTypeName, err := review.TypeName(j.reviewType)
	if err != nil {
		return err
	}
	appendLog(j.logger, j.featureDir, fmt.Sprintf("LLM exited with error for %s review (attempt %d); checking verification.", reviewTypeName, attempt))
	j.logger.Error("LLM exited with error for review job", "type", reviewTypeName, "attempt", attempt)
	return j.runVerification(attempt)
}

// runVerification executes the verification command and stores the failure output
// in j.lastError for injection into the next attempt's prompt.
// Returns nil when verification passes or no command is configured.
func (j *ReviewJob) runVerification(attempt int) error {
	if j.verificationCmd == "" {
		j.logger.Info("No verification command, skipping verification.")
		appendLog(j.logger, j.featureDir, "No verification command found, skipping verification.")
		return nil
	}

	verificationOutput, verifyErr := runShellAndCaptureOutput(j.verificationCmd, j.featureDir)
	if verifyErr != nil {
		j.lastError = fmt.Sprintf("verification failed: %v\nOutput:\n%s", verifyErr, verificationOutput)
		appendLog(j.logger, j.featureDir, fmt.Sprintf("Verification failed (attempt %d): %s", attempt, verificationOutput))
		return fmt.Errorf("verification failed (attempt %d): %w", attempt, verifyErr)
	}

	appendLog(j.logger, j.featureDir, fmt.Sprintf("Verification passed (attempt %d).", attempt))
	j.logger.Info("Verification passed", "attempt", attempt)
	return nil
}
