package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/repository"
	"github.com/stretchr/testify/assert"
)

func TestRepositoryListCmd(t *testing.T) {
	// Create a temporary directory for valid work_dir tests
	tmpDir, err := os.MkdirTemp("", "test-workdir-*")
	assert.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		assert.NoError(t, err)
	}()

	t.Run("list with no repositories", func(t *testing.T) {
		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })
		rootCmd := newTestRootCmd()
		out, err := executeCommand(rootCmd, "repository", "list")
		assert.NoError(t, err)
		assert.Equal(t, "no repositories registered", out)
	})

	t.Run("list with multiple repositories", func(t *testing.T) {
		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })

		// Add a repository to test the list output
		repo1 := repository.RepositoryConfig{
			WorkDir:    tmpDir,
			RepoName:   "org/repoA",
			IsWorktree: boolPtr(false),
			AgentsPath: "/path/to/agentsA",
			VerifyConfig: &repository.VerifyConfig{Build: "buildA", Test: "testA", Lint: "lintA"},
		}
		err := repository.Add(repo1, io.Discard)
		assert.NoError(t, err)

		repo2 := repository.RepositoryConfig{
			WorkDir:    tmpDir,
			RepoName:   "org/repoB",
			IsWorktree: boolPtr(true),
			AgentsPath: "/path/to/agentsB",
		}
		err = repository.Add(repo2, io.Discard)
		assert.NoError(t, err)

		rootCmd := newTestRootCmd()
		out, err := executeCommand(rootCmd, "repository", "list")
		assert.NoError(t, err)

		// The output is sorted by repo name, so repoA should come first
		expectedParts := []string{
			"repo:         org/repoA",
			"work_dir:     " + tmpDir,
			"agents_path:  /path/to/agentsA",
			"is_worktree:  false",
			"verify.build: buildA",
			"verify.test:  testA",
			"verify.lint:  lintA",
			"---",
			"repo:         org/repoB",
			"work_dir:     " + tmpDir,
			"agents_path:  /path/to/agentsB",
			"is_worktree:  true",
			"verify_config: (none)",
			"---",
		}
		expectedOut := strings.Join(expectedParts, "\n")
		assert.Equal(t, expectedOut, out)
	})
}
