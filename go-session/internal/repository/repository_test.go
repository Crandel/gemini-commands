package repository

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestAdd(t *testing.T) {
	// Create a temporary directory for valid work_dir tests
	tmpDir, err := os.MkdirTemp("", "test-workdir-*")
	assert.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		assert.NoError(t, err)
	}()

	tests := []struct {
		name         string
		config       RepositoryConfig
		expectedErr  string
		expectWarn   bool
		preAddConfig *RepositoryConfig // For duplicate test cases
	}{
		{
			name: "Add valid config with verify_config",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "org/repo1",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents1",
				VerifyConfig: &VerifyConfig{
					Build: "make build",
					Test:  "make test",
					Lint:  "make lint",
				},
			},
			expectedErr: "",
			expectWarn:  false,
		},
		{
			name: "Add valid config without verify_config",
			config: RepositoryConfig{
				WorkDir:      tmpDir,
				RepoName:     "org/repo2",
				IsWorktree:   boolPtr(true),
				AgentsPath:   "/path/to/agents2",
				VerifyConfig: nil,
			},
			expectedErr: "",   // The Add function will handle the warning, but it's not an error.
			expectWarn:  true, // Expect warning for missing verify_config
		},
		{
			name: "Add duplicate repo_name",
			preAddConfig: &RepositoryConfig{ // This config will be added first
				WorkDir:    tmpDir,
				RepoName:   "org/repoX",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agentsX",
				VerifyConfig: &VerifyConfig{
					Build: "make build",
					Test:  "make test",
					Lint:  "make lint",
				},
			},
			config: RepositoryConfig{ // This config will be the duplicate
				WorkDir:    tmpDir,
				RepoName:   "org/repoX",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agentsX",
				VerifyConfig: &VerifyConfig{
					Build: "make build",
					Test:  "make test",
					Lint:  "make lint",
				},
			},
			expectedErr: `repository "org/repoX" already exists; remove it manually from repositories_config.yaml to re-add`,
			expectWarn:  false,
		},
		{
			name: "Add with empty repo_name",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents3",
			},
			expectedErr: "repo_name is required",
			expectWarn:  false,
		},
		{
			name: "Add with empty work_dir",
			config: RepositoryConfig{
				WorkDir:    "",
				RepoName:   "org/repo4",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents4",
			},
			expectedErr: "work_dir is required",
			expectWarn:  false,
		},
		{
			name: "Add with empty agents_path",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "org/repo5",
				IsWorktree: boolPtr(false),
				AgentsPath: "",
			},
			expectedErr: "agents_path is required",
			expectWarn:  false,
		},
		{
			name: "Add with invalid repo_name format (no slash)",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "myrepo",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents6",
			},
			expectedErr: "repo_name must be in 'org/repo' format",
			expectWarn:  false,
		},
		{
			name: "Add with invalid repo_name format (trailing slash)",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "org/",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents7",
			},
			expectedErr: "repo_name must be in 'org/repo' format",
			expectWarn:  false,
		},
		{
			name: "Add with invalid repo_name format (leading slash)",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "/repo",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents8",
			},
			expectedErr: "repo_name must be in 'org/repo' format",
			expectWarn:  false,
		},
		{
			name: "Add with invalid repo_name format (multiple slashes)",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "org/repo/extra",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents9",
			},
			expectedErr: "repo_name must be in 'org/repo' format",
			expectWarn:  false,
		},
		{
			name: "Add with nil is_worktree",
			config: RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "org/repo10",
				IsWorktree: nil,
				AgentsPath: "/path/to/agents10",
			},
			expectedErr: "is_worktree is required",
			expectWarn:  false,
		},
		{
			name: "Add with non-existent work_dir",
			config: RepositoryConfig{
				WorkDir:    "/path/to/non-existent-dir",
				RepoName:   "org/repo11",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents11",
			},
			expectedErr: `work_dir "/path/to/non-existent-dir" does not exist`,
			expectWarn:  false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			// Set up isolated registry path for each test
			SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
			t.Cleanup(func() { SetRegistryPathOverride("") }) // Clear override after test

			if tt.preAddConfig != nil {
				// Add the prerequisite config for duplicate test
				err := Add(*tt.preAddConfig, io.Discard)
				assert.NoError(t, err, "pre-add should not fail")
				// Verify pre-add was successful for duplicate setup
				p, err := registryPath()
				assert.NoError(t, err)
				registry, loadErr := load(p)
				assert.NoError(t, loadErr, "loading registry after pre-add failed")
				assert.Contains(t, registry, tt.preAddConfig.RepoName, "pre-added repo not found in registry")
			}

			var stderrBuf bytes.Buffer
			addErr := Add(tt.config, &stderrBuf)

			if tt.expectedErr != "" {
				assert.ErrorContains(t, addErr, tt.expectedErr)
			} else {
				assert.NoError(t, addErr)
			}

			capturedStderr := strings.TrimSpace(stderrBuf.String())
			if tt.expectWarn {
				assert.Contains(t, capturedStderr, "warning: verify_config not provided — the implement command will not work for this repository")
			} else {
				assert.Empty(t, capturedStderr)
			}

			if tt.expectedErr == "" {
				// Verify the repository was added only if no error is expected
				p, err := registryPath()
				assert.NoError(t, err)
				registry, loadErr := load(p)
				assert.NoError(t, loadErr)
				assert.Contains(t, registry, tt.config.RepoName)
				assert.Equal(t, tt.config, registry[tt.config.RepoName])
			}
		})
	}
}

func TestList(t *testing.T) {
	// Create a temporary directory for valid work_dir tests
	tmpDir, err := os.MkdirTemp("", "test-workdir-*")
	assert.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		assert.NoError(t, err)
	}()

	tests := []struct {
		name          string
		setup         func(t *testing.T)
		expectedLen   int
		expectedRepos map[string]RepositoryConfig
		expectedErr   string
	}{
		{
			name: "List with no registry file",
			setup: func(t *testing.T) {
				// Ensure no registry file exists
				SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
				t.Cleanup(func() { SetRegistryPathOverride("") })
			},
			expectedLen:   0,
			expectedRepos: make(map[string]RepositoryConfig),
			expectedErr:   "",
		},
		{
			name: "List after two Adds",
			setup: func(t *testing.T) {
				// Set up isolated registry path
				SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
				t.Cleanup(func() { SetRegistryPathOverride("") })

				// Add two repositories
				repo1 := RepositoryConfig{
					WorkDir:      tmpDir,
					RepoName:     "org/repoA",
					IsWorktree:   boolPtr(false),
					AgentsPath:   "/path/to/agentsA",
					VerifyConfig: &VerifyConfig{Build: "buildA", Test: "testA", Lint: "lintA"},
				}
				repo2 := RepositoryConfig{
					WorkDir:    tmpDir,
					RepoName:   "org/repoB",
					IsWorktree: boolPtr(true),
					AgentsPath: "/path/to/agentsB",
				}
				err := Add(repo1, io.Discard)
				assert.NoError(t, err)
				err = Add(repo2, io.Discard)
				assert.NoError(t, err)
			},
			expectedLen: 2,
			expectedRepos: map[string]RepositoryConfig{
				"org/repoA": {
					WorkDir:      tmpDir,
					RepoName:     "org/repoA",
					IsWorktree:   boolPtr(false),
					AgentsPath:   "/path/to/agentsA",
					VerifyConfig: &VerifyConfig{Build: "buildA", Test: "testA", Lint: "lintA"},
				},
				"org/repoB": {
					WorkDir:      tmpDir,
					RepoName:     "org/repoB",
					IsWorktree:   boolPtr(true),
					AgentsPath:   "/path/to/agentsB",
					VerifyConfig: nil, // Should be nil as it was added without verify_config
				},
			},
			expectedErr: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			repos, err := List()

			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
				assert.Nil(t, repos)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, repos)
				assert.Len(t, repos, tt.expectedLen)
				assert.Equal(t, tt.expectedRepos, repos)
			}
		})
	}
}

func TestGet(t *testing.T) {
	// Create a temporary directory for valid work_dir tests
	tmpDir, err := os.MkdirTemp("", "test-workdir-*")
	assert.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		assert.NoError(t, err)
	}()

	tests := []struct {
		name            string
		setup           func(t *testing.T)
		repoName        string
		expectedConfig  *RepositoryConfig
		expectedErr     bool
	}{
		{
			name: "Get existing repo with verify_config",
			setup: func(t *testing.T) {
				SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
				t.Cleanup(func() { SetRegistryPathOverride("") })

				config := RepositoryConfig{
					WorkDir:    tmpDir,
					RepoName:   "org/test-repo",
					IsWorktree: boolPtr(false),
					AgentsPath: "/path/to/agents",
					VerifyConfig: &VerifyConfig{
						Build: "make build",
						Test:  "make test",
						Lint:  "make lint",
					},
				}
				err := Add(config, io.Discard)
				assert.NoError(t, err)
			},
			repoName: "org/test-repo",
			expectedConfig: &RepositoryConfig{
				WorkDir:    tmpDir,
				RepoName:   "org/test-repo",
				IsWorktree: boolPtr(false),
				AgentsPath: "/path/to/agents",
				VerifyConfig: &VerifyConfig{
					Build: "make build",
					Test:  "make test",
					Lint:  "make lint",
				},
			},
			expectedErr: false,
		},
		{
			name: "Get existing repo without verify_config",
			setup: func(t *testing.T) {
				SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
				t.Cleanup(func() { SetRegistryPathOverride("") })

				config := RepositoryConfig{
					WorkDir:      tmpDir,
					RepoName:     "org/another-repo",
					IsWorktree:   boolPtr(true),
					AgentsPath:   "/path/to/agents",
					VerifyConfig: nil,
				}
				err := Add(config, io.Discard)
				assert.NoError(t, err)
			},
			repoName: "org/another-repo",
			expectedConfig: &RepositoryConfig{
				WorkDir:      tmpDir,
				RepoName:     "org/another-repo",
				IsWorktree:   boolPtr(true),
				AgentsPath:   "/path/to/agents",
				VerifyConfig: nil,
			},
			expectedErr: false,
		},
		{
			name: "Get missing repo returns nil, nil",
			setup: func(t *testing.T) {
				SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
				t.Cleanup(func() { SetRegistryPathOverride("") })
			},
			repoName:       "org/nonexistent-repo",
			expectedConfig: nil,
			expectedErr:    false,
		},
		{
			name: "Get from empty registry returns nil, nil",
			setup: func(t *testing.T) {
				SetRegistryPathOverride(filepath.Join(t.TempDir(), "repositories_config.yaml"))
				t.Cleanup(func() { SetRegistryPathOverride("") })

				config := RepositoryConfig{
					WorkDir:    tmpDir,
					RepoName:   "org/some-repo",
					IsWorktree: boolPtr(false),
					AgentsPath: "/path/to/agents",
				}
				err := Add(config, io.Discard)
				assert.NoError(t, err)
			},
			repoName:       "org/different-repo",
			expectedConfig: nil,
			expectedErr:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			config, err := Get(tt.repoName)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedConfig != nil {
				assert.NotNil(t, config)
				assert.Equal(t, *tt.expectedConfig, *config)
			} else {
				assert.Nil(t, config)
			}
		})
	}
}
