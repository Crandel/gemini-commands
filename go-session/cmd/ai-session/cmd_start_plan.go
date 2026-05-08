package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/daniel-talonone/gemini-commands/internal/git"
	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(startPlanCmd)
	startPlanCmd.Flags().Bool("skip-enrich", false, "Skip the post-plan task enrichment step")
}

var startPlanCmd = &cobra.Command{
	Use:   "start-plan <story-id>",
	Short: "Start the headless plan for a feature",
	Long: `Executes the headless plan for a given feature.
This command replaces the 'orchestrate.sh --plan' script.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		storyID := args[0]

		logger.Info("Resolving feature directory", "story_id", storyID)
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current working directory: %w", err)
		}
		featureDir, err := feature.ResolveFeatureDir(storyID, cwd, git.RemoteURL())
		if err != nil {
			return err
		}
		logger.Info("Feature directory resolved", "path", featureDir)

		runner, err := getRunner()
		if err != nil {
			return fmt.Errorf("invalid --model flag: %w", err)
		}

		skipEnrich, _ := cmd.Flags().GetBool("skip-enrich")
		runFn := plan.Run
		if skipEnrich {
			runFn = plan.RunSkipEnrich
		}

		if err := runFn(cmd.Context(), storyID, featureDir, runner, func(msg string) {
			logger.Info(msg)
		}); err != nil {
			return err
		}

		fmt.Println("Plan generation successful.")
		return nil
	},
}
