package status

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Status represents the structure of the status.yaml file.
//
// INVARIANT: every field in this struct must have a corresponding key in status.yaml and vice versa.
// Unmarshaling a file that contains a key not present in this struct silently drops that key on the
// next Write round-trip. Add new status.yaml fields here before deploying code that writes them.
type Status struct {
	Mode         string `yaml:"mode"`
	Repo         string `yaml:"repo"`
	Branch       string `yaml:"branch"`
	WorkDir      string `yaml:"work_dir"`
	PID          int    `yaml:"pid"`
	PipelineStep string `yaml:"pipeline_step"`
	StartedAt    string `yaml:"started_at"`
	UpdatedAt    string `yaml:"updated_at"`
	StoryURL     string `yaml:"story_url"`
	ClonePath    string `yaml:"clone_path"`
	Error        string `yaml:"error"`
	PRURL        string `yaml:"pr_url"`
	PRTitle      string `yaml:"pr_title"`
	ImplementationStrategy string `yaml:"strategy"`
}

// Create creates a new status.yaml file with initial values.
// Idempotent: if status.yaml already exists the call is a no-op and returns nil.
func Create(featureDir, repo, branch, workDir, storyURL, mode string) error {
	statusPath := filepath.Join(featureDir, "status.yaml")
	if _, err := os.Stat(statusPath); err == nil {
		return nil // already exists — never overwrite live runtime state
	}

	now := time.Now().Format(time.RFC3339)
	s := Status{
		Mode:         mode,
		Repo:         repo,
		Branch:       branch,
		WorkDir:      workDir,
		PID:          0,
		PipelineStep: "new",
		StartedAt:    now,
		UpdatedAt:    now,
		StoryURL:     storyURL,
	}

	data, err := yaml.Marshal(&s)
	if err != nil {
		return fmt.Errorf("marshaling status.yaml: %w", err)
	}

	tmpPath := statusPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing status.yaml.tmp: %w", err)
	}
	if err := os.Rename(tmpPath, statusPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("renaming status.yaml.tmp: %w", err)
	}
	return nil
}

// ReadStep returns the current pipeline_step value from status.yaml.
// Returns an empty string without error if the file does not exist.
func ReadStep(featureDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(featureDir, "status.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading status.yaml: %w", err)
	}
	var s Status
	if err := yaml.Unmarshal(data, &s); err != nil {
		return "", fmt.Errorf("unmarshaling status.yaml: %w", err)
	}
	return s.PipelineStep, nil
}

// LoadStatus reads and unmarshals the status.yaml file into a Status struct.
func LoadStatus(featureDir string) (*Status, error) {
	data, err := os.ReadFile(filepath.Join(featureDir, "status.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading status.yaml: %w", err)
	}
	var s Status
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshaling status.yaml: %w", err)
	}
	return &s, nil
}

// Write reads, updates, and atomically writes the status.yaml file.
// If repo or branch are empty, their existing values are preserved.
func Write(featureDir, step, repo, branch string) error {
	statusPath := filepath.Join(featureDir, "status.yaml")
	var s Status

	// Read existing status.yaml — if absent, nothing to update.
	data, err := os.ReadFile(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no status file: skip update silently (best-effort side-effect)
		}
		return fmt.Errorf("reading status.yaml: %w", err)
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unmarshaling status.yaml: %w", err)
	}

	s.PipelineStep = step
	s.UpdatedAt = time.Now().Format(time.RFC3339)
	if repo != "" {
		s.Repo = repo
	}
	if branch != "" {
		s.Branch = branch
	}

	// Marshal back to YAML
	updatedData, err := yaml.Marshal(&s)
	if err != nil {
		return fmt.Errorf("marshaling status.yaml: %w", err)
	}

	// Atomically write to file
	tmpPath := statusPath + ".tmp"
	if err := os.WriteFile(tmpPath, updatedData, 0644); err != nil {
		return fmt.Errorf("writing status.yaml.tmp: %w", err)
	}
	if err := os.Rename(tmpPath, statusPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("renaming status.yaml.tmp to status.yaml: %w", err)
	}

	return nil
}

// WritePRTitle updates the status.yaml file with the given PR title.
func WritePRTitle(featureDir, title string) error {
	statusPath := filepath.Join(featureDir, "status.yaml")

	data, err := os.ReadFile(statusPath)
	if err != nil {
		return fmt.Errorf("reading status.yaml: %w", err)
	}

	var s Status
	if err := yaml.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unmarshaling status.yaml: %w", err)
	}

	s.PRTitle = title
	s.UpdatedAt = time.Now().Format(time.RFC3339)

	updatedData, err := yaml.Marshal(&s)
	if err != nil {
		return fmt.Errorf("marshaling status.yaml: %w", err)
	}

	tmpPath := statusPath + ".tmp"
	if err := os.WriteFile(tmpPath, updatedData, 0644); err != nil {
		return fmt.Errorf("writing status.yaml.tmp: %w", err)
	}
	if err := os.Rename(tmpPath, statusPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("renaming status.yaml.tmp to status.yaml: %w", err)
	}
	return nil
}

// WriteStrategy updates the strategy field in status.yaml, preserving all other fields.
// Returns nil (no-op) if status.yaml does not exist.
func WriteStrategy(featureDir, strategy string) error {
	statusPath := filepath.Join(featureDir, "status.yaml")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading status.yaml: %w", err)
	}
	var s Status
	if err := yaml.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unmarshaling status.yaml: %w", err)
	}
	s.ImplementationStrategy = strategy
	s.UpdatedAt = time.Now().Format(time.RFC3339)
	updatedData, err := yaml.Marshal(&s)
	if err != nil {
		return fmt.Errorf("marshaling status.yaml: %w", err)
	}
	tmpPath := statusPath + ".tmp"
	if err := os.WriteFile(tmpPath, updatedData, 0644); err != nil {
		return fmt.Errorf("writing status.yaml.tmp: %w", err)
	}
	if err := os.Rename(tmpPath, statusPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("renaming status.yaml.tmp: %w", err)
	}
	return nil
}

// WritePRURL updates the status.yaml file with the given PR URL and sets the pipeline_step.
func WritePRURL(featureDir, url string) error {
	statusPath := filepath.Join(featureDir, "status.yaml")

	data, err := os.ReadFile(statusPath)
	if err != nil {
		return fmt.Errorf("reading status.yaml: %w", err)
	}

	var s Status
	if err := yaml.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unmarshaling status.yaml: %w", err)
	}

	s.PRURL = url
	s.PipelineStep = "pr-submitted"
	s.UpdatedAt = time.Now().Format(time.RFC3339)

	updatedData, err := yaml.Marshal(&s)
	if err != nil {
		return fmt.Errorf("marshaling status.yaml: %w", err)
	}

	tmpPath := statusPath + ".tmp"
	if err := os.WriteFile(tmpPath, updatedData, 0644); err != nil {
		return fmt.Errorf("writing status.yaml.tmp: %w", err)
	}
	if err := os.Rename(tmpPath, statusPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("renaming status.yaml.tmp to status.yaml: %w", err)
	}

	return nil
}
