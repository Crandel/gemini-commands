package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3" // Added yaml import
)

// --- helpers ---

func validFinding(id string) Finding {
	return Finding{ID: id, File: "main.go", Line: 1, Feedback: "some feedback", Status: "open"}
}

// --- Create ---

func TestCreate_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Create(dir, TypeDefault))
	_, err := os.Stat(filepath.Join(dir, "review.yml"))
	require.NoError(t, err)
}

func TestCreate_Idempotent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Create(dir, TypeDefault))
	// Write something so we can detect an overwrite
	path := filepath.Join(dir, "review.yml")
	require.NoError(t, os.WriteFile(path, []byte("# keep me"), 0644))
	require.NoError(t, Create(dir, TypeDefault))
	data, _ := os.ReadFile(path)
	assert.Equal(t, "# keep me", string(data), "second Create must not overwrite existing file")
}

func TestCreate_NoTmpLeftover(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Create(dir, TypeDefault))
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "no temp files should remain after Create")
	}
}

func TestCreate_UnknownType(t *testing.T) {
	dir := t.TempDir()
	err := Create(dir, Type("unknown"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review type")
}

func TestCreate_DocsType(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Create(dir, TypeDocs))
	_, err := os.Stat(filepath.Join(dir, "review-docs.yml"))
	require.NoError(t, err)
}

func TestCreate_DevOpsType(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Create(dir, TypeDevOps))
	_, err := os.Stat(filepath.Join(dir, "review-devops.yml"))
	require.NoError(t, err)
}

// --- Load ---

func TestLoad_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestLoad_HappyPath(t *testing.T) {
	dir := t.TempDir()
	f := validFinding("find-1")
	require.NoError(t, Append(dir, TypeDefault, f))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "find-1", findings[0].ID)
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yml")
	require.NoError(t, os.WriteFile(path, []byte(":\tinvalid: yaml: :"), 0644))
	_, err := Load(dir, TypeDefault)
	require.Error(t, err)
}

func TestLoad_InvalidFinding_BadStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yml")
	require.NoError(t, os.WriteFile(path, []byte("- id: find-1\n  feedback: oops\n  status: invalid\n"), 0644))
	_, err := Load(dir, TypeDefault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid finding")
}

func TestLoad_InvalidFinding_BadID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yml")
	require.NoError(t, os.WriteFile(path, []byte("- id: Not_Valid\n  feedback: oops\n  status: open\n"), 0644))
	_, err := Load(dir, TypeDefault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid finding")
}

func TestLoad_InvalidFinding_EmptyFeedback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.yml")
	require.NoError(t, os.WriteFile(path, []byte("- id: find-1\n  feedback: \"\"\n  status: open\n"), 0644))
	_, err := Load(dir, TypeDefault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid finding")
}

func TestLoad_UnknownType(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, Type("unknown"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review type")
}

// --- Append ---

func TestAppend_CreatesFileIfAbsent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-1")))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "find-1", findings[0].ID)
}

func TestAppend_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-1")))
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-2")))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Len(t, findings, 2)
}

func TestAppend_RejectsEmptyID(t *testing.T) {
	dir := t.TempDir()
	f := validFinding("")
	err := Append(dir, TypeDefault, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ID must not be empty")
	assertNoDefaultReviewFile(t, dir)
}

func TestAppend_RejectsNonKebabID(t *testing.T) {
	dir := t.TempDir()
	f := validFinding("Not_Kebab")
	err := Append(dir, TypeDefault, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kebab-case")
	assertNoDefaultReviewFile(t, dir)
}

func TestAppend_RejectsEmptyFeedback(t *testing.T) {
	dir := t.TempDir()
	f := Finding{ID: "find-1", Status: "open"}
	err := Append(dir, TypeDefault, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Feedback must not be empty")
	assertNoDefaultReviewFile(t, dir)
}

func TestAppend_RejectsInvalidStatus(t *testing.T) {
	dir := t.TempDir()
	f := Finding{ID: "find-1", Feedback: "feedback", Status: "todo"}
	err := Append(dir, TypeDefault, f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Status")
	assertNoDefaultReviewFile(t, dir)
}

func TestAppend_UnknownType(t *testing.T) {
	dir := t.TempDir()
	err := Append(dir, Type("unknown"), validFinding("find-1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review type")
}

// --- UpdateStatus ---

func TestUpdateStatus_HappyPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-1")))
	require.NoError(t, UpdateStatus(dir, TypeDefault, "find-1", "resolved"))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Equal(t, "resolved", findings[0].Status)
}

func TestUpdateStatus_UnknownID(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-1")))
	err := UpdateStatus(dir, TypeDefault, "no-such-id", "resolved")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-1")))
	err := UpdateStatus(dir, TypeDefault, "find-1", "done")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be")
}

func TestUpdateStatus_AcceptsSkippedStatus(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-1")))
	require.NoError(t, UpdateStatus(dir, TypeDefault, "find-1", "skipped"))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Equal(t, "skipped", findings[0].Status)
}

func TestUpdateStatus_UnknownType(t *testing.T) {
	dir := t.TempDir()
	err := UpdateStatus(dir, Type("unknown"), "find-1", "resolved")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review type")
}

func TestUpdateStatus_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	// File doesn't exist: Load returns empty slice, so ID will never be found.
	err := UpdateStatus(dir, TypeDefault, "find-1", "resolved")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateStatus_CorruptNeighbourBlocksUpdate(t *testing.T) {
	// A corrupt finding anywhere in the file blocks all UpdateStatus calls via Load's validation.
	// This is intentional: if the file is corrupt, we refuse to produce a partial update.
	dir := t.TempDir()
	require.NoError(t, Append(dir, TypeDefault, validFinding("find-1")))
	// Directly corrupt the file by appending an invalid finding.
	path := filepath.Join(dir, "review.yml")
	data, _ := os.ReadFile(path)
	data = append(data, []byte("- id: INVALID\n  feedback: corrupt\n  status: open\n")...)
	require.NoError(t, os.WriteFile(path, data, 0644))
	err := UpdateStatus(dir, TypeDefault, "find-1", "resolved")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid finding")
}

// --- kebab-case boundary ---

func TestValidate_SingleSegmentIDIsValid(t *testing.T) {
	// Single-segment lowercase IDs like "fix" are valid kebab-case by design.
	f := Finding{ID: "fix", Feedback: "feedback", Status: "open"}
	assert.NoError(t, validate(f))
}

// --- Append rejection does not write non-default types ---

func TestAppend_RejectsInvalidStatus_DocsType(t *testing.T) {
	dir := t.TempDir()
	f := Finding{ID: "find-1", Feedback: "feedback", Status: "todo"}
	err := Append(dir, TypeDocs, f)
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(dir, "review-docs.yml"))
	assert.True(t, os.IsNotExist(statErr), "review-docs.yml must not exist after a rejected write")
}

// --- DiscoverTypes ---

func TestDiscoverTypes_NoFilesFound(t *testing.T) {
	dir := t.TempDir()
	types, err := DiscoverTypes(dir)
	require.NoError(t, err)
	assert.Empty(t, types)
}

func TestDiscoverTypes_DefaultReviewYml(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.yml"), []byte(""), 0644))
	types, err := DiscoverTypes(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{""}, types)
}

func TestDiscoverTypes_DocsReviewYml(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-docs.yml"), []byte(""), 0644))
	types, err := DiscoverTypes(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, types)
}

func TestDiscoverTypes_DevOpsReviewYaml(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-devops.yaml"), []byte(""), 0644))
	types, err := DiscoverTypes(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"devops"}, types)
}

func TestDiscoverTypes_MixedFilesAndExtensions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.yml"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-docs.yaml"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-custom.yml"), []byte(""), 0644))
	types, err := DiscoverTypes(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"", "custom", "docs"}, types) // Should be sorted alphabetically
}

func TestDiscoverTypes_OtherFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review.yml"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(""), 0644))
	types, err := DiscoverTypes(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{""}, types)
}

func TestDiscoverTypes_FeatureDirDoesNotExist(t *testing.T) {
	_, err := DiscoverTypes("/nonexistent/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feature directory does not exist")
}

// --- Write ---

func TestWrite_HappyPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "find-1", findings[0].ID)
}

func TestWrite_ReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-2"), validFinding("find-3")}))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	require.Len(t, findings, 2)
	assert.Equal(t, "find-2", findings[0].ID)
}

func TestWrite_ValidationError_BadID(t *testing.T) {
	dir := t.TempDir()
	err := Write(dir, TypeDefault, []Finding{{ID: "Bad ID", Feedback: "feedback", Status: "open"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "finding[0].id")
	assertNoDefaultReviewFile(t, dir)
}

func TestWrite_ValidationError_EmptyFeedback(t *testing.T) {
	dir := t.TempDir()
	err := Write(dir, TypeDefault, []Finding{{ID: "find-1", Feedback: "", Status: "open"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "finding[0].feedback")
	assertNoDefaultReviewFile(t, dir)
}

func TestWrite_ValidationError_BadStatus(t *testing.T) {
	dir := t.TempDir()
	err := Write(dir, TypeDefault, []Finding{{ID: "find-1", Feedback: "feedback", Status: "todo"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "finding[0].status")
	assertNoDefaultReviewFile(t, dir)
}

func TestWrite_AcceptsSkippedStatus(t *testing.T) {
	dir := t.TempDir()
	finding := validFinding("find-1")
	finding.Status = "skipped"
	err := Write(dir, TypeDefault, []Finding{finding})
	require.NoError(t, err)
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "skipped", findings[0].Status)
}

func TestWrite_UnknownType(t *testing.T) {
	dir := t.TempDir()
	err := Write(dir, Type("unknown"), []Finding{validFinding("find-1")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review type")
}

func TestWrite_NoTmpLeftover(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp")
	}
}

func TestWrite_EmptyFindings(t *testing.T) {
	// Empty findings list is valid — "no issues found" is a legitimate result.
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{}))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestWrite_NonexistentDir(t *testing.T) {
	err := Write("/nonexistent/path", TypeDefault, []Finding{validFinding("find-1")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feature directory does not exist")
}

// --- AllTypes ---

func TestAllTypes_ReturnsAllThree(t *testing.T) {
	types := AllTypes()
	assert.Len(t, types, 3)
	assert.Contains(t, types, TypeDefault)
	assert.Contains(t, types, TypeDocs)
	assert.Contains(t, types, TypeDevOps)
}

// --- TypeName ---

func TestTypeName_KnownTypes(t *testing.T) {
	cases := []struct {
		t    Type
		want string
	}{
		{TypeDefault, "regular"},
		{TypeDocs, "docs"},
		{TypeDevOps, "devops"},
	}
	for _, c := range cases {
		name, err := TypeName(c.t)
		require.NoError(t, err)
		assert.Equal(t, c.want, name)
	}
}

func TestTypeName_UnknownType(t *testing.T) {
	_, err := TypeName(Type("unknown"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review type")
}

// --- ReadFindings ---

func TestReadFindings_HappyPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))
	s, err := ReadFindings(dir, TypeDefault)
	require.NoError(t, err)
	assert.Contains(t, s, "find-1")
}

func TestReadFindings_FiltersResolvedFindings(t *testing.T) {
	dir := t.TempDir()
	open := validFinding("open-1")
	resolved := Finding{ID: "resolved-1", File: "main.go", Line: 1, Feedback: "was fixed", Status: "resolved"}
	require.NoError(t, Write(dir, TypeDefault, []Finding{open, resolved}))
	s, err := ReadFindings(dir, TypeDefault)
	require.NoError(t, err)
	assert.Contains(t, s, "open-1")
	assert.NotContains(t, s, "resolved-1")
}

func TestReadFindings_AllResolved_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	resolved := Finding{ID: "resolved-1", File: "main.go", Line: 1, Feedback: "was fixed", Status: "resolved"}
	require.NoError(t, Write(dir, TypeDefault, []Finding{resolved}))
	s, err := ReadFindings(dir, TypeDefault)
	require.NoError(t, err)
	assert.Empty(t, s)
}

func TestReadFindings_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{}))
	s, err := ReadFindings(dir, TypeDefault)
	require.NoError(t, err)
	assert.Empty(t, s)
}

func TestReadFindings_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	s, err := ReadFindings(dir, TypeDefault)
	require.NoError(t, err)
	assert.Empty(t, s)
}

func TestReadFindings_UnknownType(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadFindings(dir, Type("unknown"))
	require.Error(t, err)
}

// --- LoadByFilename ---

func TestLoadByFilename_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	findings, err := LoadByFilename(dir, "review-nonexistent.yml")
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestLoadByFilename_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review-custom.yml")
	finding := validFinding("custom-find-1")
	data, err := yaml.Marshal([]Finding{finding})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	findings, err := LoadByFilename(dir, "review-custom.yml")
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "custom-find-1", findings[0].ID)
}

func TestLoadByFilename_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review-invalid.yml")
	require.NoError(t, os.WriteFile(path, []byte(":\tinvalid: yaml: :"), 0644))
	_, err := LoadByFilename(dir, "review-invalid.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing review-invalid.yml")
}

func TestLoadByFilename_InvalidFinding_BadStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review-bad-status.yml")
	require.NoError(t, os.WriteFile(path, []byte("- id: find-1\n  feedback: oops\n  status: invalid\n"), 0644))
	_, err := LoadByFilename(dir, "review-bad-status.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid finding in review-bad-status.yml")
}

func TestLoadByFilename_InvalidFinding_BadID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review-bad-id.yml")
	require.NoError(t, os.WriteFile(path, []byte("- id: Not_Valid\n  feedback: oops\n  status: open\n"), 0644))
	_, err := LoadByFilename(dir, "review-bad-id.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid finding in review-bad-id.yml")
}

func TestLoadByFilename_InvalidFinding_EmptyFeedback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review-empty-feedback.yml")
	require.NoError(t, os.WriteFile(path, []byte("- id: find-1\n  feedback: \"\"\n  status: open\n"), 0644))
	_, err := LoadByFilename(dir, "review-empty-feedback.yml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid finding in review-empty-feedback.yml")
}

// --- helpers ---

func assertNoDefaultReviewFile(t *testing.T, dir string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, "review.yml"))
	assert.True(t, os.IsNotExist(err), "review.yml must not exist after a rejected write")
}

func TestFinding_NotesField_Marshaling(t *testing.T) {
	t.Run("marshals with notes when present", func(t *testing.T) {
		f := Finding{ID: "f-1", Feedback: "feedback", Status: "open", Notes: "this is a note"}
		data, err := yaml.Marshal(f)
		require.NoError(t, err)
		assert.Contains(t, string(data), "notes: this is a note")
	})

	t.Run("marshals without notes when empty", func(t *testing.T) {
		f := Finding{ID: "f-1", Feedback: "feedback", Status: "open", Notes: ""}
		data, err := yaml.Marshal(f)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "notes:")
	})

	t.Run("unmarshals with notes when present", func(t *testing.T) {
		yamlData := `
id: f-1
feedback: feedback
status: open
notes: this is a note
`
		var f Finding
		err := yaml.Unmarshal([]byte(yamlData), &f)
		require.NoError(t, err)
		assert.Equal(t, "this is a note", f.Notes)
	})

	t.Run("unmarshals without notes when absent", func(t *testing.T) {
		yamlData := `
id: f-1
feedback: feedback
status: open
`
		var f Finding
		err := yaml.Unmarshal([]byte(yamlData), &f)
		require.NoError(t, err)
		assert.Equal(t, "", f.Notes)
	})
}

// --- UpdateStatuses ---

func TestUpdateStatuses_HappyPath_SingleUpdate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1"), validFinding("find-2")}))

	updates := []UpdateRequest{
		{ID: "find-1", Status: "resolved", Notes: "Fixed this."},
	}
	require.NoError(t, UpdateStatuses(dir, TypeDefault, updates))

	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Len(t, findings, 2)
	assert.Equal(t, "resolved", findings[0].Status)
	assert.Equal(t, "Fixed this.", findings[0].Notes)
	assert.Equal(t, "open", findings[1].Status) // Unchanged
}

func TestUpdateStatuses_HappyPath_MultiUpdate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1"), validFinding("find-2"), validFinding("find-3")}))

	updates := []UpdateRequest{
		{ID: "find-1", Status: "resolved", Notes: "Done."},
		{ID: "find-3", Status: "skipped", Notes: "Will do later."},
	}
	require.NoError(t, UpdateStatuses(dir, TypeDefault, updates))

	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Len(t, findings, 3)
	assert.Equal(t, "resolved", findings[0].Status)
	assert.Equal(t, "Done.", findings[0].Notes)
	assert.Equal(t, "open", findings[1].Status) // Unchanged
	assert.Equal(t, "skipped", findings[2].Status)
	assert.Equal(t, "Will do later.", findings[2].Notes)
}

func TestUpdateStatuses_Error_IDNotFound(t *testing.T) {
	dir := t.TempDir()
	originalContent, err := yaml.Marshal([]Finding{validFinding("find-1")})
	require.NoError(t, err)
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))

	updates := []UpdateRequest{
		{ID: "find-nonexistent", Status: "resolved"},
	}
	err = UpdateStatuses(dir, TypeDefault, updates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `finding ID "find-nonexistent" not found`)

	// Verify file is untouched
	currentContent, _ := os.ReadFile(filepath.Join(dir, "review.yml"))
	assert.Equal(t, string(originalContent), string(currentContent))
}

func TestUpdateStatuses_Error_InvalidStatus(t *testing.T) {
	dir := t.TempDir()
	originalContent, err := yaml.Marshal([]Finding{validFinding("find-1")})
	require.NoError(t, err)
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))

	updates := []UpdateRequest{
		{ID: "find-1", Status: "invalid-status"},
	}
	err = UpdateStatuses(dir, TypeDefault, updates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `update[0].status: "invalid-status" is not valid`)

	// Verify file is untouched
	currentContent, _ := os.ReadFile(filepath.Join(dir, "review.yml"))
	assert.Equal(t, string(originalContent), string(currentContent))
}

func TestUpdateStatuses_Error_InvalidKebabCaseID(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))

	updates := []UpdateRequest{
		{ID: "InvalidID", Status: "resolved"},
	}
	err := UpdateStatuses(dir, TypeDefault, updates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `update[0].id: "InvalidID" is not kebab-case`)
}

func TestUpdateStatuses_Error_EmptyID(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))

	updates := []UpdateRequest{
		{ID: "", Status: "resolved"},
	}
	err := UpdateStatuses(dir, TypeDefault, updates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `update[0].id: value is empty`)
}

func TestUpdateStatuses_EmptyUpdatesList(t *testing.T) {
	dir := t.TempDir()
	originalContent, err := yaml.Marshal([]Finding{validFinding("find-1")})
	require.NoError(t, err)
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))

	// Empty update list should be a no-op
	require.NoError(t, UpdateStatuses(dir, TypeDefault, []UpdateRequest{}))

	// Verify file is untouched
	currentContent, _ := os.ReadFile(filepath.Join(dir, "review.yml"))
	assert.Equal(t, string(originalContent), string(currentContent))
}

func TestUpdateStatuses_UpdateNotesOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Write(dir, TypeDefault, []Finding{validFinding("find-1")}))

	updates := []UpdateRequest{
		// Status is a required field in the update request validation, so we must provide one.
		// We can "update" it to the same status.
		{ID: "find-1", Status: "skipped", Notes: "Adding a note here."},
	}
	// First update to skipped with a note
	require.NoError(t, UpdateStatuses(dir, TypeDefault, updates))
	findings, err := Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Equal(t, "skipped", findings[0].Status)
	assert.Equal(t, "Adding a note here.", findings[0].Notes)

	// Second update to change the note on the same status
	updates = []UpdateRequest{
		{ID: "find-1", Status: "skipped", Notes: "Updated note."},
	}
	require.NoError(t, UpdateStatuses(dir, TypeDefault, updates))
	findings, err = Load(dir, TypeDefault)
	require.NoError(t, err)
	assert.Equal(t, "skipped", findings[0].Status)
	assert.Equal(t, "Updated note.", findings[0].Notes)
}
