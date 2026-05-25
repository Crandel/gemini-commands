package log

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	logFileName   = "log.md"
	logFileHeader = `# Work Log`
)

// CreateLogFile creates the log.md file with a header if it doesn't already exist.
func CreateLogFile(featureDir string) error {
	logPath := filepath.Join(featureDir, logFileName)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		if err := os.WriteFile(logPath, []byte(logFileHeader), 0644); err != nil {
			return fmt.Errorf("failed to create log.md: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check log.md existence: %w", err)
	}
	return nil
}

// AppendLog appends a timestamped Markdown entry to log.md in featureDir.
// It creates the log.md file with its header if it doesn't already exist.
// The write is atomic: content is staged to a temp file and renamed into place.
func AppendLog(featureDir, message string) error {
	if _, err := os.Stat(featureDir); os.IsNotExist(err) {
		return fmt.Errorf("feature directory does not exist: %s", featureDir)
	}

	logPath := filepath.Join(featureDir, logFileName)

	existing, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		existing = []byte(logFileHeader)
	} else if err != nil {
		return fmt.Errorf("failed to read log.md: %w", err)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	entry := fmt.Sprintf("\n## [%s]\n\n%s\n", timestamp, message)
	updated := append(existing, []byte(entry)...)

	tmp, err := os.CreateTemp(featureDir, ".log.md.tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(updated); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpName) //nolint:errcheck
		return fmt.Errorf("failed to write temp log file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName) //nolint:errcheck
		return fmt.Errorf("failed to close temp log file: %w", err)
	}

	if err := os.Rename(tmpName, logPath); err != nil {
		os.Remove(tmpName) //nolint:errcheck
		return fmt.Errorf("failed to atomically replace log.md: %w", err)
	}
	return nil
}

// LoadLog reads the content of log.md from featureDir.
// Returns ("", nil) if log.md does not exist.
func LoadLog(featureDir string) (string, error) {
	logPath := filepath.Join(featureDir, logFileName)
	content, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read log.md: %w", err)
	}
	return string(content), nil
}
