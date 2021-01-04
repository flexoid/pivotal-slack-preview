package messages

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractStoriesFromMessage(t *testing.T) {
	text := `test message https://www.pivotaltracker.com/story/show/172893694 some text
		http://pivotaltracker.com/story/show/11237854 and
		another pivotaltracker.com/story/show/939764745
		and different
		format https://www.pivotaltracker.com/projects/12412/stories/873924 also
		should work
		https://www.pivotaltracker.com/n/projects/1234/stories/989213175`

	assert.Equal(t, ExtractStoriesFromMessage(text), []int{172893694, 11237854, 939764745, 873924, 989213175})
}

func TestExtractStoriesFromMessageWithoutLinks(t *testing.T) {
	text := `some message without links 123 http://example.com/123 other links
		here https://www.pivotaltracker.com/n/projects/41245`

	assert.Equal(t, ExtractStoriesFromMessage(text), []int(nil))
}

func TestExtractStoriesFromMessageUniqueOnly(t *testing.T) {
	text := `pivotaltracker.com/story/show/123 and pivotaltracker.com/story/show/237
		and pivotaltracker.com/story/show/123`

	assert.Equal(t, ExtractStoriesFromMessage(text), []int{123, 237})
}
