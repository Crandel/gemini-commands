package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/daniel-talonone/gemini-commands/internal/git"
	"github.com/daniel-talonone/gemini-commands/internal/github"
	"github.com/daniel-talonone/gemini-commands/internal/pr"
	"github.com/daniel-talonone/gemini-commands/internal/status"
	"github.com/spf13/cobra"
)

// ResolveFeatureDir is a variable to allow mocking in tests.
var ResolveFeatureDir = feature.ResolveFeatureDir

// PrSubmitCmd represents the submit command. Exported for main package to use.
var PrSubmitCmd = &cobra.Command{
	Use:   "submit <story-id>",
	Short: "Submits a GitHub PR and updates feature state",
	Long: `Submits a GitHub pull request using the PR description from pr.md.

The command:
1. Resolves the feature directory for the given story-id
2. Validates that pr.md is present and non-empty
3. Checks that no PR has already been submitted (pr_url in status.yaml)
4. Reads the PR title from status.yaml (set by 'ai-session create-pr-description')
5. Calls github.CreatePR with the title and pr.md body
6. Updates status.yaml with the PR URL and sets pipeline_step to pr-submitted

Fails with a clear error if:
- pr.md is missing or empty
- pr_title is missing in status.yaml (run create-pr-description first)
- A PR has already been submitted (pr_url already in status.yaml)
- PR creation fails`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storyId := args[0]

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current working directory: %w", err)
		}

		featureDir, err := ResolveFeatureDir(storyId, cwd, git.RemoteURL())
		if err != nil {
			return fmt.Errorf("resolving feature directory: %w", err)
		}

		s, err := status.LoadStatus(featureDir)
		if err != nil {
			return fmt.Errorf("loading status.yaml: %w", err)
		}

		if s.PRURL != "" {
			return fmt.Errorf("PR already submitted for story %s: %s", storyId, s.PRURL)
		}

		prContent, err := pr.Read(featureDir)
		if err != nil {
			return fmt.Errorf("reading pr.md: %w", err)
		}

		if strings.TrimSpace(prContent) == "" || strings.TrimSpace(prContent) == "# Pull Request" {
			return fmt.Errorf("pr.md is missing or empty for story %s — run 'ai-session create-pr-description %s' first", storyId, storyId)
		}

		if s.PRTitle == "" {
			return fmt.Errorf("pr_title not set for story %s — run 'ai-session create-pr-description %s' first", storyId, storyId)
		}

		base := git.DefaultBranch()
		head := git.CurrentBranch()
		if head == "" {
			return fmt.Errorf("unable to determine current git branch")
		}

		prURL, err := github.CreatePR(s.WorkDir, base, head, s.PRTitle, prContent)
		if err != nil {
			return fmt.Errorf("creating PR: %w", err)
		}

		if err := status.WritePRURL(featureDir, prURL); err != nil {
			return fmt.Errorf("updating status.yaml: %w", err)
		}

		fmt.Printf("Pull request submitted successfully: %s\n", prURL)
		return nil
	},
}
