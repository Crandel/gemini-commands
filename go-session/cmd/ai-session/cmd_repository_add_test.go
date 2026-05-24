package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/repository"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// newTestRootCmd creates a new root command for testing purposes,
// ensuring that command structure is reset for each test run to avoid state leakage.
func newTestRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "ai-session"}
	// Reset commands and flags to ensure a clean slate for each test
	repositoryCmd.ResetCommands()

	// Re-configure commands
	// This is necessary because flags are defined in init() functions, which don't re-run for each test.
	repositoryAddCmd.ResetFlags()
	configureRepositoryAddCmd()

	repositoryCmd.AddCommand(repositoryAddCmd)
	repositoryCmd.AddCommand(repositoryListCmd)
	root.AddCommand(repositoryCmd)
	return root
}

func TestRepositoryAddCmd(t *testing.T) {
	// Create a temporary directory for valid work_dir tests
	tmpDir, err := os.MkdirTemp("", "test-workdir-*")
	assert.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		assert.NoError(t, err)
	}()

	validConfig := repository.RepositoryConfig{
		WorkDir:    tmpDir,
		RepoName:   "org/repo1",
		IsWorktree: boolPtr(false),
		AgentsPath: "/path/to/agents1",
		VerifyConfig: &repository.VerifyConfig{
			Build: "make build",
			Test:  "make test",
			Lint:  "make lint",
		},
	}
	validConfigJSON, err := json.Marshal(validConfig)
	assert.NoError(t, err)

	t.Run("add valid config", func(t *testing.T) {
		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })
		rootCmd := newTestRootCmd()
		out, err := executeCommand(rootCmd, "repository", "add", "--config-json", string(validConfigJSON))
		assert.NoError(t, err)
		assert.Equal(t, `repository "org/repo1" added successfully`, out)
	})

	t.Run("add with missing config-json flag", func(t *testing.T) {
		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })
		rootCmd := newTestRootCmd()
		_, err := executeCommand(rootCmd, "repository", "add")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `required flag(s) "config-json" not set`)
	})

	t.Run("add with invalid json", func(t *testing.T) {
		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })
		rootCmd := newTestRootCmd()
		_, err := executeCommand(rootCmd, "repository", "add", "--config-json", `{"bad json"`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --config-json: unexpected end of JSON input")
	})

	t.Run("add duplicate repo", func(t *testing.T) {
		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })

		// Add the repo first
		rootCmd := newTestRootCmd()
		_, err := executeCommand(rootCmd, "repository", "add", "--config-json", string(validConfigJSON))
		assert.NoError(t, err)

		// Try to add it again
		rootCmd = newTestRootCmd()
		_, err = executeCommand(rootCmd, "repository", "add", "--config-json", string(validConfigJSON))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `repository "org/repo1" already exists`)
	})
}
