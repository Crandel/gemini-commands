package plan_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteQuestions(t *testing.T) {
	validYAML := `
questions:
  - id: q1
    question: "What is love?"
    status: open
  - id: q2
    question: "Baby don't hurt me"
    status: resolved
    answer: "No more"
`

	t.Run("writes valid questions atomically", func(t *testing.T) {
		tempDir := t.TempDir()
		err := plan.WriteQuestions(tempDir, []byte(validYAML))
		require.NoError(t, err)

		finalPath := filepath.Join(tempDir, "questions.yml")
		_, err = os.Stat(finalPath)
		require.NoError(t, err, "questions.yml should exist")

		content, err := os.ReadFile(finalPath)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(validYAML), strings.TrimSpace(string(content)))
	})

	t.Run("returns error for non-existent feature directory", func(t *testing.T) {
		err := plan.WriteQuestions("/non-existent-dir", []byte(validYAML))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "feature directory does not exist")
	})

	t.Run("returns LLM-actionable error for invalid YAML", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidYAML := `questions: - id: q1`
		err := plan.WriteQuestions(tempDir, []byte(invalidYAML))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parsing questions.yml")
	})

	validationTests := []struct {
		name          string
		yaml          string
		expectedError string
	}{
		{
			name: "missing id",
			yaml: `
questions:
  - question: "missing id"
    status: open`,
			expectedError: `questions[0].id: value is empty`,
		},
		{
			name: "invalid kebab-case id",
			yaml: `
questions:
  - id: "invalidID"
    question: "invalid id"
    status: open`,
			expectedError: `questions[0].id: "invalidID" is not kebab-case`,
		},
		{
			name: "empty question",
			yaml: `
questions:
  - id: "q1"
    question: ""
    status: open`,
			expectedError: `questions[0].question: value is empty`,
		},
		{
			name: "invalid status",
			yaml: `
questions:
  - id: "q1"
    question: "some question"
    status: "pending"`,
			expectedError: `questions[0].status: "pending" is not valid`,
		},
		{
			name: "indexed error message",
			yaml: `
questions:
  - id: q1
    question: "valid"
    status: open
  - id: Q2
    question: "invalid"
    status: resolved`,
			expectedError: `questions[1].id: "Q2" is not kebab-case`,
		},
	}

	for _, tc := range validationTests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			err := plan.WriteQuestions(tempDir, []byte(tc.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedError)

			// Check that no file was created
			_, err = os.Stat(filepath.Join(tempDir, "questions.yml"))
			assert.True(t, os.IsNotExist(err), "questions.yml should not have been created on validation failure")
		})
	}
}

func TestLoadQuestions(t *testing.T) {
	t.Run("returns content as string when questions.yml exists", func(t *testing.T) {
		tempDir := t.TempDir()
		expectedContent := `questions:
  - id: test-q
    question: "Test Question"
    status: open
`
		err := os.WriteFile(filepath.Join(tempDir, "questions.yml"), []byte(expectedContent), 0644)
		require.NoError(t, err)

		content, err := plan.LoadQuestions(tempDir)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, content)
	})

	t.Run("returns empty string when file doesn't exist", func(t *testing.T) {
		tempDir := t.TempDir()
		content, err := plan.LoadQuestions(tempDir)
		require.NoError(t, err)
		assert.Equal(t, "", content)
	})

	t.Run("returns error when feature directory doesn't exist", func(t *testing.T) {
		_, err := plan.LoadQuestions("/non-existent-feature-dir")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "feature directory does not exist")
	})
}
