package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescriptionUpsertCmd(t *testing.T) {
	// Setup a temporary feature directory
	dir := t.TempDir()
	featureDir := filepath.Join(dir, ".features", "test-org", "test-repo", "sc-123")
	require.NoError(t, os.MkdirAll(featureDir, 0755))

	// Mock the feature directory resolution
	originalResolveFeatureDir := feature.ResolveFeatureDirImpl
	feature.ResolveFeatureDirImpl = func(storyID, cwd, remoteURL string) (string, error) {
		return featureDir, nil
	}
	defer func() { feature.ResolveFeatureDirImpl = originalResolveFeatureDir }()

	t.Run("Success with argument", func(t *testing.T) {
		// Cleanup description file for this test
		_ = os.Remove(filepath.Join(featureDir, "description.md"))

		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		content := "description from arg"
		rootCmd.SetArgs([]string{"description", "upsert", "sc-123", content})

		err := rootCmd.Execute()
		require.NoError(t, err)

		assert.Contains(t, out.String(), "description.md written successfully.")

		// Verify file content
		fileContent, err := os.ReadFile(filepath.Join(featureDir, "description.md"))
		require.NoError(t, err)
		assert.Equal(t, content, string(fileContent))
	})

	t.Run("Success with stdin", func(t *testing.T) {
		// Cleanup description file for this test
		_ = os.Remove(filepath.Join(featureDir, "description.md"))

		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		content := "description from stdin"
		r, w, _ := os.Pipe()
		_, _ = w.WriteString(content)
		_ = w.Close()

		origStdin := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = origStdin }()

		rootCmd.SetArgs([]string{"description", "upsert", "sc-123"})

		err := rootCmd.Execute()
		require.NoError(t, err)

		assert.Contains(t, out.String(), "description.md written successfully.")

		// Verify file content
		fileContent, err := os.ReadFile(filepath.Join(featureDir, "description.md"))
		require.NoError(t, err)
		assert.Equal(t, content, string(fileContent))
	})

	t.Run("Failure with both stdin and argument", func(t *testing.T) {
		// No need to clean up, command should fail before writing
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		content := "description from stdin"
		r, w, _ := os.Pipe()
		_, _ = w.WriteString(content)
		_ = w.Close()

		origStdin := os.Stdin
		os.Stdin = r
		defer func() { os.Stdin = origStdin }()

		rootCmd.SetArgs([]string{"description", "upsert", "sc-123", "arg content"})

		err := rootCmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ambiguous input")
	})

	t.Run("Failure with no content", func(t *testing.T) {
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		// We need to ensure stat doesn't think it's a pipe
		// Let's try a different approach, redirect stdin from /dev/null
		f, err := os.Open(os.DevNull)
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		origStdin := os.Stdin
		os.Stdin = f
		defer func() { os.Stdin = origStdin }()

		rootCmd.SetArgs([]string{"description", "upsert", "sc-123"})

		err = rootCmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "neither stdin nor positional argument provided")
	})

	t.Run("Success on overwrite", func(t *testing.T) {
		// Ensure file exists
		initialContent := "existing content"
		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "description.md"), []byte(initialContent), 0644))

		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		newContent := "new content"
		rootCmd.SetArgs([]string{"description", "upsert", "sc-123", newContent})
		err := rootCmd.Execute()

		require.NoError(t, err)
		assert.Contains(t, out.String(), "description.md written successfully.")

		// Verify file content is overwritten
		fileContent, err := os.ReadFile(filepath.Join(featureDir, "description.md"))
		require.NoError(t, err)
		assert.Equal(t, newContent, string(fileContent))
	})
}
