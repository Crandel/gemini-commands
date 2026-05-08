package server_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/daniel-talonone/gemini-commands/internal/dashboard"
	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/daniel-talonone/gemini-commands/internal/server"
	"github.com/daniel-talonone/gemini-commands/internal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalHandler_MissingPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/action/terminal", nil)
	w := httptest.NewRecorder()
	server.TerminalHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "path parameter is required")
}

func TestTerminalHandler_NonExistentPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/action/terminal?path=/nonexistent/path/xyz123", nil)
	w := httptest.NewRecorder()
	server.TerminalHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTerminalHandler_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "testfile")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	req := httptest.NewRequest(http.MethodGet, "/action/terminal?path="+filepath.ToSlash(f.Name()), nil)
	w := httptest.NewRecorder()
	server.TerminalHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFinderHandler_MissingPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/action/finder", nil)
	w := httptest.NewRecorder()
	server.FinderHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "path parameter is required")
}

func TestFinderHandler_NonExistentPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/action/finder?path=/nonexistent/path/xyz123", nil)
	w := httptest.NewRecorder()
	server.FinderHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFinderHandler_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "testfile")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	req := httptest.NewRequest(http.MethodGet, "/action/finder?path="+filepath.ToSlash(f.Name()), nil)
	w := httptest.NewRecorder()
	server.FinderHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// mockScanner implements the Scanner interface for testing purposes.
type mockScanner struct {
	features []dashboard.FeatureState
	err      error
}

func (m *mockScanner) ScanAll() ([]dashboard.FeatureState, error) {
	return m.features, m.err
}

// storyIDs extracts StoryID from a slice of FeatureState for easy assertion.
func storyIDs(features []dashboard.FeatureState) []string {
	ids := make([]string, len(features))
	for i, f := range features {
		ids[i] = f.StoryID
	}
	return ids
}

// parsedOrder parses space-separated StoryIDs from the dummy template output.
func parsedOrder(body string) []string {
	return strings.Fields(strings.TrimSpace(body))
}

func TestSortingLogic(t *testing.T) {
	mockData := []dashboard.FeatureState{
		{
			StoryID:   "feature-c",
			Repo:      "repo-a",
			UpdatedAt: time.Date(2023, time.January, 3, 0, 0, 0, 0, time.UTC),
			StartedAt: time.Date(2023, time.January, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			StoryID:   "feature-a",
			Repo:      "repo-b",
			IsRunning: true,
			UpdatedAt: time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
			StartedAt: time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			StoryID:   "feature-b",
			Repo:      "repo-a",
			UpdatedAt: time.Date(2023, time.January, 2, 0, 0, 0, 0, time.UTC),
			StartedAt: time.Date(2023, time.January, 2, 0, 0, 0, 0, time.UTC),
		},
	}
	mockS := &mockScanner{features: mockData}

	// Dummy template outputs StoryIDs space-separated for easy order verification.
	tmpl, err := template.New("test").Parse(`{{range .Features}}{{.StoryID}} {{end}}`)
	require.NoError(t, err)

	srv := server.New(8080, mockS)

	tests := []struct {
		name          string
		url           string
		expectedOrder []string
	}{
		{
			name:          "Default sort is updated desc",
			url:           "/",
			expectedOrder: []string{"feature-c", "feature-b", "feature-a"},
		},
		{
			name:          "Sort by updated asc",
			url:           "/?sort=updated&order=asc",
			expectedOrder: []string{"feature-a", "feature-b", "feature-c"},
		},
		{
			name:          "Sort by updated desc",
			url:           "/?sort=updated&order=desc",
			expectedOrder: []string{"feature-c", "feature-b", "feature-a"},
		},
		{
			name:          "Sort by started asc",
			url:           "/?sort=started&order=asc",
			expectedOrder: []string{"feature-a", "feature-b", "feature-c"},
		},
		{
			name:          "Sort by started desc",
			url:           "/?sort=started&order=desc",
			expectedOrder: []string{"feature-c", "feature-b", "feature-a"},
		},
		{
			name:          "Unknown sort key falls back to updated field with given order",
			url:           "/?sort=unknown&order=asc",
			expectedOrder: []string{"feature-a", "feature-b", "feature-c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rr := httptest.NewRecorder()
			srv.MakeListHandler(tmpl).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tt.expectedOrder, parsedOrder(rr.Body.String()))
		})
	}
}

func TestSortingWithFilters(t *testing.T) {
	mockData := []dashboard.FeatureState{
		{StoryID: "feature-c", Repo: "repo-a", AllDone: true, UpdatedAt: time.Date(2023, time.January, 3, 0, 0, 0, 0, time.UTC)},
		{StoryID: "feature-a", Repo: "repo-b", IsRunning: true, UpdatedAt: time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)},
		{StoryID: "feature-b", Repo: "repo-a", UpdatedAt: time.Date(2023, time.January, 2, 0, 0, 0, 0, time.UTC)},
	}
	mockS := &mockScanner{features: mockData}

	tmpl, err := template.New("test").Parse(`{{range .Features}}{{.StoryID}} {{end}}`)
	require.NoError(t, err)

	srv := server.New(8080, mockS)

	tests := []struct {
		name          string
		url           string
		expectedOrder []string
	}{
		{
			name:          "Sort by updated asc with repo filter",
			url:           "/?repo=repo-a&sort=updated&order=asc",
			expectedOrder: []string{"feature-b", "feature-c"},
		},
		{
			name:          "Sort by updated desc with done status filter",
			url:           "/?status=done&sort=updated&order=desc",
			expectedOrder: []string{"feature-c"},
		},
		{
			name:          "Sort by updated desc with running status filter",
			url:           "/?status=running&sort=updated&order=desc",
			expectedOrder: []string{"feature-a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rr := httptest.NewRecorder()
			srv.MakeListHandler(tmpl).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tt.expectedOrder, parsedOrder(rr.Body.String()))
		})
	}
}

func TestSortFeaturesByUpdatedAt(t *testing.T) {
	features := []dashboard.FeatureState{
		{StoryID: "a", UpdatedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{StoryID: "b", UpdatedAt: time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC)},
		{StoryID: "c", UpdatedAt: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	cp := make([]dashboard.FeatureState, len(features))

	copy(cp, features)
	server.SortFeatures(cp, "updated", "asc")
	assert.Equal(t, []string{"a", "c", "b"}, storyIDs(cp))

	copy(cp, features)
	server.SortFeatures(cp, "updated", "desc")
	assert.Equal(t, []string{"b", "c", "a"}, storyIDs(cp))
}

func TestSortFeaturesByStartedAt(t *testing.T) {
	features := []dashboard.FeatureState{
		{StoryID: "a", StartedAt: time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC)},
		{StoryID: "b", StartedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{StoryID: "c", StartedAt: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	cp := make([]dashboard.FeatureState, len(features))

	copy(cp, features)
	server.SortFeatures(cp, "started", "asc")
	assert.Equal(t, []string{"b", "c", "a"}, storyIDs(cp))

	copy(cp, features)
	server.SortFeatures(cp, "started", "desc")
	assert.Equal(t, []string{"a", "c", "b"}, storyIDs(cp))
}

func TestSortFeaturesZeroTimesLast(t *testing.T) {
	features := []dashboard.FeatureState{
		{StoryID: "a", UpdatedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{StoryID: "b"}, // zero UpdatedAt
		{StoryID: "c", UpdatedAt: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	cp := make([]dashboard.FeatureState, len(features))

	copy(cp, features)
	server.SortFeatures(cp, "updated", "desc")
	assert.Equal(t, []string{"c", "a", "b"}, storyIDs(cp), "zero times should sort last in desc")

	copy(cp, features)
	server.SortFeatures(cp, "updated", "asc")
	assert.Equal(t, []string{"b", "a", "c"}, storyIDs(cp), "zero times should sort first in asc")
}

// TestHandlerWithRealTemplate renders the actual template.html to catch
// field name mismatches between PageData/FeatureState and the template.
func TestHandlerWithRealTemplate(t *testing.T) {
	tmplContent, err := os.ReadFile("template.html")
	require.NoError(t, err)

	funcMap := template.FuncMap{
		"safeURL": func(s string) template.URL { return template.URL(s) },
	}
	tmpl, err := template.New("dashboard").Funcs(funcMap).Parse(string(tmplContent))
	require.NoError(t, err)

	now := time.Now()
	mockS := &mockScanner{features: []dashboard.FeatureState{
		{
			StoryID:            "sc-1",
			Repo:               "org/repo",
			StartedAt:          now.Add(-1 * time.Hour),
			UpdatedAt:          now,
			FormattedStartedAt: "1 hour ago",
			FormattedUpdatedAt: "just now",
		},
	}}
	srv := server.New(8080, mockS)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.MakeListHandler(tmpl).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "real template should render without errors")
	assert.Contains(t, rr.Body.String(), "sc-1")
	assert.Contains(t, rr.Body.String(), "1 hour ago")
	assert.Contains(t, rr.Body.String(), "just now")
}

func setupFeatureDetailHandlerTest(t *testing.T) (*server.Server, *template.Template, *mockScanner) {
	tmplContent, err := os.ReadFile("template.html")
	require.NoError(t, err)

	funcMap := template.FuncMap{
		"safeURL":  func(s string) template.URL { return template.URL(s) },
		"urlquery": func(s string) string { return url.QueryEscape(s) },
	}
	tmpl, err := template.New("dashboard").Funcs(funcMap).Parse(string(tmplContent))
	require.NoError(t, err)

	mockS := &mockScanner{}
	srv := server.New(8080, mockS)
	return srv, tmpl, mockS
}

func TestFeatureDetailHandler(t *testing.T) {
	srv, tmpl, mockS := setupFeatureDetailHandlerTest(t)

	t.Run("no review files", func(t *testing.T) {
		featureID := "sc-no-review"
		repo := "org/repo"
		home, _ := os.UserHomeDir()
		featureDir := filepath.Join(home, ".features", repo, featureID)
		require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
		defer func() { _ = os.RemoveAll(featureDir) }()
		// Remove the default review file to simulate no review files existing
		require.NoError(t, os.Remove(filepath.Join(featureDir, "review.yml")))

		mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

		req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
		rr := httptest.NewRecorder()
		srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.NotContains(t, rr.Body.String(), "<summary>Review Findings</summary>")
	})

	t.Run("default selection", func(t *testing.T) {
		featureID := "sc-review-default"
		repo := "org/repo"
		home, _ := os.UserHomeDir()
		featureDir := filepath.Join(home, ".features", repo, featureID)
		require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
		defer func() { _ = os.RemoveAll(featureDir) }()
		mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review.yml"), []byte(`- id: find-1
  feedback: default feedback
  status: open
  file: main.go
  line: 10`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review-docs.yml"), []byte(`- id: find-2
  feedback: docs feedback
  status: open`), 0644))

		req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
		rr := httptest.NewRecorder()
		srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, "<summary>Review Findings</summary>")
		assert.Contains(t, body, `<option value="" selected`)
		assert.Contains(t, body, "default feedback")
		assert.NotContains(t, body, "docs feedback")
		assert.Equal(t, 1, strings.Count(body, `class="review-card open"`))
	})

	t.Run("query param selection", func(t *testing.T) {
		featureID := "sc-review-query"
		repo := "org/repo"
		home, _ := os.UserHomeDir()
		featureDir := filepath.Join(home, ".features", repo, featureID)
		require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
		defer func() { _ = os.RemoveAll(featureDir) }()
		mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review.yml"), []byte(`- id: find-1
  feedback: default feedback
  status: open`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review-docs.yml"), []byte(`- id: find-2
  feedback: docs feedback
  status: open
  file: README.md
  line: 5`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review-devops.yaml"), []byte(`- id: find-3
  feedback: devops feedback
  status: open`), 0644))

		req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID+"?review=docs", nil)
		rr := httptest.NewRecorder()
		srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, `<option value="docs" selected`)
		assert.NotContains(t, body, "default feedback")
		assert.Contains(t, body, "docs feedback")
		assert.NotContains(t, body, "devops feedback")

		req = httptest.NewRequest(http.MethodGet, "/feature/"+featureID+"?review=devops", nil)
		rr = httptest.NewRecorder()
		srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		body = rr.Body.String()
		assert.Contains(t, body, `<option value="devops" selected`)
		assert.Contains(t, body, "devops feedback")
	})

	t.Run("details open with open findings", func(t *testing.T) {
		featureID := "sc-review-open"
		repo := "org/repo"
		home, _ := os.UserHomeDir()
		featureDir := filepath.Join(home, ".features", repo, featureID)
		require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
		defer func() { _ = os.RemoveAll(featureDir) }()
		mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review.yml"), []byte(`- id: f1
  feedback: open
  status: open`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review-docs.yml"), []byte(`- id: f2
  feedback: resolved
  status: resolved`), 0644))

		req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
		rr := httptest.NewRecorder()
		srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "<details open>", "details should be open when open findings exist")

		req = httptest.NewRequest(http.MethodGet, "/feature/"+featureID+"?review=docs", nil)
		rr = httptest.NewRecorder()
		srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.NotContains(t, rr.Body.String(), "<details open>", "details should be closed when no open findings")
	})

	t.Run("non-existent type gracefully handled", func(t *testing.T) {
		featureID := "sc-review-nonexistent"
		repo := "org/repo"
		home, _ := os.UserHomeDir()
		featureDir := filepath.Join(home, ".features", repo, featureID)
		require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
		defer func() { _ = os.RemoveAll(featureDir) }()
		mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}
		// review.yml is created by default
		require.NoError(t, os.WriteFile(filepath.Join(featureDir, "review.yml"), []byte(`- id: f1
  feedback: default
  status: open`), 0644))

		req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID+"?review=nonexistent", nil)
		rr := httptest.NewRecorder()
		srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()

		assert.Contains(t, body, "<summary>Review Findings</summary>")
		assert.Contains(t, body, `<option value="nonexistent" selected`)
		assert.Contains(t, body, `<p class="empty">No findings for this review file.</p>`)
		assert.NotContains(t, body, `class="review-card`)
	})
}


func TestFeatureDetailHandler_ContentRendering(t *testing.T) {
	srv, tmpl, mockS := setupFeatureDetailHandlerTest(t)

	featureID := "sc-content-rendering"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()
	mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

	// 1. Test with Description and Plan
	descContent := "# My Feature\n\n- Point 1\n- Point 2"
	planContent := `- id: slice-1
  description: The first slice
  status: done
  tasks:
    - id: task-1
      task: First task
      status: done`
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "description.md"), []byte(descContent), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "plan.yml"), []byte(planContent), 0644))
	// Remove review file to isolate content rendering
	require.NoError(t, os.Remove(filepath.Join(featureDir, "review.yml")))

	req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
	rr := httptest.NewRecorder()
	srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	// Assert Description is rendered
	assert.Contains(t, body, "<summary>Description</summary>", "should show description section")
	assert.Contains(t, body, "<h1>My Feature</h1>", "should render description markdown")
	assert.Contains(t, body, "<li>Point 1</li>", "should render description list")

	// Assert Plan is rendered
	assert.Contains(t, body, "<summary>Plan</summary>", "should show plan section")
	assert.Contains(t, body, "<h3>slice-1 — The first slice</h3>", "should render slice description")
	assert.Contains(t, body, "<strong>task-1</strong>: First task", "should render task")
	assert.Contains(t, body, `<span class="status-badge done">done</span>`, "should render status badges")

	// Assert other sections are hidden
	assert.NotContains(t, body, "<summary>Review Findings</summary>", "should hide review section")

	// 2. Test with no Description and no Plan
	require.NoError(t, os.Remove(filepath.Join(featureDir, "description.md")))
	require.NoError(t, os.Remove(filepath.Join(featureDir, "plan.yml")))

	req = httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
	rr = httptest.NewRecorder()
	srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	body = rr.Body.String()
	assert.NotContains(t, body, "<summary>Description</summary>", "should hide description when file is missing")
	assert.NotContains(t, body, "<summary>Plan</summary>", "should hide plan when file is missing")
}

func TestFeatureDetailHandler_LogLoading(t *testing.T) {
	// Template with Log section support
	tmplContent := `{{define "feature_detail"}}
<a href="/">← Back</a>
<h1>{{.ID}}</h1>
{{if .Log}}
<details>
  <summary>Log</summary>
  <div class="description-content">
    {{.Log}}
  </div>
</details>
{{end}}
{{end}}`

	funcMap := template.FuncMap{
		"safeURL":  func(s string) template.URL { return template.URL(s) },
		"urlquery": func(s string) string { return url.QueryEscape(s) },
	}
	tmpl, err := template.New("dashboard").Funcs(funcMap).Parse(tmplContent)
	require.NoError(t, err)

	mockS := &mockScanner{}
	srv := server.New(8080, mockS)

	tests := []struct {
		name       string
		featureID  string
		repo       string
		logContent string
		deleteLog  bool
		assertions func(t *testing.T, body string)
	}{
		{
			name:       "Log with markdown content is rendered as HTML",
			featureID:  "sc-log-markdown",
			repo:       "org/repo",
			logContent: `# Log Title

**Bold entry** and _italic note_

- Item 1
- Item 2`,
			deleteLog:  false,
			assertions: func(t *testing.T, body string) {
				// Verify log section exists
				assert.Contains(t, body, "<summary>Log</summary>", "expected Log section summary")
				assert.Contains(t, body, `<div class="description-content">`, "expected log content div")
				// Verify markdown rendering
				assert.Contains(t, body, "<h1>Log Title</h1>", "expected h1 heading from log")
				assert.Contains(t, body, "<strong>Bold entry</strong>", "expected strong tag from log")
				assert.Contains(t, body, "<em>italic note</em>", "expected em tag from log")
				assert.Contains(t, body, "<li>Item 1</li>", "expected list item from log")
				// Verify HTML tags are not escaped
				assert.NotContains(t, body, "&lt;h1&gt;", "HTML tags should not be escaped")
				assert.NotContains(t, body, "&lt;strong&gt;", "HTML tags should not be escaped")
			},
		},
		{
			name:       "Missing log.md hides log section",
			featureID:  "sc-no-log",
			repo:       "org/repo",
			logContent: "",
			deleteLog:  true,
			assertions: func(t *testing.T, body string) {
				// Verify feature still renders
				assert.Contains(t, body, "<h1>sc-no-log</h1>", "expected feature ID to be rendered")
				// Verify log section is hidden
				assert.NotContains(t, body, "<summary>Log</summary>", "expected no Log section when log.md missing")
				assert.NotContains(t, body, `<div class="description-content">`, "expected no log content div when log.md missing")
			},
		},
		{
			name:       "Complex log with code blocks renders correctly",
			featureID:  "sc-log-code",
			repo:       "org/repo",
			logContent: `## Development Notes

` + "```go" + `
func test() {
  fmt.Println("code")
}
` + "```" + `

See the code above.`,
			deleteLog:  false,
			assertions: func(t *testing.T, body string) {
				assert.Contains(t, body, "<h2>Development Notes</h2>", "expected h2 heading from log")
				assert.Contains(t, body, "<pre>", "expected pre tag for code block")
				assert.Contains(t, body, "fmt.Println", "expected code content to be preserved")
				assert.Contains(t, body, "See the code above", "expected text after code block")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, err := os.UserHomeDir()
			require.NoError(t, err)
			featureDir := filepath.Join(home, ".features", tt.repo, tt.featureID)
			require.NoError(t, feature.CreateFeature(featureDir, tt.repo, "main", ""))
			defer func() { _ = os.RemoveAll(featureDir) }()

			logPath := filepath.Join(featureDir, "log.md")
			if tt.deleteLog {
				// Remove the auto-created log.md file to test missing log scenario
				require.NoError(t, os.Remove(logPath))
			} else if tt.logContent != "" {
				// Overwrite with custom content
				require.NoError(t, os.WriteFile(logPath, []byte(tt.logContent), 0644))
			}

			mockS.features = []dashboard.FeatureState{{StoryID: tt.featureID, Repo: tt.repo, Dir: featureDir}}

			req := httptest.NewRequest(http.MethodGet, "/feature/"+tt.featureID, nil)
			rr := httptest.NewRecorder()
			srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			body := rr.Body.String()
			tt.assertions(t, body)
		})
	}
}

func TestResetHandler_SuccessfulReset(t *testing.T) {
	srv := server.New(8080, &mockScanner{})

	featureID := "sc-reset-success"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Create a plan with a slice and task
	planContent := `- id: slice-1
  description: Test slice
  status: in-progress
  tasks:
    - id: task-1
      task: Test task
      status: done`
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "plan.yml"), []byte(planContent), 0644))

	// Create status.yaml with a pipeline_step set
	statusContent := `mode: test
repo: org/repo
branch: main
pipeline_step: some-step`
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "status.yaml"), []byte(statusContent), 0644))

	mockS := &mockScanner{features: []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}}
	srv.ScanAllFunc = mockS.ScanAll

	// Make the POST request
	req := httptest.NewRequest(http.MethodPost, "/feature/"+featureID+"/reset", nil)
	rr := httptest.NewRecorder()
	srv.MakeResetHandler()(rr, req)

	// Verify 303 redirect to /feature/{id}
	require.Equal(t, http.StatusSeeOther, rr.Code, "should return 303 redirect")
	assert.Equal(t, "/feature/"+featureID, rr.Header().Get("Location"))

	// Verify plan was reset
	pln, err := plan.LoadPlan(featureDir)
	require.NoError(t, err)
	require.Len(t, pln, 1)
	assert.Equal(t, "todo", pln[0].Status, "slice status should be reset to todo")
	require.Len(t, pln[0].Tasks, 1)
	assert.Equal(t, "todo", pln[0].Tasks[0].Status, "task status should be reset to todo")

	// Verify pipeline_step was cleared
	st, err := status.LoadStatus(featureDir)
	require.NoError(t, err)
	assert.Equal(t, "", st.PipelineStep, "pipeline_step should be cleared")
}

func TestResetHandler_NonexistentFeature(t *testing.T) {
	srv := server.New(8080, &mockScanner{features: []dashboard.FeatureState{}})

	req := httptest.NewRequest(http.MethodPost, "/feature/nonexistent/reset", nil)
	rr := httptest.NewRecorder()
	srv.MakeResetHandler()(rr, req)

	// Verify 404 response
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestResetHandler_EmptyPlan(t *testing.T) {
	featureID := "sc-reset-empty-plan"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Create an empty plan
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "plan.yml"), []byte(""), 0644))

	mockS := &mockScanner{features: []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}}
	srv := server.New(8080, mockS)

	req := httptest.NewRequest(http.MethodPost, "/feature/"+featureID+"/reset", nil)
	rr := httptest.NewRecorder()
	srv.MakeResetHandler()(rr, req)

	// Verify 500 error for empty plan
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "plan is empty or missing")
}

func TestResetHandler_MethodNotAllowed(t *testing.T) {
	srv := server.New(8080, &mockScanner{})

	// GET request should not be handled by reset handler
	req := httptest.NewRequest(http.MethodGet, "/feature/sc-123/reset", nil)
	rr := httptest.NewRecorder()
	srv.MakeResetHandler()(rr, req)

	// Verify 404 for non-POST requests
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestResetHandler_InvalidPath(t *testing.T) {
	srv := server.New(8080, &mockScanner{})

	// Request without /reset suffix should not be handled
	req := httptest.NewRequest(http.MethodPost, "/feature/sc-123", nil)
	rr := httptest.NewRecorder()
	srv.MakeResetHandler()(rr, req)

	// Verify 404 for non-reset paths
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestResetButtonRendering_WithPlan(t *testing.T) {
	srv, tmpl, mockS := setupFeatureDetailHandlerTest(t)

	featureID := "sc-button-with-plan"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Create a plan with a slice
	planContent := `- id: slice-1
  description: Test slice
  status: todo
  tasks:
    - id: task-1
      task: Test task
      status: todo`
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "plan.yml"), []byte(planContent), 0644))
	// Remove review file to isolate
	require.NoError(t, os.Remove(filepath.Join(featureDir, "review.yml")))

	mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

	req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
	rr := httptest.NewRecorder()
	srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	// Verify Reset button is present (HTMX-based, no form submit)
	assert.Contains(t, body, `hx-post="/feature/`+featureID+`/reset"`, "Reset button should use HTMX post")
	assert.Contains(t, body, `hx-confirm=`, "Reset button should use hx-confirm for confirmation")
	assert.Contains(t, body, "Reset</button>", "Reset button label should be present")
}

func TestResetButtonRendering_WithoutPlan(t *testing.T) {
	srv, tmpl, mockS := setupFeatureDetailHandlerTest(t)

	featureID := "sc-button-without-plan"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Remove the plan file to simulate no plan
	require.NoError(t, os.Remove(filepath.Join(featureDir, "plan.yml")))
	// Remove review file to isolate
	require.NoError(t, os.Remove(filepath.Join(featureDir, "review.yml")))

	mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

	req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
	rr := httptest.NewRecorder()
	srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	// Verify Reset button form is NOT present
	assert.NotContains(t, body, `/reset"`, "Reset form should not be present when plan is missing")
	assert.NotContains(t, body, "Reset</button>", "Reset button should not be present when plan is missing")
}

func TestPlanHandler_SuccessfulPlanGeneration(t *testing.T) {
	srv := server.New(8080, &mockScanner{})

	featureID := "sc-plan-success"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Plan.yml should be empty (created with empty array by feature.CreateFeature)
	mockS := &mockScanner{features: []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}}
	srv.ScanAllFunc = mockS.ScanAll

	// Make the POST request
	req := httptest.NewRequest(http.MethodPost, "/feature/"+featureID+"/plan", nil)
	rr := httptest.NewRecorder()
	srv.MakePlanHandler()(rr, req)

	// Verify 303 redirect to /feature/{id}
	require.Equal(t, http.StatusSeeOther, rr.Code, "should return 303 redirect")
	assert.Equal(t, "/feature/"+featureID, rr.Header().Get("Location"))
}

func TestPlanHandler_NonexistentFeature(t *testing.T) {
	srv := server.New(8080, &mockScanner{features: []dashboard.FeatureState{}})

	req := httptest.NewRequest(http.MethodPost, "/feature/nonexistent/plan", nil)
	rr := httptest.NewRecorder()
	srv.MakePlanHandler()(rr, req)

	// Verify 404 response
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPlanHandler_ExistingPlan(t *testing.T) {
	srv := server.New(8080, &mockScanner{})

	featureID := "sc-plan-existing"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Create a non-empty plan
	planContent := `- id: slice-1
  description: Test slice
  status: todo
  tasks:
    - id: task-1
      task: Test task
      status: todo`
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "plan.yml"), []byte(planContent), 0644))

	mockS := &mockScanner{features: []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}}
	srv.ScanAllFunc = mockS.ScanAll

	// Make the POST request
	req := httptest.NewRequest(http.MethodPost, "/feature/"+featureID+"/plan", nil)
	rr := httptest.NewRecorder()
	srv.MakePlanHandler()(rr, req)

	// Verify 409 conflict for non-empty plan
	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "plan already exists")
}

func TestPlanHandler_MethodNotAllowed(t *testing.T) {
	srv := server.New(8080, &mockScanner{})

	// GET request should not be handled by plan handler
	req := httptest.NewRequest(http.MethodGet, "/feature/sc-123/plan", nil)
	rr := httptest.NewRecorder()
	srv.MakePlanHandler()(rr, req)

	// Verify 404 for non-POST requests
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPlanHandler_InvalidPath(t *testing.T) {
	srv := server.New(8080, &mockScanner{})

	// Request without /plan suffix should not be handled
	req := httptest.NewRequest(http.MethodPost, "/feature/sc-123", nil)
	rr := httptest.NewRecorder()
	srv.MakePlanHandler()(rr, req)

	// Verify 404 for non-plan paths
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPlanButtonRendering_WithoutPlan(t *testing.T) {
	srv, tmpl, mockS := setupFeatureDetailHandlerTest(t)

	featureID := "sc-button-without-plan-plan"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Remove the plan file to simulate no plan
	require.NoError(t, os.Remove(filepath.Join(featureDir, "plan.yml")))
	// Remove review file to isolate
	require.NoError(t, os.Remove(filepath.Join(featureDir, "review.yml")))

	mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

	req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
	rr := httptest.NewRecorder()
	srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	// Verify Plan split button is present (HTMX-based, no form submit)
	assert.Contains(t, body, `hx-post="/feature/`+featureID+`/plan"`, "Plan button should use HTMX post")
	assert.Contains(t, body, "Plan</button>", "Plan button label should be present")
	assert.Contains(t, body, `split-btn`, "Split button container should be present")

	// Verify Reset button form is NOT present
	assert.NotContains(t, body, `/reset"`, "Reset form should not be present when plan is missing")
	assert.NotContains(t, body, "Reset</button>", "Reset button should not be present when plan is missing")
}

func TestPlanButtonRendering_WithPlan(t *testing.T) {
	srv, tmpl, mockS := setupFeatureDetailHandlerTest(t)

	featureID := "sc-button-with-plan-plan"
	repo := "org/repo"
	home, _ := os.UserHomeDir()
	featureDir := filepath.Join(home, ".features", repo, featureID)
	require.NoError(t, feature.CreateFeature(featureDir, repo, "main", ""))
	defer func() { _ = os.RemoveAll(featureDir) }()

	// Create a plan with a slice
	planContent := `- id: slice-1
  description: Test slice
  status: todo
  tasks:
    - id: task-1
      task: Test task
      status: todo`
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "plan.yml"), []byte(planContent), 0644))
	// Remove review file to isolate
	require.NoError(t, os.Remove(filepath.Join(featureDir, "review.yml")))

	mockS.features = []dashboard.FeatureState{{StoryID: featureID, Repo: repo, Dir: featureDir}}

	req := httptest.NewRequest(http.MethodGet, "/feature/"+featureID, nil)
	rr := httptest.NewRecorder()
	srv.MakeFeatureDetailHandler(tmpl).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	// Verify Reset button is present (HTMX-based, no form submit)
	assert.Contains(t, body, `hx-post="/feature/`+featureID+`/reset"`, "Reset button should use HTMX post")
	assert.Contains(t, body, "Reset</button>", "Reset button label should be present")

	// Verify Plan split button is NOT present when plan exists
	assert.NotContains(t, body, `Plan</button>`, "Plan button should not be present when plan is non-empty")
}
