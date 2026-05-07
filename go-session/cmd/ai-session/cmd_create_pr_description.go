package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/daniel-talonone/gemini-commands/internal/description"
	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/daniel-talonone/gemini-commands/internal/git"
	"github.com/daniel-talonone/gemini-commands/internal/log"
	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/daniel-talonone/gemini-commands/internal/pr"
	"github.com/daniel-talonone/gemini-commands/internal/status"
	"github.com/spf13/cobra"
)

const maxPRDescriptionAttempts = 3

var rePRTitle = regexp.MustCompile(`(?s)<title>(.*?)</title>`)
var rePRDescription = regexp.MustCompile(`(?s)<description>(.*?)</description>`)
var reConventionalTitle = regexp.MustCompile(`^(feat|fix|chore)\([a-z][a-z0-9-]*\): .{1,50}$`)

func init() {
	rootCmd.AddCommand(createPRDescriptionCmd)
}

var createPRDescriptionCmd = &cobra.Command{
	Use:   "create-pr-description <feature-name>",
	Short: "Generates a PR title and description, saving both to feature state",
	Long: `Generates a PR title and description based on the feature's context.

The LLM outputs a structured response with <title> and <description> XML tags.
The title is saved to status.yaml (pr_title) for use by 'ai-session pr submit'.
The description is saved to pr.md.
Retries up to 3 times if the output is missing or malformed.
Re-generation is idempotent — overwrites existing pr.md and pr_title.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		featureName := args[0]
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current working directory: %w", err)
		}
		featureDir, err := feature.ResolveFeatureDir(featureName, cwd, git.RemoteURL())
		if err != nil {
			return err
		}

		s, err := status.LoadStatus(featureDir)
		if err != nil {
			return fmt.Errorf("loading status.yaml: %w", err)
		}

		repo := git.OrgRepo()
		branch := git.CurrentBranch()
		if err := status.Write(featureDir, "create-pr-description", repo, branch); err != nil {
			return fmt.Errorf("updating status: %w", err)
		}

		desc, err := description.LoadDescription(featureDir)
		if err != nil {
			return fmt.Errorf("loading description.md: %w", err)
		}

		p, err := plan.LoadPlan(featureDir)
		if err != nil {
			return fmt.Errorf("loading plan.yml: %w", err)
		}
		planStr, err := p.ToString()
		if err != nil {
			return fmt.Errorf("marshaling plan: %w", err)
		}

		l, err := log.LoadLog(featureDir)
		if err != nil {
			return fmt.Errorf("loading log.md: %w", err)
		}

		// Optional PR template — render as a section only when present.
		prTemplateSection := ""
		prTemplatePath := filepath.Join(s.WorkDir, ".github", "pull_request_template.md")
		if content, err := os.ReadFile(prTemplatePath); err == nil {
			prTemplateSection = "**Pull Request Template:**\n\n" + string(content) + "\n\n---\n"
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("reading PR template: %w", err)
		}

		// Optional story URL — render as a line only when present.
		storyURLSection := ""
		if s.StoryURL != "" {
			storyURLSection = "\n**Story URL:** " + s.StoryURL + "\n"
		}

		diff, err := git.Diff(s.WorkDir)
		if err != nil {
			return fmt.Errorf("getting git diff: %w", err)
		}

		promptTemplatePath := filepath.Join(getAISessionHome(), "headless", "session", "create-pr-description.md")
		promptTemplate, err := os.ReadFile(promptTemplatePath)
		if err != nil {
			return fmt.Errorf("reading prompt template: %w", err)
		}

		promptContent := strings.ReplaceAll(string(promptTemplate), "{{description_here}}", desc)
		promptContent = strings.ReplaceAll(promptContent, "{{plan_here}}", planStr)
		promptContent = strings.ReplaceAll(promptContent, "{{log_here}}", l)
		promptContent = strings.ReplaceAll(promptContent, "{{diff_here}}", diff)
		promptContent = strings.ReplaceAll(promptContent, "{{pr_template_section_here}}", prTemplateSection)
		promptContent = strings.ReplaceAll(promptContent, "{{story_url_section_here}}", storyURLSection)
		promptContent = strings.ReplaceAll(promptContent, "{{story_id_here}}", featureName)

		runner, err := getRunner()
		if err != nil {
			return fmt.Errorf("invalid --model flag: %w", err)
		}

		var prTitle, prDescription string
		for attempt := 1; attempt <= maxPRDescriptionAttempts; attempt++ {
			fmt.Fprintf(os.Stderr, "Generating PR description for feature %s (attempt %d/%d)...\n", featureName, attempt, maxPRDescriptionAttempts)

			var out bytes.Buffer
			if err := runner.Run(strings.NewReader(promptContent), &out, os.Stderr); err != nil {
				return fmt.Errorf("generating PR description: %w", err)
			}

			output := out.String()
			prTitle, prDescription = parsePROutput(output)
			if prTitle != "" && prDescription != "" && reConventionalTitle.MatchString(prTitle) {
				break
			}

			if prTitle != "" && !reConventionalTitle.MatchString(prTitle) {
				fmt.Fprintf(os.Stderr, "Attempt %d: title %q does not match feat|fix|chore(<scope>): <summary>, retrying...\n", attempt, prTitle)
			} else {
				fmt.Fprintf(os.Stderr, "Attempt %d: missing <title> or <description> tag in output, retrying...\n", attempt)
			}
			prTitle, prDescription = "", ""
		}

		if prTitle == "" || prDescription == "" {
			return fmt.Errorf("failed to generate PR description after %d attempts: response missing <title> or <description> tag", maxPRDescriptionAttempts)
		}

		if err := pr.Write(featureDir, prDescription+"\n"); err != nil {
			return fmt.Errorf("writing pr.md: %w", err)
		}

		if err := status.WritePRTitle(featureDir, prTitle); err != nil {
			return fmt.Errorf("writing pr_title to status.yaml: %w", err)
		}

		fmt.Fprintf(os.Stderr, "PR title:  %s\n", prTitle)
		fmt.Fprintf(os.Stderr, "PR description written to pr.md\n")

		if err := status.Write(featureDir, "pr-description-done", repo, branch); err != nil {
			return fmt.Errorf("updating status: %w", err)
		}
		return nil
	},
}

// parsePROutput extracts title and description from the LLM's structured XML output.
func parsePROutput(output string) (title, description string) {
	if m := rePRTitle.FindStringSubmatch(output); len(m) == 2 {
		title = strings.TrimSpace(m[1])
	}
	if m := rePRDescription.FindStringSubmatch(output); len(m) == 2 {
		description = strings.TrimSpace(m[1])
	}
	return
}
