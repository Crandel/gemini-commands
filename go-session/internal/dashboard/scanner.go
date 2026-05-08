package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/daniel-talonone/gemini-commands/internal/feature"
)

// defaultIsAlive checks whether a process is alive using kill(pid, 0).
func defaultIsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// ScanAll walks ~/.features/<org>/<repo>/<story-id>/ and returns
// a FeatureState for every story directory found.
// Returns an empty slice (not an error) if the features root does not exist.
func ScanAll() ([]FeatureState, error) {
	root, err := feature.FeaturesDir()
	if err != nil {
		return nil, err
	}
	return ScanRoot(root)
}

// ScanRoot is the testable core of ScanAll — it walks an arbitrary root path
// with the same three-level org/repo/story-id structure.
func ScanRoot(root string) ([]FeatureState, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return []FeatureState{}, nil
	}

	var results []FeatureState

	orgEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, orgEntry := range orgEntries {
		if !orgEntry.IsDir() || strings.HasPrefix(orgEntry.Name(), ".") {
			continue
		}
		orgPath := filepath.Join(root, orgEntry.Name())

		repoEntries, err := os.ReadDir(orgPath)
		if err != nil {
			continue
		}

		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() || strings.HasPrefix(repoEntry.Name(), ".") {
				continue
			}
			repoPath := filepath.Join(orgPath, repoEntry.Name())
			repoSlug := orgEntry.Name() + "/" + repoEntry.Name()

			storyEntries, err := os.ReadDir(repoPath)
			if err != nil {
				continue
			}

			for _, storyEntry := range storyEntries {
				if !storyEntry.IsDir() || strings.HasPrefix(storyEntry.Name(), ".") {
					continue
				}
				storyDir := filepath.Join(repoPath, storyEntry.Name())
				storyID := storyEntry.Name()

				var statusPtr *FeatureStatus
				if data, err := os.ReadFile(filepath.Join(storyDir, "status.yaml")); err == nil {
					var s FeatureStatus
					if yaml.Unmarshal(data, &s) == nil {
						statusPtr = &s
					}
				}

				var plan []PlanSlice
				if data, err := os.ReadFile(filepath.Join(storyDir, "plan.yml")); err == nil {
					_ = yaml.Unmarshal(data, &plan)
				}

				state := DeriveState(storyID, repoSlug, statusPtr, plan, defaultIsAlive)
				state.Dir = storyDir
				results = append(results, state)
			}
		}
	}

	return results, nil
}
