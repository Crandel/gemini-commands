package description_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-talonone/gemini-commands/internal/description"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDescription(t *testing.T) {
	dir := t.TempDir()
	descPath := filepath.Join(dir, "description.md")
	require.NoError(t, os.WriteFile(descPath, []byte("Test Description"), 0644))

	desc, err := description.LoadDescription(dir)
	require.NoError(t, err)
	assert.Equal(t, "Test Description", desc)
}

func TestLoadDescription_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := description.LoadDescription(dir)
	assert.Error(t, err)
}

func TestUpsertDescription(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		dir := t.TempDir()
		content := "This is a test description."
		err := description.UpsertDescription(dir, content)
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(dir, "description.md"))
		require.NoError(t, err)
		assert.Equal(t, content, string(data))
	})

	t.Run("Error on empty content", func(t *testing.T) {
		dir := t.TempDir()
		err := description.UpsertDescription(dir, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "content is empty")
	})

	t.Run("Error on whitespace content", func(t *testing.T) {
		dir := t.TempDir()
		err := description.UpsertDescription(dir, "   \t\n   ")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "content is empty")
	})

	t.Run("Success on overwrite", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "description.md"), []byte("exists"), 0644))
		err := description.UpsertDescription(dir, "new content")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(dir, "description.md"))
		require.NoError(t, err)
		assert.Equal(t, "new content", string(data))
	})

	t.Run("Error on non-existent feature directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "non-existent")
		err := description.UpsertDescription(dir, "content")
		assert.Error(t, err)
	})

	t.Run("Atomic write cleanup", func(t *testing.T) {
		// This test is hard to trigger reliably, but we can check for temp files
		// not being left over in the success case.
		dir := t.TempDir()
		content := "This is a test description."
		err := description.UpsertDescription(dir, content)
		require.NoError(t, err)

		files, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, file := range files {
			assert.NotContains(t, file.Name(), "description.md.")
		}
	})
}

func TestRenderMarkdown_ValidMarkdown(t *testing.T) {
	input := "# Heading\n**bold** _italic_\n- list item\n`code`"
	output := description.RenderMarkdown(input)

	htmlStr := string(output)
	// Verify markdown rendering produced HTML tags
	assert.Contains(t, htmlStr, "<h1>", "expected h1 tag for heading")
	assert.Contains(t, htmlStr, "<strong>", "expected strong tag for bold")
	assert.Contains(t, htmlStr, "<em>", "expected em tag for italic")
	assert.Contains(t, htmlStr, "<li>", "expected li tag for list item")
	assert.Contains(t, htmlStr, "<code>", "expected code tag for inline code")
}

func TestRenderMarkdown_EmptyInput(t *testing.T) {
	output := description.RenderMarkdown("")
	assert.Equal(t, "", string(output), "expected empty output for empty input")
}

func TestRenderMarkdown_SafeHTML(t *testing.T) {
	// Test that output is template.HTML type (won't be escaped in templates)
	input := "**bold text**"
	output := description.RenderMarkdown(input)

	htmlStr := string(output)
	// Verify the output contains actual HTML tags, not escaped entities
	assert.Contains(t, htmlStr, "<strong>", "expected actual <strong> tag, not &lt;strong&gt;")
	assert.NotContains(t, htmlStr, "&lt;strong&gt;", "should not contain escaped HTML")
}

func TestRenderMarkdown_CodeBlock(t *testing.T) {
	input := "```go\nfunc main() {\n}\n```"
	output := description.RenderMarkdown(input)

	htmlStr := string(output)
	// Verify code block is rendered with <pre> and <code> tags
	assert.Contains(t, htmlStr, "<pre>", "expected pre tag for code block")
	// Code tag might have attributes like class="language-go"
	assert.True(t, strings.Contains(htmlStr, "<code"), "expected code tag inside pre")
}

func TestRenderMarkdown_MultipleHeadingLevels(t *testing.T) {
	input := "# H1\n## H2\n### H3"
	output := description.RenderMarkdown(input)

	htmlStr := string(output)
	assert.Contains(t, htmlStr, "<h1>", "expected h1 tag")
	assert.Contains(t, htmlStr, "<h2>", "expected h2 tag")
	assert.Contains(t, htmlStr, "<h3>", "expected h3 tag")
}

func TestRenderMarkdown_UnorderedAndOrderedLists(t *testing.T) {
	input := "- Item 1\n- Item 2\n\n1. First\n2. Second"
	output := description.RenderMarkdown(input)

	htmlStr := string(output)
	assert.Contains(t, htmlStr, "<ul>", "expected unordered list")
	assert.Contains(t, htmlStr, "<ol>", "expected ordered list")
	assert.Contains(t, htmlStr, "<li>", "expected list items")
}
