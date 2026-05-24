package repository

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"gopkg.in/yaml.v3"
)

var _registryPathOverride string

func SetRegistryPathOverride(path string) {
	_registryPathOverride = path
}

type VerifyConfig struct {
    Build string `yaml:"build" json:"build"`
    Test  string `yaml:"test"  json:"test"`
    Lint  string `yaml:"lint"  json:"lint"`
}

type RepositoryConfig struct {
	WorkDir      string        `yaml:"work_dir"                json:"work_dir"`
	RepoName     string        `yaml:"repo_name"               json:"repo_name"`
	VerifyConfig *VerifyConfig `yaml:"verify_config,omitempty" json:"verify_config,omitempty"`
	IsWorktree   *bool         `yaml:"is_worktree"             json:"is_worktree"`
	AgentsPath   string        `yaml:"agents_path"             json:"agents_path"`
}

func registryPath() (string, error) {
    if _registryPathOverride != "" {
        return _registryPathOverride, nil
    }
    base, err := feature.FeaturesDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(base, "repositories_config.yaml"), nil
}

func load(path string) (map[string]RepositoryConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return make(map[string]RepositoryConfig), nil
        }
        return nil, fmt.Errorf("reading repositories_config.yaml: %w", err)
    }
    var registry map[string]RepositoryConfig
    if err := yaml.Unmarshal(data, &registry); err != nil {
        return nil, fmt.Errorf("unmarshaling repositories_config.yaml: %w", err)
    }
    if registry == nil {
        return make(map[string]RepositoryConfig), nil
    }
    return registry, nil
}

func save(path string, registry map[string]RepositoryConfig) error {
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        return fmt.Errorf("creating features dir: %w", err)
    }
    data, err := yaml.Marshal(registry)
    if err != nil {
        return fmt.Errorf("marshaling repositories_config.yaml: %w", err)
    }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, 0644); err != nil {
        return fmt.Errorf("writing repositories_config.yaml.tmp: %w", err)
    }
    if err := os.Rename(tmp, path); err != nil {
        os.Remove(tmp) //nolint:errcheck
        return fmt.Errorf("renaming repositories_config.yaml.tmp: %w", err)
    }
    return nil
}

func Add(cfg RepositoryConfig, stderr io.Writer) error {
	if stderr == nil {
		stderr = os.Stderr
	}
	if cfg.RepoName == "" {
		return fmt.Errorf("repo_name is required")
	}
	parts := strings.Split(cfg.RepoName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("repo_name must be in 'org/repo' format")
	}
	if cfg.WorkDir == "" {
		return fmt.Errorf("work_dir is required")
	}
	if _, err := os.Stat(cfg.WorkDir); os.IsNotExist(err) {
		return fmt.Errorf("work_dir %q does not exist", cfg.WorkDir)
	}
	if cfg.AgentsPath == "" {
		return fmt.Errorf("agents_path is required")
	}
	if cfg.IsWorktree == nil {
		return fmt.Errorf("is_worktree is required")
	}
	if cfg.VerifyConfig == nil {
		//nolint:errcheck
		fmt.Fprintln(stderr, "warning: verify_config not provided — the implement command will not work for this repository")
	}

	p, err := registryPath()
	if err != nil {
		return err
	}

	registry, err := load(p)
	if err != nil {
		return err
	}
	if _, exists := registry[cfg.RepoName]; exists {
		return fmt.Errorf("repository %q already exists; remove it manually from repositories_config.yaml to re-add", cfg.RepoName)
	}
	registry[cfg.RepoName] = cfg
	return save(p, registry)
}

func List() (map[string]RepositoryConfig, error) {
	p, err := registryPath()
	if err != nil {
		return nil, err
	}
	return load(p)
}
