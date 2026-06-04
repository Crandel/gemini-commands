package description

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
)

// LoadDescription reads description.md from featureDir.
func LoadDescription(featureDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(featureDir, "description.md"))
	if err != nil {
		return "", fmt.Errorf("reading description.md: %w", err)
	}
	return string(data), nil
}

// RenderMarkdown converts a markdown string to safe HTML using goldmark.
// Returns empty template.HTML if input is empty or rendering fails.
func RenderMarkdown(markdown string) template.HTML {
	if markdown == "" {
		return template.HTML("")
	}
	var buf bytes.Buffer
	if err := goldmark.New().Convert([]byte(markdown), &buf); err != nil {
		return template.HTML("")
	}
	return template.HTML(buf.String())
}

// UpsertDescription saves the description content to description.md in an atomic
// way. It validates that the content is not empty. If the file already
// exists, it will be overwritten.
func UpsertDescription(featureDir, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("content is empty; provide non-empty content via stdin or positional argument")
	}

	p := filepath.Join(featureDir, "description.md")

	tempFile, err := os.CreateTemp(featureDir, "description.md.*")
	if err != nil {
		return fmt.Errorf("creating temp file for description: %w", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()

	if _, err := tempFile.WriteString(content); err != nil {
		return fmt.Errorf("writing to temp file for description: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("closing temp file for description: %w", err)
	}

	if err := os.Rename(tempFile.Name(), p); err != nil {
		return fmt.Errorf("renaming temp file for description: %w", err)
	}

	return nil
}
