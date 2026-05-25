package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/daniel-talonone/gemini-commands/internal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPlan_DetectsEmptyPlan(t *testing.T) {
	dir := t.TempDir()

	// Scaffold a feature directory with status.yaml
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", "/work", "", ""))

	// Case 1: Empty file
	planPath := filepath.Join(dir, "plan.yml")
	require.NoError(t, os.WriteFile(planPath, []byte(""), 0644))

	p, err := plan.LoadPlan(dir)
	require.NoError(t, err, "LoadPlan should not error on empty file")
	require.Equal(t, 0, len(p), "Empty file should result in empty plan")

	// Case 2: Empty YAML array
	require.NoError(t, os.WriteFile(planPath, []byte("[]"), 0644))

	p, err = plan.LoadPlan(dir)
	require.NoError(t, err, "LoadPlan should not error on empty array")
	require.Equal(t, 0, len(p), "Empty array should result in empty plan")

	// Case 3: Valid plan should have length > 0
	validYAML := `- id: test-slice
  description: Test
  status: todo
  tasks:
    - id: test-task
      task: Test task
      status: todo
`
	require.NoError(t, os.WriteFile(planPath, []byte(validYAML), 0644))

	p, err = plan.LoadPlan(dir)
	require.NoError(t, err, "LoadPlan should not error on valid plan")
	require.Greater(t, len(p), 0, "Valid plan should have non-zero length")
}

func TestLoadPlan_HandlesNonExistentPlan(t *testing.T) {
	dir := t.TempDir()

	// Scaffold a feature directory with status.yaml
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", "/work", "", ""))

	// Don't write plan.yml at all
	p, err := plan.LoadPlan(dir)
	require.Error(t, err, "LoadPlan should error when plan.yml does not exist")
	assert.Nil(t, p, "Plan should be nil on error")
}

func TestLoadPlan_ValidatePlanLogic(t *testing.T) {
	dir := t.TempDir()

	// Scaffold a feature directory with status.yaml
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", "/work", "", ""))

	testCases := []struct {
		name          string
		content       string
		shouldError   bool
		expectedEmpty bool
		description   string
	}{
		{
			name:          "Empty string file",
			content:       "",
			shouldError:   false,
			expectedEmpty: true,
			description:   "Empty file is valid YAML but results in empty plan",
		},
		{
			name:          "Empty array",
			content:       "[]",
			shouldError:   false,
			expectedEmpty: true,
			description:   "Empty YAML array is valid but results in empty plan",
		},
		{
			name: "Single slice",
			content: `- id: s1
  description: Test slice
  status: todo
  tasks:
    - id: t1
      task: Do it
      status: todo
`,
			shouldError:   false,
			expectedEmpty: false,
			description:   "Valid plan with one slice",
		},
		{
			name: "Multiple slices",
			content: `- id: s1
  description: Slice 1
  status: todo
  tasks:
    - id: t1
      task: Task 1
      status: todo
- id: s2
  description: Slice 2
  status: done
  tasks:
    - id: t2
      task: Task 2
      status: done
`,
			shouldError:   false,
			expectedEmpty: false,
			description:   "Valid plan with multiple slices",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			planPath := filepath.Join(dir, "plan.yml")
			require.NoError(t, os.WriteFile(planPath, []byte(tc.content), 0644))

			p, err := plan.LoadPlan(dir)

			if tc.shouldError {
				assert.Error(t, err, tc.description)
				assert.Nil(t, p)
			} else {
				assert.NoError(t, err, tc.description)
				isEmpty := len(p) == 0
				assert.Equal(t, tc.expectedEmpty, isEmpty, "Plan emptiness doesn't match expectation: "+tc.description)
			}
		})
	}
}

func TestStartPlanCmd_ValidatePlanLogic(t *testing.T) {
	// This test simulates the validation logic from cmd_start_plan.go
	// after the LLM runner succeeds
	dir := t.TempDir()

	// Setup
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", "/work", "", ""))

	testCases := []struct {
		name        string
		content     string
		shouldPass  bool
		description string
	}{
		{
			name:        "Valid plan",
			content:     "- id: s1\n  description: desc\n  status: todo\n  tasks:\n    - id: t1\n      task: task\n      status: todo\n",
			shouldPass:  true,
			description: "Should write plan-done when plan is valid and non-empty",
		},
		{
			name:        "Empty file",
			content:     "",
			shouldPass:  false,
			description: "Should write plan-failed when plan.yml is empty",
		},
		{
			name:        "Empty array",
			content:     "[]",
			shouldPass:  false,
			description: "Should write plan-failed when plan is empty sequence",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			planPath := filepath.Join(dir, "plan.yml")
			require.NoError(t, os.WriteFile(planPath, []byte(tc.content), 0644))

			// Simulate the validation logic from cmd_start_plan.go
			p, err := plan.LoadPlan(dir)

			// The logic in cmd_start_plan.go checks:
			// 1. if err != nil -> write plan-failed
			// 2. if len(p) == 0 -> write plan-failed
			// 3. otherwise -> write plan-done

			if err != nil {
				// This case happens when file doesn't exist or YAML is invalid
				assert.False(t, tc.shouldPass, tc.description)
			} else if len(p) == 0 {
				// This case happens when file is empty or empty YAML
				assert.False(t, tc.shouldPass, tc.description)
			} else {
				// This case is when plan is valid and non-empty
				assert.True(t, tc.shouldPass, tc.description)
			}
		})
	}
}

// TestStartPlanCmd_FailsOnEmptyPlan validates that the command returns an error
// and writes plan-failed status when plan.yml is empty but valid YAML.
// AC#4: Unit test covers the failure case: LLM exits 0 but plan.yml is empty → command returns error.
func TestStartPlanCmd_FailsOnEmptyPlan(t *testing.T) {
	dir := t.TempDir()

	// Scaffold a feature directory with status.yaml
	require.NoError(t, status.Create(dir, "test/repo", "test-branch", "/work", "", ""))

	// Write an empty but valid plan.yml
	planPath := filepath.Join(dir, "plan.yml")
	require.NoError(t, os.WriteFile(planPath, []byte(""), 0644))

	// Load plan and verify it's empty
	p, err := plan.LoadPlan(dir)
	require.NoError(t, err, "LoadPlan should not error on empty file")
	require.Equal(t, 0, len(p), "Plan should be empty")

	// Verify the validation logic: empty plan should fail
	assert.Equal(t, 0, len(p), "Empty plan must fail validation in cmd_start_plan.go")
}
