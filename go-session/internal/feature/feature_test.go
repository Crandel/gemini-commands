package feature_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFeature(t *testing.T) {
	tempDir := t.TempDir()
	featureDir := filepath.Join(tempDir, "sc-12345")

	err := feature.CreateFeature(featureDir, "org/repo", "main", "/path/to/workdir")
	require.NoError(t, err)

	// Check that all files are created
	expectedFiles := []string{
		"plan.yml",
		"questions.yml",
		"review.yml",
		"pr.md",
		"status.yaml",
		"log.md",
	}
	for _, f := range expectedFiles {
		_, err := os.Stat(filepath.Join(featureDir, f))
		assert.NoError(t, err, "file %s should be created", f)
	}
}

func TestResolveFeatureDir_ExplicitAbsPath(t *testing.T) {
	dir := t.TempDir()
	result, err := feature.ResolveFeatureDir("/absolute/path/sc-1", dir, "")
	require.NoError(t, err)
	assert.Equal(t, "/absolute/path/sc-1", result)
}

func TestResolveFeatureDir_DotPath(t *testing.T) {
	dir := t.TempDir()
	result, err := feature.ResolveFeatureDir(".features/sc-1", dir, "")
	require.NoError(t, err)
	assert.Equal(t, ".features/sc-1", result)
}

func TestResolveFeatureDir_LocalFeaturesDir(t *testing.T) {
	cwd := t.TempDir()
	local := filepath.Join(cwd, ".features", "sc-1234")
	require.NoError(t, os.MkdirAll(local, 0755))
	result, err := feature.ResolveFeatureDir("sc-1234", cwd, "")
	require.NoError(t, err)
	assert.Equal(t, local, result)
}

func TestResolveFeatureDir_HTTPSRemote(t *testing.T) {
	dir := t.TempDir()
	result, err := feature.ResolveFeatureDir(
		"sc-1234", dir, "https://github.com/myorg/myrepo.git",
	)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(result, filepath.Join("myorg", "myrepo", "sc-1234")))
	assert.Contains(t, result, ".features/")
}

func TestResolveFeatureDir_SSHRemote(t *testing.T) {
	dir := t.TempDir()
	result, err := feature.ResolveFeatureDir(
		"sc-1234", dir, "git@github.com:myorg/myrepo.git",
	)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(result, filepath.Join("myorg", "myrepo", "sc-1234")))
	assert.Contains(t, result, ".features/")
}

func TestResolveFeatureDir_SSHSchemeRemote(t *testing.T) {
	dir := t.TempDir()
	result, err := feature.ResolveFeatureDir(
		"sc-1234", dir, "ssh://git@github.com/myorg/myrepo.git",
	)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(result, filepath.Join("myorg", "myrepo", "sc-1234")))
	assert.Contains(t, result, ".features/")
}

func TestResolveFeatureDir_NoRemoteNoLocal(t *testing.T) {
	dir := t.TempDir()
	_, err := feature.ResolveFeatureDir("sc-1234", dir, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sc-1234")
}

func TestLoadContext_OutputsFilesAsXML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.md"), []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.yml"), []byte("world"), 0644))

	out, err := feature.LoadContext(dir)
	require.NoError(t, err)
	assert.Contains(t, out, `<file name="description.md">
hello
</file>`)
	assert.Contains(t, out, `<file name="plan.yml">
world
</file>`)
}

func TestLoadContext_SortedAlphabetically(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "z.md"), []byte("last"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("first"), 0644))

	out, err := feature.LoadContext(dir)
	require.NoError(t, err)
	aIdx := strings.Index(out, `name="a.md"`)
	zIdx := strings.Index(out, `name="z.md"`)
	assert.Greater(t, zIdx, aIdx, "a.md block should appear before z.md block")
}

func TestLoadContext_ExcludesUnderscoreFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "_SUMMARY.md"), []byte("generated"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.md"), []byte("real"), 0644))

	out, err := feature.LoadContext(dir)
	require.NoError(t, err)
	assert.NotContains(t, out, "_SUMMARY.md")
	assert.Contains(t, out, "description.md")
}

func TestLoadContext_IncludesOnlyTargetExtensions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.yml"), []byte("include me"), 0644))

	out, err := feature.LoadContext(dir)
	require.NoError(t, err)
	assert.Contains(t, out, "plan.yml")
	assert.NotContains(t, out, "notes.txt")
}

func TestLoadContext_SupportedExtensions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("md"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yml"), []byte("yml"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.yaml"), []byte("yaml"), 0644))

	out, err := feature.LoadContext(dir)
	require.NoError(t, err)
	assert.Contains(t, out, `name="a.md"`)
	assert.Contains(t, out, `name="b.yml"`)
	assert.Contains(t, out, `name="c.yaml"`)
}

func TestLoadContext_ErrorOnMissingFeatureDir(t *testing.T) {
	_, err := feature.LoadContext("/nonexistent/path/xyz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestLoadContext_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	out, err := feature.LoadContext(dir)
	require.NoError(t, err)
	assert.Equal(t, "", out)
}
