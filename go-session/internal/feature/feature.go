package feature

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daniel-talonone/gemini-commands/internal/git"
	"github.com/daniel-talonone/gemini-commands/internal/log"
	"github.com/daniel-talonone/gemini-commands/internal/pr"
	"github.com/daniel-talonone/gemini-commands/internal/review"
	"github.com/daniel-talonone/gemini-commands/internal/status"
)

// CreateFeature creates a feature directory with placeholder files.
// repo, branch, and workDir are written into status.yaml if provided; pass "" to leave them empty.
// Idempotent: succeeds if the directory already exists and never overwrites existing files.
func CreateFeature(featureDir, repo, branch, workDir string) error {
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		return fmt.Errorf("creating feature directory: %w", err)
	}

	if err := status.Create(featureDir, repo, branch, workDir, "", ""); err != nil {
		return fmt.Errorf("creating status.yaml: %w", err)
	}

	files := map[string]string{
		"plan.yml":      "[]",
		"questions.yml": "[]",
	}

	for name, content := range files {
		path := filepath.Join(featureDir, name)
		if _, err := os.Stat(path); err == nil {
			continue // file already exists — never overwrite live data
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	if err := review.Create(featureDir, review.TypeDefault); err != nil {
		return fmt.Errorf("creating review.yml: %w", err)
	}

	if err := pr.Create(featureDir); err != nil {
		return fmt.Errorf("creating pr.md: %w", err)
	}

	if err := log.CreateLogFile(featureDir); err != nil {
		return fmt.Errorf("writing log: %w", err)
	}
	return nil
}

// ResolveFeatureDir resolves the full path to a feature directory.
// cwd is used for the local .features/ backward-compat check.
// remoteURL is the git remote origin URL (pass "" if not in a git repo).
func ResolveFeatureDir(storyID, cwd, remoteURL string) (string, error) {
	return ResolveFeatureDirImpl(storyID, cwd, remoteURL)
}

var ResolveFeatureDirImpl = func(storyID, cwd, remoteURL string) (string, error) {
	if strings.Contains(storyID, "/") ||
		strings.HasPrefix(storyID, ".") ||
		strings.HasPrefix(storyID, "~") {
		return storyID, nil
	}

	localDir := filepath.Join(cwd, ".features", storyID)
	if info, err := os.Stat(localDir); err == nil && info.IsDir() {
		return localDir, nil
	}

	if remoteURL == "" {
		return "", fmt.Errorf(
			"cannot resolve %q: not an explicit path, no local .features/%s directory found, "+
				"and no git remote URL provided — run from inside a git repository or pass a full path",
			storyID, storyID,
		)
	}

	orgRepo := git.ParseOrgRepo(remoteURL)
	if orgRepo == "" {
		return "", fmt.Errorf("cannot parse org/repo from remote URL: %s", remoteURL)
	}

	base, err := FeaturesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, orgRepo, storyID), nil
}

// FeaturesDir returns the root directory where all feature directories are stored.
// This is the single source of truth for the features base path.
func FeaturesDir() (string, error) {
	if envDir := os.Getenv("AI_SESSION_FEATURES_DIR"); envDir != "" {
		return envDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".features"), nil
}

// LoadContext reads all .md, .yml, .yaml files from featureDir (excluding _* files),
// sorts them alphabetically, and returns them formatted as XML blocks for LLM consumption.
//
// Output format:
//
//	<file name="description.md">
//	...content...
//	</file>
//
//	<file name="plan.yml">
//	...content...
//	</file>
func LoadContext(featureDir string) (string, error) {
	if _, err := os.Stat(featureDir); os.IsNotExist(err) {
		return "", fmt.Errorf("feature directory does not exist: %s", featureDir)
	}

	entries, err := os.ReadDir(featureDir)
	if err != nil {
		return "", fmt.Errorf("reading feature directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}
		ext := filepath.Ext(name)
		if ext != ".md" && ext != ".yml" && ext != ".yaml" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var blocks []string
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(featureDir, name))
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", name, err)
		}
		blocks = append(blocks, fmt.Sprintf("<file name=%q>\n%s\n</file>", name, content))
	}

	return strings.Join(blocks, "\n\n"), nil
}
