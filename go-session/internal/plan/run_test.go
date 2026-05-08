package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAISessionHome_EnvVar(t *testing.T) {
	t.Setenv("AI_SESSION_HOME", "/tmp/fake-home")
	got := AISessionHome()
	if got != "/tmp/fake-home" {
		t.Fatalf("expected /tmp/fake-home, got %q", got)
	}
}

func TestAISessionHome_FallbackFromExecutable(t *testing.T) {
	t.Setenv("AI_SESSION_HOME", "")
	got := AISessionHome()
	if got == "" {
		t.Fatal("expected non-empty fallback path, got empty string")
	}
}

func TestPreparePrompt_SubstitutesStoryIDAndDir(t *testing.T) {
	dir := t.TempDir()
	promptDir := filepath.Join(dir, "headless", "session")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatalf("creating prompt dir: %v", err)
	}
	templateContent := "Plan for <story-id> at <feature-dir>."
	if err := os.WriteFile(filepath.Join(promptDir, "plan.md"), []byte(templateContent), 0644); err != nil {
		t.Fatalf("writing plan.md: %v", err)
	}

	t.Setenv("AI_SESSION_HOME", dir)
	got, err := PreparePrompt("sc-9999", "/tmp/features/sc-9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Plan for sc-9999 at /tmp/features/sc-9999."
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPreparePrompt_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AI_SESSION_HOME", dir)
	_, err := PreparePrompt("sc-1234", "/tmp/features/sc-1234")
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
	}
}
