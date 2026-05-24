package main

import (
	"encoding/json"
	"fmt"

	"github.com/daniel-talonone/gemini-commands/internal/repository"
	"github.com/spf13/cobra"
)

func init() {
	repositoryCmd.AddCommand(repositoryAddCmd)
	configureRepositoryAddCmd()
}

func configureRepositoryAddCmd() {
	repositoryAddCmd.Flags().String("config-json", "", "JSON-encoded repository configuration (required)")
	_ = repositoryAddCmd.MarkFlagRequired("config-json")
}

var repositoryAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Register a new repository in the registry",
	Long: `Parses --config-json and appends the entry to ~/.features/repositories_config.yaml.
Fails if repo_name already exists. Warns to stderr if verify_config is absent.

Required JSON fields: repo_name, work_dir, is_worktree, agents_path.
Optional: verify_config { build, test, lint }.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		raw, _ := cmd.Flags().GetString("config-json")
		var cfg repository.RepositoryConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("invalid --config-json: %w", err)
		}
		if err := repository.Add(cfg, cmd.ErrOrStderr()); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "repository %q added successfully\n", cfg.RepoName) //nolint:errcheck
		return nil
	},
}
