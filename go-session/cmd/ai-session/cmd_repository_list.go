package main

import (
	"fmt"
	"sort"

	"github.com/daniel-talonone/gemini-commands/internal/repository"
	"github.com/spf13/cobra"
)

func init() {
	repositoryCmd.AddCommand(repositoryListCmd)
}

var repositoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered repositories",
	// TODO: Add a --json or --output flag for machine-readable output.
	RunE: func(cmd *cobra.Command, _ []string) error {
		repos, err := repository.List()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if len(repos) == 0 {
			fmt.Fprintln(out, "no repositories registered") //nolint:errcheck
			return nil
		}
		names := make([]string, 0, len(repos))
		for name := range repos {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			r := repos[name]
			fmt.Fprintf(out, "repo:         %s\n", r.RepoName)   //nolint:errcheck
			fmt.Fprintf(out, "work_dir:     %s\n", r.WorkDir)    //nolint:errcheck
			fmt.Fprintf(out, "agents_path:  %s\n", r.AgentsPath) //nolint:errcheck
			if r.IsWorktree != nil {
				fmt.Fprintf(out, "is_worktree:  %v\n", *r.IsWorktree) //nolint:errcheck
			}
			if r.VerifyConfig != nil {
				fmt.Fprintf(out, "verify.build: %s\n", r.VerifyConfig.Build) //nolint:errcheck
				fmt.Fprintf(out, "verify.test:  %s\n", r.VerifyConfig.Test)  //nolint:errcheck
				fmt.Fprintf(out, "verify.lint:  %s\n", r.VerifyConfig.Lint)  //nolint:errcheck
			} else {
				fmt.Fprintln(out, "verify_config: (none)") //nolint:errcheck
			}
			fmt.Fprintln(out, "---") //nolint:errcheck
		}
		return nil
	},
}
