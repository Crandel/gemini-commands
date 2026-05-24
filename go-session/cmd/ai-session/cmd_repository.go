package main

import "github.com/spf13/cobra"

var repositoryCmd = &cobra.Command{
    Use:   "repository",
    Short: "Operations for managing repository configurations.",
    Long:  `Commands for registering and listing repository configurations stored in ~/.features/repositories_config.yaml.`,
}

func init() {
    rootCmd.AddCommand(repositoryCmd)
}
