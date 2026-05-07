package github

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to mock exec.Command
func mockExecCommand(t *testing.T, expectedCmd string, expectedArgs []string, stdout, stderr string, exitCode int) func(name string, arg ...string) *exec.Cmd {
	return func(name string, arg ...string) *exec.Cmd {
		args := strings.Join(arg, " ")
		fullCmd := fmt.Sprintf("%s %s", name, args)

		assert.Contains(t, fullCmd, expectedCmd, "Command mismatch")
		for _, eArg := range expectedArgs {
			assert.Contains(t, fullCmd, eArg, "Argument mismatch")
		}

		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, arg...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{
			"GO_WANT_HELPER_PROCESS=1",
			fmt.Sprintf("STDOUT=%s", stdout),
			fmt.Sprintf("STDERR=%s", stderr),
			fmt.Sprintf("EXIT_CODE=%d", exitCode),
		}
		return cmd
	}
}

// TestHelperProcess is not a real test. It's a helper for TestCreatePR.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	_, err := fmt.Fprint(os.Stdout, os.Getenv("STDOUT"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to stdout: %v\n", err)
		os.Exit(1)
	}
	_, err = fmt.Fprint(os.Stderr, os.Getenv("STDERR"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to stderr: %v\n", err)
		os.Exit(1)
	}
	os.Exit(toExitCode(os.Getenv("EXIT_CODE")))
}

func toExitCode(s string) int {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		// Log the error or handle it as appropriate for a test helper
		// For now, we'll just return 1 to indicate an error during parsing
		return 1
	}
	return i
}

func TestCreatePR_NewPR(t *testing.T) {
	originalExecCommand := execCommand
	defer func() { execCommand = originalExecCommand }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		args := strings.Join(arg, " ")
		if strings.Contains(args, "pr view") {
			return mockExecCommand(t, "gh pr view", []string{"--json", "url,state"}, "", "no pull requests found", 1)(name, arg...)
		}
		if strings.Contains(args, "pr create") {
			return mockExecCommand(t, "gh pr create", []string{"--base", "main", "--head", "feature", "--title", "feat: feature", "--body-file"}, "https://github.com/owner/repo/pull/1", "", 0)(name, arg...)
		}
		return exec.Command(name, arg...)
	}

	prURL, err := CreatePR(t.TempDir(), "main", "feature", "feat: feature", "PR Body")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/repo/pull/1", prURL)
}

func TestCreatePR_OpenPRAlreadyExists(t *testing.T) {
	originalExecCommand := execCommand
	defer func() { execCommand = originalExecCommand }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		args := strings.Join(arg, " ")
		if strings.Contains(args, "pr view") {
			return mockExecCommand(t, "gh pr view", []string{"--json", "url,state"}, `{"url": "https://github.com/owner/repo/pull/99", "state": "OPEN"}`, "", 0)(name, arg...)
		}
		return exec.Command(name, arg...)
	}

	prURL, err := CreatePR(t.TempDir(), "main", "feature", "feat: feature", "PR Body")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/repo/pull/99", prURL)
}

func TestCreatePR_ClosedPRCreatesNew(t *testing.T) {
	originalExecCommand := execCommand
	defer func() { execCommand = originalExecCommand }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		args := strings.Join(arg, " ")
		if strings.Contains(args, "pr view") {
			return mockExecCommand(t, "gh pr view", []string{"--json", "url,state"}, `{"url": "https://github.com/owner/repo/pull/99", "state": "CLOSED"}`, "", 0)(name, arg...)
		}
		if strings.Contains(args, "pr create") {
			return mockExecCommand(t, "gh pr create", []string{"--base", "main", "--head", "feature"}, "https://github.com/owner/repo/pull/100", "", 0)(name, arg...)
		}
		return exec.Command(name, arg...)
	}

	prURL, err := CreatePR(t.TempDir(), "main", "feature", "feat: feature", "PR Body")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/repo/pull/100", prURL)
}

func TestCreatePR_EmptyBody(t *testing.T) {
	originalExecCommand := execCommand
	defer func() { execCommand = originalExecCommand }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		args := strings.Join(arg, " ")
		if strings.Contains(args, "pr view") {
			return mockExecCommand(t, "gh pr view", []string{"--json", "url,state"}, "", "no pull requests found", 1)(name, arg...)
		}
		if strings.Contains(args, "pr create") {
			assert.NotContains(t, args, "--body-file", "should not pass --body-file for empty body")
			return mockExecCommand(t, "gh pr create", []string{"--base", "main", "--head", "feature", "--title", "feat: feature"}, "https://github.com/owner/repo/pull/2", "", 0)(name, arg...)
		}
		return exec.Command(name, arg...)
	}

	prURL, err := CreatePR(t.TempDir(), "main", "feature", "feat: feature", "")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/repo/pull/2", prURL)
}

func TestCreatePR_ViewErrorOtherThanNoPR(t *testing.T) {
	originalExecCommand := execCommand
	defer func() { execCommand = originalExecCommand }()

	execCommand = func(name string, arg ...string) *exec.Cmd {
		args := strings.Join(arg, " ")
		if strings.Contains(args, "pr view") {
			return mockExecCommand(t, "gh pr view", []string{"--json", "url"}, "", "authentication required", 1)(name, arg...)
		}
		return exec.Command(name, arg...)
	}

	prURL, err := CreatePR(t.TempDir(), "main", "feature", "feat: feature", "PR Body")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for existing PR")
	assert.Empty(t, prURL)
}
