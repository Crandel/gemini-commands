package main

import (
	"fmt"
	"os"
	"time"

	"github.com/daniel-talonone/gemini-commands/internal/llm"
	"github.com/spf13/cobra"
)

var (
	modelFlag   string
	timeoutFlag time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "ai-session",
	Short: "Local file operations for the ai-session workflow",
	Long: `ai-session manages feature directories for the ai-session workflow.

It provides deterministic, testable replacements for shell scripts and yq
one-liners. Designed to be invoked by LLMs — every subcommand has strict
input validation and --help output sufficient to use without reading source.

Feature directory: a directory containing description.md, plan.yml,
questions.yml, review.yml, log.md, and pr.md for a single story.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&modelFlag, "model", "gemini", "LLM backend: gemini, gemini-flash, or claude")
	rootCmd.PersistentFlags().DurationVar(&timeoutFlag, "llm-timeout", 0, "Wall-clock timeout per LLM call (0 = disabled). On timeout the run is retried with context that the previous attempt got stuck.")
}

// getRunner returns the Runner selected by the --model flag.
func getRunner() (llm.Runner, error) {
	return llm.NewRunner(llm.Model(modelFlag), llm.RunnerOptions{Timeout: timeoutFlag})
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
