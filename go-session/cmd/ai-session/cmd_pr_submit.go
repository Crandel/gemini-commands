package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/daniel-talonone/gemini-commands/internal/git"
	"github.com/daniel-talonone/gemini-commands/internal/github"
	"github.com/daniel-talonone/gemini-commands/internal/pr"
	"github.com/daniel-talonone/gemini-commands/internal/status"
	"github.com/spf13/cobra"
)

var prSubmitCmd = &cobra.Command{
	Use:   "submit <story-id>",
	Short: "Submits a GitHub PR and updates feature state",
	Long: `Submits a GitHub pull request using the PR description from pr.md.

The command:
1. Resolves the feature directory for the given story-id
2. Validates that pr.md exists and is non-empty
3. Checks that no PR has already been submitted (pr_url in status.yaml)
4. Uses the LLM to generate a conventional-commit-style title from pr.md content
5. Calls github.CreatePR with the generated title
6. Updates status.yaml with the PR URL and sets pipeline_step to pr-submitted

Fails with a clear error if:
- pr.md is missing or empty
- A PR has already been submitted (pr_url already in status.yaml)
- Title generation fails
- PR creation fails`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storyId := args[0]

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current working directory: %w", err)
		}

		featureDir, err := feature.ResolveFeatureDir(storyId, cwd, git.RemoteURL())
		if err != nil {
			return fmt.Errorf("resolving feature directory: %w", err)
		}

		// Load status to check if PR already exists
		s, err := status.LoadStatus(featureDir)
		if err != nil {
			return fmt.Errorf("loading status.yaml: %w", err)
		}

		// Check if PR was already submitted
		if s.PRURL != "" {
			return fmt.Errorf("PR already submitted for story %s: %s", storyId, s.PRURL)
		}

		// Read pr.md and validate it's non-empty
		prContent, err := pr.Read(featureDir)
		if err != nil {
			return fmt.Errorf("reading pr.md: %w", err)
		}

		if prContent == "" {
			return fmt.Errorf("pr.md is missing or empty for story %s — run 'ai-session create-pr-description %s' first", storyId, storyId)
		}

		// Generate title using LLM
		title, err := generatePRTitle(prContent)
		if err != nil {
			return fmt.Errorf("generating PR title: %w", err)
		}

		// Print generated title to stderr for visibility
		fmt.Fprintf(os.Stderr, "Generated PR title: %s\n", title)

		// Create PR using existing functions
		base := git.DefaultBranch()
		head := git.CurrentBranch()
		if head == "" {
			return fmt.Errorf("unable to determine current git branch")
		}

		prURL, err := github.CreatePR(s.WorkDir, base, head, title, prContent)
		if err != nil {
			return fmt.Errorf("creating PR: %w", err)
		}

		// Update status with PR URL
		if err := status.WritePRURL(featureDir, prURL); err != nil {
			return fmt.Errorf("updating status.yaml: %w", err)
		}

		fmt.Printf("Pull request submitted successfully: %s\n", prURL)
		return nil
	},
}

// generatePRTitle uses the LLM to extract a conventional-commit-style title from pr.md content.
func generatePRTitle(prContent string) (string, error) {
	promptTemplatePath := filepath.Join(getAISessionHome(), "headless", "session", "pr-submit-title.md")
	promptTemplate, err := os.ReadFile(promptTemplatePath)
	if err != nil {
		return "", fmt.Errorf("reading prompt template: %w", err)
	}

	promptContent := strings.ReplaceAll(string(promptTemplate), "{{pr_content}}", prContent)

	runner, err := getRunner()
	if err != nil {
		return "", fmt.Errorf("invalid --model flag: %w", err)
	}

	// Capture LLM output into a buffer
	var out bytes.Buffer
	var stderr bytes.Buffer
	if err := runner.Run(strings.NewReader(promptContent), &out, &stderr); err != nil {
		return "", fmt.Errorf("LLM error: %w", err)
	}

	// Trim whitespace and newlines
	title := strings.TrimSpace(out.String())
	if title == "" {
		return "", fmt.Errorf("LLM generated empty title")
	}

	return title, nil
}

func init() {
	prCmd.AddCommand(prSubmitCmd)
}
