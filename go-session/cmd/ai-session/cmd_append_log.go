package main

import (
	"fmt"
	"io"
	"os"

	commands "github.com/daniel-talonone/gemini-commands/internal/commands"
	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/daniel-talonone/gemini-commands/internal/git"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(appendLogCmd)
}

var appendLogCmd = &cobra.Command{
	Use:   "append-log <feature-dir> [message]",
	Short: "Append a timestamped entry to log.md",
	Long: `Appends a timestamped Markdown entry to log.md in the feature directory.

Arguments:
  <feature-dir>  Path to the feature directory, OR a story ID (e.g. sc-123) that
                 will be resolved to the feature directory automatically.
  [message]      Log message text. If omitted, message is read from stdin.

Entry format written to log.md:
  ## [2026-01-01T10:00:00Z]

  <message>

Entries are separated by a blank line. Creates log.md if it does not exist.

Examples:
  ai-session append-log ~/.features/org/repo/sc-123 "my message"
  ai-session append-log sc-123 "my message"
  cat report.md | ai-session append-log ~/.features/org/repo/sc-123

Errors:
  - Feature directory does not exist and cannot be resolved`,
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("exactly 2 arguments required: <feature-dir> and <message>, got %d", len(args))
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		var message string
		if len(args) == 2 {
			message = args[1]
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error reading stdin:", err)
				os.Exit(1)
			}
			message = string(data)
		}

		featureDir := args[0]
		if info, err := os.Stat(featureDir); err != nil || !info.IsDir() {
			// Not a direct path — try resolving as a story ID.
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			resolved, err := feature.ResolveFeatureDir(featureDir, cwd, git.RemoteURL())
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
			featureDir = resolved
		}

		if err := commands.AppendLog(featureDir, message); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return nil
	},
}
