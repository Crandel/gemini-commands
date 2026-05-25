package implement_test

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/implement"
	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/daniel-talonone/gemini-commands/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRunner is a no-op Runner for use in tests (IN_TEST_MODE bypasses the
// actual Run call, so this is never invoked).
type testRunner struct{}

func (r *testRunner) Run(_ io.Reader, _, _ io.Writer) error { return nil }

// perSliceTestEnv holds the directories created by setupPerSliceTest.
// verifyScript is the path to the verification shell script; overwrite its
// content before calling implement.Run to change verification behaviour.
type perSliceTestEnv struct {
	featureDir    string
	projectDir    string
	aiSessionHome string
	verifyScript  string
	logger        *slog.Logger
}

// setupPerSliceTest creates a minimal test environment for PerSliceStrategy
// tests. The verification script defaults to always-success (exit 0).
func setupPerSliceTest(t *testing.T, planYML string) perSliceTestEnv {
	t.Helper()
	t.Setenv("IN_TEST_MODE", "true")

	// Verification script — tests can overwrite the content to change behaviour.
	scriptsDir := t.TempDir()
	verifyScript := filepath.Join(scriptsDir, "verify.sh")
	require.NoError(t, os.WriteFile(verifyScript, []byte(`#!/bin/bash
exit 0`), 0755))

	// projectDir holds AGENTS.md which points at the verify script.
	projectDir := t.TempDir()
	agentsMD := "## Verification\nRun: " + verifyScript + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), []byte(agentsMD), 0644))

	// aiSessionHome needs execute_slice.md with all placeholder tokens.
	aiSessionHome := t.TempDir()
	headlessDir := filepath.Join(aiSessionHome, "headless", "session")
	require.NoError(t, os.MkdirAll(headlessDir, 0755))
	sliceMD := "{{story_description_here}} {{architecture_description_here}} {{slice_description_here}} {{tasks_here}} {{changes_so_far_here}} {{verification_command_here}} {{feature_dir_here}}"
	require.NoError(t, os.WriteFile(filepath.Join(headlessDir, "execute_slice.md"), []byte(sliceMD), 0644))
	// execute_task.md is required by PerTaskStrategy (used in TestRun).
	require.NoError(t, os.WriteFile(filepath.Join(headlessDir, "execute_task.md"), []byte("prompt"), 0644))

	// featureDir holds the feature files.
	featureDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "description.md"), []byte("Test story"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "architecture.md"), []byte("Test arch"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "status.yaml"), []byte("pipeline_step: plan-done\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "plan.yml"), []byte(planYML), 0644))

	return perSliceTestEnv{
		featureDir:    featureDir,
		projectDir:    projectDir,
		aiSessionHome: aiSessionHome,
		verifyScript:  verifyScript,
		logger:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

func TestRun(t *testing.T) {
	t.Setenv("IN_TEST_MODE", "true")

	planYML := `- id: test-slice
  description: "Test slice"
  status: todo
  depends_on: []
  tasks:
    - id: test-task
      task: "Do something"
      status: todo
    - id: done-task
      task: "Already done"
      status: done
`
	env := setupPerSliceTest(t, planYML)

	err := implement.Run(env.logger, "test-feature", env.featureDir, env.projectDir, env.aiSessionHome, 3, 0, &implement.PerTaskStrategy{}, &testRunner{})
	require.NoError(t, err)

	// plan.yml: test-task and test-slice must be marked done.
	planBytes, err := os.ReadFile(filepath.Join(env.featureDir, "plan.yml"))
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(planBytes), "status: done"))

	// log.md must record completion.
	logBytes, err := os.ReadFile(filepath.Join(env.featureDir, "log.md"))
	require.NoError(t, err)
	assert.Contains(t, string(logBytes), "IMPLEMENT COMPLETE")

	// status.yaml must be updated to implement-done.
	statusBytes, err := os.ReadFile(filepath.Join(env.featureDir, "status.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(statusBytes), "implement-done")
}

func TestExtractVerificationCommand(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		tempDir := t.TempDir()
		agentsMD := `## Verification
Run: echo 'hello world'
`
		agentsPath := filepath.Join(tempDir, "AGENTS.md")
		require.NoError(t, os.WriteFile(agentsPath, []byte(agentsMD), 0644))

		cmd, err := implement.ExtractVerificationCommand(agentsPath)
		require.NoError(t, err)
		assert.Equal(t, "echo 'hello world'", cmd)
	})

	t.Run("missing AGENTS.md", func(t *testing.T) {
		tempDir := t.TempDir()
		agentsPath := filepath.Join(tempDir, "AGENTS.md")
		_, err := implement.ExtractVerificationCommand(agentsPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading AGENTS.md")
	})

	t.Run("missing verification section", func(t *testing.T) {
		tempDir := t.TempDir()
		agentsMD := `## Some Other Section
Content here.
`
		agentsPath := filepath.Join(tempDir, "AGENTS.md")
		require.NoError(t, os.WriteFile(agentsPath, []byte(agentsMD), 0644))

		_, err := implement.ExtractVerificationCommand(agentsPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verification command not found")
	})
}

func TestPerSliceStrategy_ExecuteSlice_Success(t *testing.T) {
	planYML := `- id: test-slice-success
  description: "Test slice success"
  status: todo
  depends_on: []
  tasks:
    - id: task-1
      task: "Complete this task"
      status: todo
`
	env := setupPerSliceTest(t, planYML)

	// Simulate Gemini marking the task done (IN_TEST_MODE skips the real call).
	require.NoError(t, plan.UpdateTask(env.featureDir, "task-1", "done"))

	err := implement.Run(env.logger, "test-feature-success", env.featureDir, env.projectDir, env.aiSessionHome, 1, 0, &implement.PerSliceStrategy{}, &testRunner{})
	require.NoError(t, err)

	// Slice and task must be marked done.
	p, err := plan.LoadPlan(env.featureDir)
	require.NoError(t, err)
	slice, found := p.FindSlice("test-slice-success")
	require.True(t, found)
	assert.Equal(t, "done", slice.Status)
	assert.Equal(t, "done", slice.Tasks[0].Status)

	logBytes, err := os.ReadFile(filepath.Join(env.featureDir, "log.md"))
	require.NoError(t, err)
	assert.Contains(t, string(logBytes), "Slice test-slice-success: all gates passed (attempt 1).")
	assert.Contains(t, string(logBytes), "--- IMPLEMENT COMPLETE ---")

	statusBytes, err := os.ReadFile(filepath.Join(env.featureDir, "status.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(statusBytes), "implement-done")
}

func TestPerSliceStrategy_ExecuteSlice_Gate1RetryFails(t *testing.T) {
	planYML := `- id: test-slice-gate1-fail
  description: "Test slice Gate 1 fail"
  status: todo
  depends_on: []
  tasks:
    - id: task-g1-1
      task: "Complete this task"
      status: todo
`
	env := setupPerSliceTest(t, planYML)
	// Tasks never get marked done — Gate 1 fails every attempt.

	err := implement.Run(env.logger, "test-feature-gate1-fail", env.featureDir, env.projectDir, env.aiSessionHome, 2, 0, &implement.PerSliceStrategy{}, &testRunner{})
	require.Error(t, err)

	p, err := plan.LoadPlan(env.featureDir)
	require.NoError(t, err)
	slice, found := p.FindSlice("test-slice-gate1-fail")
	require.True(t, found)
	assert.Equal(t, "in-progress", slice.Status)
	assert.Equal(t, "todo", slice.Tasks[0].Status)

	logBytes, err := os.ReadFile(filepath.Join(env.featureDir, "log.md"))
	require.NoError(t, err)
	assert.Contains(t, string(logBytes), "Gate 1 failed for slice test-slice-gate1-fail (attempt 1)")
	assert.Contains(t, string(logBytes), "Gate 1 failed for slice test-slice-gate1-fail (attempt 2)")

	statusBytes, err := os.ReadFile(filepath.Join(env.featureDir, "status.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(statusBytes), "implement-failed")
}

func TestPerSliceStrategy_ExecuteSlice_Gate2RetrySucceeds(t *testing.T) {
	planYML := `- id: test-slice-gate2-retry
  description: "Test slice Gate 2 retry"
  status: todo
  depends_on: []
  tasks:
    - id: task-g2-1
      task: "Complete this task"
      status: todo
`
	env := setupPerSliceTest(t, planYML)

	// Tasks pre-marked done so Gate 1 always passes.
	require.NoError(t, plan.UpdateTask(env.featureDir, "task-g2-1", "done"))

	// Script call sequence: initial gate (pass), Gate 2 attempt 1 (fail), Gate 2 attempt 2 (pass).
	counterFile := filepath.Join(t.TempDir(), "count")
	require.NoError(t, os.WriteFile(env.verifyScript, []byte(`#!/bin/bash
COUNT=$(cat "`+counterFile+`" 2>/dev/null || echo 0)
COUNT=$((COUNT + 1))
echo "$COUNT" > "`+counterFile+`"
[ "$COUNT" -le 1 ] && exit 0   # initial gate: pass
[ "$COUNT" -le 2 ] && exit 1   # Gate 2 attempt 1: fail
exit 0                          # Gate 2 attempt 2: pass`), 0755))

	err := implement.Run(env.logger, "test-feature-gate2-retry", env.featureDir, env.projectDir, env.aiSessionHome, 3, 0, &implement.PerSliceStrategy{}, &testRunner{})
	require.NoError(t, err)

	logBytes, err := os.ReadFile(filepath.Join(env.featureDir, "log.md"))
	require.NoError(t, err)
	assert.Contains(t, string(logBytes), "Gate 2 failed for slice test-slice-gate2-retry (attempt 1)")
	assert.Contains(t, string(logBytes), "Slice test-slice-gate2-retry: all gates passed (attempt 2).")
}

func TestPerSliceStrategy_ExecuteSlice_Gate2ExhaustedRetries(t *testing.T) {
	planYML := `- id: test-slice-gate2-exhausted
  description: "Test slice Gate 2 exhausted"
  status: todo
  depends_on: []
  tasks:
    - id: task-g2ex-1
      task: "Complete this task"
      status: todo
`
	env := setupPerSliceTest(t, planYML)

	// Tasks pre-marked done so Gate 1 always passes.
	// Script: initial gate passes, all Gate 2 attempts fail.
	require.NoError(t, plan.UpdateTask(env.featureDir, "task-g2ex-1", "done"))
	counterFile := filepath.Join(t.TempDir(), "count")
	require.NoError(t, os.WriteFile(env.verifyScript, []byte(`#!/bin/bash
COUNT=$(cat "`+counterFile+`" 2>/dev/null || echo 0)
COUNT=$((COUNT + 1))
echo "$COUNT" > "`+counterFile+`"
[ "$COUNT" -le 1 ] && exit 0  # initial gate: pass
exit 1                         # Gate 2: always fail`), 0755))

	err := implement.Run(env.logger, "test-feature-gate2-exhausted", env.featureDir, env.projectDir, env.aiSessionHome, 2, 0, &implement.PerSliceStrategy{}, &testRunner{})
	require.Error(t, err)

	logBytes, err := os.ReadFile(filepath.Join(env.featureDir, "log.md"))
	require.NoError(t, err)
	assert.Contains(t, string(logBytes), "Gate 2 failed for slice test-slice-gate2-exhausted (attempt 1)")
	assert.Contains(t, string(logBytes), "Gate 2 failed for slice test-slice-gate2-exhausted (attempt 2)")

	statusBytes, err := os.ReadFile(filepath.Join(env.featureDir, "status.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(statusBytes), "implement-failed")
}

func TestImplementRun_InitialVerificationFails(t *testing.T) {
	planYML := `- id: test-slice-initial-fail
  description: "Test slice initial fail"
  status: todo
  depends_on: []
  tasks:
    - id: task-initial-1
      task: "Complete this task"
      status: todo
`
	env := setupPerSliceTest(t, planYML)

	// Make initial verification fail before any slice runs.
	require.NoError(t, os.WriteFile(env.verifyScript, []byte(`#!/bin/bash
exit 1`), 0755))

	err := implement.Run(env.logger, "test-feature-initial-fail", env.featureDir, env.projectDir, env.aiSessionHome, 1, 0, &implement.PerSliceStrategy{}, &testRunner{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Initial verification failed")

	// Slice and task must remain untouched.
	p, err := plan.LoadPlan(env.featureDir)
	require.NoError(t, err)
	slice, found := p.FindSlice("test-slice-initial-fail")
	require.True(t, found)
	assert.Equal(t, "todo", slice.Status)
	assert.Equal(t, "todo", slice.Tasks[0].Status)

	statusBytes, err := os.ReadFile(filepath.Join(env.featureDir, "status.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(statusBytes), "implement-failed")
}

func TestResolveAgentsPath(t *testing.T) {
	t.Run("repo config agents_path exists - return that path", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoAgentsPath := filepath.Join(tmpDir, "repo-agents.md")
		require.NoError(t, os.WriteFile(repoAgentsPath, []byte("repo agents"), 0644))

		workDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("workdir agents"), 0644))

		repoConfig := &repository.RepositoryConfig{RepoName: "org/test-repo", AgentsPath: repoAgentsPath}
		assert.Equal(t, repoAgentsPath, implement.ResolveAgentsPath(workDir, repoConfig))
	})

	t.Run("repo config agents_path does not exist - fallback to workDir/AGENTS.md", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoAgentsPath := filepath.Join(tmpDir, "nonexistent-agents.md")

		workDir := t.TempDir()
		workDirAgentsPath := filepath.Join(workDir, "AGENTS.md")
		require.NoError(t, os.WriteFile(workDirAgentsPath, []byte("workdir agents"), 0644))

		repoConfig := &repository.RepositoryConfig{RepoName: "org/test-repo", AgentsPath: repoAgentsPath}
		assert.Equal(t, workDirAgentsPath, implement.ResolveAgentsPath(workDir, repoConfig))
	})

	t.Run("repo config agents_path is empty string - fallback to workDir/AGENTS.md", func(t *testing.T) {
		workDir := t.TempDir()
		workDirAgentsPath := filepath.Join(workDir, "AGENTS.md")
		require.NoError(t, os.WriteFile(workDirAgentsPath, []byte("workdir agents"), 0644))

		repoConfig := &repository.RepositoryConfig{RepoName: "org/test-repo", AgentsPath: ""}
		assert.Equal(t, workDirAgentsPath, implement.ResolveAgentsPath(workDir, repoConfig))
	})

	t.Run("no repo config (nil) - fallback to workDir/AGENTS.md", func(t *testing.T) {
		workDir := t.TempDir()
		workDirAgentsPath := filepath.Join(workDir, "AGENTS.md")
		require.NoError(t, os.WriteFile(workDirAgentsPath, []byte("workdir agents"), 0644))

		assert.Equal(t, workDirAgentsPath, implement.ResolveAgentsPath(workDir, nil))
	})

	t.Run("workDir/AGENTS.md does not exist - return fallback path anyway", func(t *testing.T) {
		workDir := t.TempDir()
		assert.Equal(t, filepath.Join(workDir, "AGENTS.md"), implement.ResolveAgentsPath(workDir, nil))
	})

	t.Run("repo config agents_path and workDir AGENTS.md both exist - prefer repo config", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoAgentsPath := filepath.Join(tmpDir, "repo-agents.md")
		require.NoError(t, os.WriteFile(repoAgentsPath, []byte("repo agents"), 0644))

		workDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("workdir agents"), 0644))

		repoConfig := &repository.RepositoryConfig{RepoName: "org/test-repo", AgentsPath: repoAgentsPath}
		assert.Equal(t, repoAgentsPath, implement.ResolveAgentsPath(workDir, repoConfig))
	})
}

func TestFormatVerificationCommands(t *testing.T) {
	t.Run("formats as Build: Test: Lint: with newlines", func(t *testing.T) {
		verifyConfig := &repository.VerifyConfig{
			Build: "make build",
			Test:  "make test",
			Lint:  "make lint",
		}
		assert.Equal(t, "Build: make build\nTest: make test\nLint: make lint", implement.FormatVerificationCommands(verifyConfig))
	})

	t.Run("nil verify_config returns empty string", func(t *testing.T) {
		assert.Equal(t, "", implement.FormatVerificationCommands(nil))
	})

	t.Run("empty strings in verify_config are formatted", func(t *testing.T) {
		verifyConfig := &repository.VerifyConfig{Build: "", Test: "", Lint: ""}
		assert.Equal(t, "Build: \nTest: \nLint: ", implement.FormatVerificationCommands(verifyConfig))
	})

	t.Run("complex commands with special characters are formatted", func(t *testing.T) {
		verifyConfig := &repository.VerifyConfig{
			Build: "go build ./... && echo 'build complete'",
			Test:  "go test -v -race ./... | grep -E 'PASS|FAIL'",
			Lint:  "golangci-lint run --timeout=5m .",
		}
		expected := "Build: go build ./... && echo 'build complete'\nTest: go test -v -race ./... | grep -E 'PASS|FAIL'\nLint: golangci-lint run --timeout=5m ."
		assert.Equal(t, expected, implement.FormatVerificationCommands(verifyConfig))
	})
}

func TestExtractContextPattern(t *testing.T) {
	t.Run("reads patterns from specified file path", func(t *testing.T) {
		tempDir := t.TempDir()
		agentsMD := `## Context files
Pattern: *.go **/*.ts
`
		agentsPath := filepath.Join(tempDir, "AGENTS.md")
		require.NoError(t, os.WriteFile(agentsPath, []byte(agentsMD), 0644))

		patterns := implement.ExtractContextPattern(agentsPath)
		assert.Equal(t, []string{"*.go", "**/*.ts"}, patterns)
	})

	t.Run("handles missing file gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		agentsPath := filepath.Join(tempDir, "nonexistent-AGENTS.md")

		patterns := implement.ExtractContextPattern(agentsPath)
		assert.Nil(t, patterns)
	})

	t.Run("returns parsed patterns", func(t *testing.T) {
		tempDir := t.TempDir()
		agentsMD := `## Context files
Pattern: src/**/*.go cmd/**/*.go
`
		agentsPath := filepath.Join(tempDir, "AGENTS.md")
		require.NoError(t, os.WriteFile(agentsPath, []byte(agentsMD), 0644))

		patterns := implement.ExtractContextPattern(agentsPath)
		assert.Equal(t, []string{"src/**/*.go", "cmd/**/*.go"}, patterns)
	})
}

func TestRunVerify(t *testing.T) {
	t.Run("executes all three commands successfully in sequence", func(t *testing.T) {
		workDir := t.TempDir()
		verifyConfig := &repository.VerifyConfig{
			Build: "echo 'build ok'",
			Test:  "echo 'test ok'",
			Lint:  "echo 'lint ok'",
		}

		err := implement.RunVerify(workDir, verifyConfig)
		assert.NoError(t, err)
	})

	t.Run("returns error when build fails with step name and output", func(t *testing.T) {
		workDir := t.TempDir()
		verifyConfig := &repository.VerifyConfig{
			Build: "exit 1",
			Test:  "echo 'test ok'",
			Lint:  "echo 'lint ok'",
		}

		err := implement.RunVerify(workDir, verifyConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Build failed")
	})

	t.Run("returns error when test fails with step name and output", func(t *testing.T) {
		workDir := t.TempDir()
		verifyConfig := &repository.VerifyConfig{
			Build: "echo 'build ok'",
			Test:  "exit 1",
			Lint:  "echo 'lint ok'",
		}

		err := implement.RunVerify(workDir, verifyConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Test failed")
	})

	t.Run("returns error when lint fails with step name and output", func(t *testing.T) {
		workDir := t.TempDir()
		verifyConfig := &repository.VerifyConfig{
			Build: "echo 'build ok'",
			Test:  "echo 'test ok'",
			Lint:  "exit 1",
		}

		err := implement.RunVerify(workDir, verifyConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Lint failed")
	})

	t.Run("fails early on build failure, does not run test or lint", func(t *testing.T) {
		workDir := t.TempDir()
		testFile := filepath.Join(workDir, "test_marker.txt")
		verifyConfig := &repository.VerifyConfig{
			Build: "exit 1",
			Test:  fmt.Sprintf("echo 'should not run' > %s", testFile),
			Lint:  fmt.Sprintf("echo 'should not run' > %s", testFile),
		}

		err := implement.RunVerify(workDir, verifyConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Build failed")

		// Verify that test and lint did not run
		_, err = os.Stat(testFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("fails early on test failure, does not run lint", func(t *testing.T) {
		workDir := t.TempDir()
		lintFile := filepath.Join(workDir, "lint_marker.txt")
		verifyConfig := &repository.VerifyConfig{
			Build: "echo 'build ok'",
			Test:  "exit 1",
			Lint:  fmt.Sprintf("echo 'should not run' > %s", lintFile),
		}

		err := implement.RunVerify(workDir, verifyConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Test failed")

		_, statErr := os.Stat(lintFile)
		assert.True(t, os.IsNotExist(statErr), "lint must not run when test fails")
	})

	t.Run("returns error when verify config is nil", func(t *testing.T) {
		err := implement.RunVerify(t.TempDir(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verify config is nil")
	})

	t.Run("captures output and includes step label in error on failure", func(t *testing.T) {
		workDir := t.TempDir()
		verifyConfig := &repository.VerifyConfig{
			Build: "echo 'build error output'; exit 1",
			Test:  "echo 'test ok'",
			Lint:  "echo 'lint ok'",
		}

		err := implement.RunVerify(workDir, verifyConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Build failed")
		assert.Contains(t, err.Error(), "build error output")
	})
}

// TestRun_VerifyConfigRouting verifies that Run() routes to RunVerify when the
// repository config has a VerifyConfig, and falls back to AGENTS.md otherwise.
func TestRun_VerifyConfigRouting(t *testing.T) {
	t.Setenv("IN_TEST_MODE", "true")

	planYML := `- id: test-slice
  description: "Test slice"
  status: todo
  depends_on: []
  tasks:
    - id: test-task
      task: "Do something"
      status: done
`

	t.Run("uses RunVerify when repo config has VerifyConfig", func(t *testing.T) {
		env := setupPerSliceTest(t, planYML)

		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })

		isWorktree := false
		err := repository.Add(repository.RepositoryConfig{
			RepoName:   "org/routing-test",
			WorkDir:    env.projectDir,
			IsWorktree: &isWorktree,
			AgentsPath: filepath.Join(env.projectDir, "AGENTS.md"),
			VerifyConfig: &repository.VerifyConfig{
				Build: "echo 'build ok'",
				Test:  "echo 'test ok'",
				Lint:  "echo 'lint ok'",
			},
		}, io.Discard)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(env.featureDir, "status.yaml"),
			[]byte("repo: org/routing-test\npipeline_step: plan-done\n"), 0644))

		err = implement.Run(env.logger, "test-routing", env.featureDir, env.projectDir, env.aiSessionHome, 1, 0, &implement.PerTaskStrategy{}, &testRunner{})
		require.NoError(t, err)

		logBytes, err := os.ReadFile(filepath.Join(env.featureDir, "log.md"))
		require.NoError(t, err)
		assert.Contains(t, string(logBytes), "Build: echo 'build ok'")
	})

	t.Run("falls back to AGENTS.md when repo not in registry", func(t *testing.T) {
		env := setupPerSliceTest(t, planYML)

		repository.SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
		t.Cleanup(func() { repository.SetRegistryPathOverride("") })

		require.NoError(t, os.WriteFile(filepath.Join(env.featureDir, "status.yaml"),
			[]byte("repo: org/unknown-repo\npipeline_step: plan-done\n"), 0644))

		err := implement.Run(env.logger, "test-fallback", env.featureDir, env.projectDir, env.aiSessionHome, 1, 0, &implement.PerTaskStrategy{}, &testRunner{})
		require.NoError(t, err)

		logBytes, err := os.ReadFile(filepath.Join(env.featureDir, "log.md"))
		require.NoError(t, err)
		assert.NotContains(t, string(logBytes), "Build:")
	})
}
