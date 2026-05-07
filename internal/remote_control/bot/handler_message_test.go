package bot

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildMediaPrompt(t *testing.T) {
	t.Run("image_only_directs_model_to_use_Read_for_vision", func(t *testing.T) {
		got := buildMediaPrompt("what is this?", []string{".agent/cat.png"}, nil, nil)

		assert.Contains(t, got, "Read tool", "must instruct the model to call Read")
		assert.Contains(t, got, "vision", "image directive must mention vision so the model trusts Read returns visual content")
		assert.Contains(t, got, ".agent/cat.png")
		assert.Contains(t, got, "what is this?")
		assert.Contains(t, got, "<upload_file>.agent/cat.png</upload_file>", "legacy tag preserved for back-compat")
	})

	t.Run("multiple_images_pluralizes_count", func(t *testing.T) {
		got := buildMediaPrompt("", []string{".agent/a.png", ".agent/b.jpg"}, nil, nil)

		assert.Contains(t, got, "2 images")
		assert.Contains(t, got, ".agent/a.png")
		assert.Contains(t, got, ".agent/b.jpg")
	})

	t.Run("documents_get_separate_directive", func(t *testing.T) {
		got := buildMediaPrompt("summarize", nil, []string{".agent/spec.pdf"}, nil)

		assert.Contains(t, got, "document")
		assert.Contains(t, got, "Read tool")
		assert.Contains(t, got, ".agent/spec.pdf")
		assert.Contains(t, got, "summarize")
	})

	t.Run("mixed_images_and_documents", func(t *testing.T) {
		got := buildMediaPrompt("compare", []string{".agent/a.png"}, []string{".agent/b.pdf"}, nil)

		// Image directive precedes document directive — both present.
		imgIdx := strings.Index(got, "image")
		docIdx := strings.Index(got, "document")
		assert.NotEqual(t, -1, imgIdx)
		assert.NotEqual(t, -1, docIdx)
		assert.Contains(t, got, "<upload_file>.agent/a.png</upload_file>")
		assert.Contains(t, got, "<upload_file>.agent/b.pdf</upload_file>")
	})

	t.Run("empty_caption_omits_user_message_section", func(t *testing.T) {
		got := buildMediaPrompt("", []string{".agent/x.png"}, nil, nil)

		assert.NotContains(t, got, "User message:")
	})

	t.Run("no_files_returns_just_caption_or_empty", func(t *testing.T) {
		assert.Equal(t, "User message: hi", buildMediaPrompt("hi", nil, nil, nil))
		assert.Equal(t, "", buildMediaPrompt("", nil, nil, nil))
	})
}
