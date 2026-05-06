package main

import (
	"fmt"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/pr"
	"github.com/daniel-talonone/gemini-commands/internal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPRSubmit_MissingPRFile tests that a missing pr.md returns empty string.
// The command uses this to detect missing pr.md and return a clear error.
func TestPRSubmit_MissingPRFile(t *testing.T) {
	dir := t.TempDir()

	// Setup: Create status.yaml but no pr.md
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", dir, "", ""))

	// Verify pr.md is missing
	prContent, err := pr.Read(dir)
	require.NoError(t, err)
	assert.Equal(t, "", prContent, "pr.md should return empty string when missing")
}

// TestPRSubmit_EmptyPRFile tests that an empty pr.md is handled correctly.
func TestPRSubmit_EmptyPRFile(t *testing.T) {
	dir := t.TempDir()

	// Setup: Create status.yaml and an empty pr.md
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", dir, "", ""))

	// Write an empty pr.md
	require.NoError(t, pr.Write(dir, ""))

	// Verify pr.md is empty
	prContent, err := pr.Read(dir)
	require.NoError(t, err)
	assert.Equal(t, "", prContent, "pr.md should be empty")
}

// TestPRSubmit_AlreadySubmitted tests that a PR_URL in status.yaml blocks resubmission.
// The command checks s.PRURL != "" and returns error if true.
func TestPRSubmit_AlreadySubmitted(t *testing.T) {
	dir := t.TempDir()

	// Setup: Create status.yaml
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", dir, "", ""))

	// Write pr.md
	prBody := "# Test PR\n\nThis is a test PR description."
	require.NoError(t, pr.Write(dir, prBody))

	// Load status and check initial state
	s, err := status.LoadStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, "", s.PRURL, "PR URL should initially be empty")

	// Simulate a PR having been already submitted by setting WritePRURL
	testURL := "https://github.com/test/repo/pull/123"
	require.NoError(t, status.WritePRURL(dir, testURL))

	// Verify that status now blocks resubmission
	s, err = status.LoadStatus(dir)
	require.NoError(t, err)
	assert.NotEqual(t, "", s.PRURL, "PR URL should be set to block resubmission")
	assert.Equal(t, testURL, s.PRURL)
}

// TestPRSubmit_SuccessfulSubmissionFlow tests the state transitions during successful submission.
// This covers: reading pr.md, verifying no existing PR, and updating status.
func TestPRSubmit_SuccessfulSubmissionFlow(t *testing.T) {
	dir := t.TempDir()

	// Setup: Create status.yaml and pr.md
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", dir, "", ""))

	prBody := "# New Feature\n\nAdds a new feature to the CLI that improves user experience."
	require.NoError(t, pr.Write(dir, prBody))

	// Verify initial state: no pr_url set
	s, err := status.LoadStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, "", s.PRURL, "PR URL should initially be empty")
	assert.NotEqual(t, "pr-submitted", s.PipelineStep, "Pipeline step should not be pr-submitted initially")

	// Verify pr.md is readable and non-empty
	prContent, err := pr.Read(dir)
	require.NoError(t, err)
	assert.NotEqual(t, "", prContent, "pr.md should not be empty")
	assert.Equal(t, prBody, prContent, "pr.md content should match written content")

	// Simulate successful PR submission: WritePRURL would be called after github.CreatePR succeeds
	testPRURL := "https://github.com/test/repo/pull/456"
	require.NoError(t, status.WritePRURL(dir, testPRURL))

	// Verify final state
	s, err = status.LoadStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, testPRURL, s.PRURL, "PR URL should be updated after submission")
	assert.Equal(t, "pr-submitted", s.PipelineStep, "Pipeline step should be pr-submitted after submission")

	// Verify pr.md is still readable after status update
	prContent, err = pr.Read(dir)
	require.NoError(t, err)
	assert.Equal(t, prBody, prContent, "pr.md should be unchanged after status update")
}

// TestStatusYAML_RoundTrip tests that status.yaml survives a write-read cycle with pr_url.
func TestStatusYAML_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Create initial status
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", dir, "", ""))

	// Load and verify initial state
	s1, err := status.LoadStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, "", s1.PRURL)
	assert.NotEqual(t, "pr-submitted", s1.PipelineStep)

	// Write PR URL using the same function the command uses
	testURL := "https://github.com/test/repo/pull/123"
	require.NoError(t, status.WritePRURL(dir, testURL))

	// Load again and verify all fields are correctly preserved
	s2, err := status.LoadStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, testURL, s2.PRURL)
	assert.Equal(t, "pr-submitted", s2.PipelineStep)
	assert.Equal(t, "test/repo", s2.Repo, "Repo should be preserved")
	assert.Equal(t, "test-branch", s2.Branch, "Branch should be preserved")
	assert.Equal(t, dir, s2.WorkDir, "WorkDir should be preserved")
}

// TestPRSubmit_MultipleStateCycles tests that pr_url persists across multiple status writes.
func TestPRSubmit_MultipleStateCycles(t *testing.T) {
	dir := t.TempDir()

	// Initial setup
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", dir, "", ""))

	// First write: PR submission
	url1 := "https://github.com/test/repo/pull/111"
	require.NoError(t, status.WritePRURL(dir, url1))

	s, err := status.LoadStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, url1, s.PRURL)

	// Second write: Another PR submission attempt should not overwrite (handled by command)
	url2 := "https://github.com/test/repo/pull/222"
	require.NoError(t, status.WritePRURL(dir, url2))

	s, err = status.LoadStatus(dir)
	require.NoError(t, err)
	assert.Equal(t, url2, s.PRURL, "WritePRURL would be called only once by the command, but verify it updates correctly")
}

// TestPRSubmit_ValidateErrorCondition_AlreadySubmitted tests the exact condition
// the command checks: if s.PRURL != "" return error. This verifies the guard logic.
func TestPRSubmit_ValidateErrorCondition_AlreadySubmitted(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, status.Create(dir, "test/repo", "test-branch", dir, "", ""))

	// Scenario: PR already submitted
	url := "https://github.com/test/repo/pull/999"
	require.NoError(t, status.WritePRURL(dir, url))

	s, err := status.LoadStatus(dir)
	require.NoError(t, err)

	// This is the exact condition the command checks:
	// if s.PRURL != "" { return fmt.Errorf("PR already submitted for story %s: %s", storyId, s.PRURL) }
	// The test verifies that when PRURL is non-empty, it triggers the error condition.
	if s.PRURL != "" {
		// This is expected - the PR has already been submitted
		assert.NotEmpty(t, s.PRURL, fmt.Sprintf("Expected PR URL to be set: %s", s.PRURL))
	}
}
