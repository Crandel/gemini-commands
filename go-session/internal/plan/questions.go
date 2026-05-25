package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Question represents a single question in questions.yml.
type Question struct {
	ID       string `yaml:"id"`
	Question string `yaml:"question"`
	Status   string `yaml:"status"`
	Answer   string `yaml:"answer,omitempty"`
}

// Questions wraps a list of questions with a top-level key.
type Questions struct {
	Questions []Question `yaml:"questions"`
}

// LoadQuestions reads questions.yml from featureDir. Returns an empty string
// without error if the file does not exist — questions are optional.
// Returns an error if the feature directory does not exist.
func LoadQuestions(featureDir string) (string, error) {
	if _, err := os.Stat(featureDir); os.IsNotExist(err) {
		return "", fmt.Errorf("feature directory does not exist: %s", featureDir)
	}

	data, err := os.ReadFile(filepath.Join(featureDir, "questions.yml"))
	if os.IsNotExist(err) {
		return "", nil // Return an empty string instead of nil
	}
	if err != nil {
		return "", fmt.Errorf("reading questions.yml: %w", err)
	}
	return string(data), nil
}

// Read reads questions.yml from featureDir. Returns an empty byte slice
// without error if the file does not exist — questions are optional.
// Returns an error if the feature directory does not exist.
func Read(featureDir string) ([]byte, error) {
	if _, err := os.Stat(featureDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("feature directory does not exist: %s", featureDir)
	}

	data, err := os.ReadFile(filepath.Join(featureDir, "questions.yml"))
	if os.IsNotExist(err) {
		return []byte{}, nil // Return an empty byte slice instead of nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading questions.yml: %w", err)
	}
	return data, nil
}

var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// writeValidateQuestions validates questions with LLM-actionable error messages.
func writeValidateQuestions(questions Questions) error {
	for i, q := range questions.Questions {
		if q.ID == "" {
			return fmt.Errorf(`questions[%d].id: value is empty — must be a non-empty kebab-case string (e.g. "data-model-clarification")`, i)
		}
		if !kebabCase.MatchString(q.ID) {
			return fmt.Errorf(`questions[%d].id: %q is not kebab-case — must match ^[a-z0-9]+(-[a-z0-9]+)*$ (e.g. "data-model-clarification")`, i, q.ID)
		}
		if q.Question == "" {
			return fmt.Errorf("questions[%d].question: value is empty — must be a non-empty string", i)
		}
		if q.Status != "open" && q.Status != "resolved" && q.Status != "skipped" {
			return fmt.Errorf(`questions[%d].status: %q is not valid — must be "open", "resolved", or "skipped" (e.g. "open")`, i, q.Status)
		}
	}
	return nil
}

// WriteQuestions validates and atomically writes questions to questions.yml.
func WriteQuestions(featureDir string, data []byte) error {
	if _, err := os.Stat(featureDir); os.IsNotExist(err) {
		return fmt.Errorf("feature directory does not exist: %s", featureDir)
	}

	var questions Questions
	if err := yaml.Unmarshal(data, &questions); err != nil {
		return fmt.Errorf("parsing questions.yml: %w", err)
	}

	if err := writeValidateQuestions(questions); err != nil {
		return err
	}

	path := filepath.Join(featureDir, "questions.yml")
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".questions.tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpName) //nolint:errcheck
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName) //nolint:errcheck
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName) //nolint:errcheck
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
