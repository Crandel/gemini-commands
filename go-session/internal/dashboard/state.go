package dashboard

import (
	"fmt"
	"time"
)

// FeatureStatus is parsed directly from status.yaml.
type FeatureStatus struct {
	Mode         string `yaml:"mode"`
	Repo         string `yaml:"repo"`
	Branch       string `yaml:"branch"`
	WorkDir      string `yaml:"work_dir"`
	PID          int    `yaml:"pid"`
	PipelineStep string `yaml:"pipeline_step"`
	StartedAt    string `yaml:"started_at"`
	UpdatedAt    string `yaml:"updated_at"`
}

// PlanTask is an unexported-friendly exported type for plan.yml task entries.
type PlanTask struct {
	ID     string `yaml:"id"`
	Status string `yaml:"status"`
}

// PlanSlice is an unexported-friendly exported type for plan.yml slice entries.
type PlanSlice struct {
	ID    string     `yaml:"id"`
	Tasks []PlanTask `yaml:"tasks"`
}

// FeatureState is the derived, template-ready view of one feature.
type FeatureState struct {
	StoryID            string
	Dir                string // absolute path to the feature directory on disk
	Repo               string // org/repo — from status.yaml or derived from dir path
	Mode               string
	WorkDir            string // absolute path to repo root on disk, from status.yaml
	PipelineStep       string
	IsRunning          bool
	LastDoneTask       string    // ID of last done task in document order
	AllDone            bool      // true if every task in plan.yml is "done"
	HasStatus          bool      // false if status.yaml was absent
	StartedAt          time.Time // Changed type
	UpdatedAt          time.Time // Changed type
	FormattedStartedAt string    // New field
	FormattedUpdatedAt string    // New field
}

// DeriveState computes a FeatureState from parsed inputs.
// isAlive is injected so callers can mock it in tests.
// status may be nil (feature has no status.yaml yet).
// repo is the fallback org/repo derived from the directory path.
func DeriveState(storyID string, repo string, status *FeatureStatus, plan []PlanSlice, isAlive func(int) bool) FeatureState {
	state := FeatureState{
		StoryID: storyID,
		Repo:    repo,
	}

	if status != nil {
		state.HasStatus = true
		state.Mode = status.Mode
		state.PipelineStep = status.PipelineStep
		state.WorkDir = status.WorkDir
		state.IsRunning = status.PID > 0 && isAlive(status.PID)
		if status.Repo != "" {
			state.Repo = status.Repo
		}

		// Parse StartedAt and UpdatedAt strings into time.Time objects
		if t, err := time.Parse(time.RFC3339, status.StartedAt); err == nil {
			state.StartedAt = t
		}
		if t, err := time.Parse(time.RFC3339, status.UpdatedAt); err == nil {
			state.UpdatedAt = t
		}

		state.FormattedStartedAt = formatTime(state.StartedAt)
		state.FormattedUpdatedAt = formatTime(state.UpdatedAt)
	}

	if len(plan) == 0 {
		return state
	}

	var totalTasks int
	var lastDone string
	allDone := true
	for _, s := range plan {
		for _, t := range s.Tasks {
			totalTasks++
			if t.Status == "done" {
				lastDone = t.ID
			} else {
				allDone = false
			}
		}
	}
	state.AllDone = totalTasks > 0 && allDone
	state.LastDoneTask = lastDone

	return state
}

// formatTime formats a time.Time object into a human-readable string.
// If the the time is within the last 24 hours, it displays a relative time (e.g., "5 minutes ago", "3 hours ago").
// Otherwise, it displays an absolute timestamp in "YYYY-MM-DD HH:mm" format.
// If the time is zero, it returns "—".
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}

	now := time.Now()
	diff := now.Sub(t)

	// Check if within the last 24 hours
	if diff < 24*time.Hour {
		if diff < time.Minute {
			return "just now"
		} else if diff < time.Hour {
			minutes := int(diff.Minutes())
			return fmt.Sprintf("%d %s ago", minutes, pluralize(minutes, "minute"))
		} else { // between 1 hour and 24 hours
			hours := int(diff.Hours())
			return fmt.Sprintf("%d %s ago", hours, pluralize(hours, "hour"))
		}
	}
	// If older than 24 hours, format as YYYY-MM-DD HH:mm
	return t.Format("2006-01-02 15:04")
}

// pluralize returns the singular or plural form of a word based on a count.
func pluralize(count int, word string) string {
	if count == 1 {
		return word
	}
	return word + "s"
}
