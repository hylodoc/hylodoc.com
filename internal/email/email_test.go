package email

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPostUpdateText_Internal(t *testing.T) {
	blogLink := "https://example.com/blog/post-123"
	blogBody := "This is the body of the blog post."
	unsubscribeLink := "https://example.com/unsubscribe?email=test@example.com"

	expectedOutput := `View this post at: https://example.com/blog/post-123

This is the body of the blog post.

Unsubscribe https://example.com/unsubscribe?email=test@example.com`

	result, err := newPostUpdateText(blogLink, blogBody, unsubscribeLink)

	/* verify no error */
	require.NoError(t, err)
	/* verify expected output */
	require.Equal(t, expectedOutput, result)
}
