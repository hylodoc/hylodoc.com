package email

import (
	"log"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xr0-org/progstack/internal/config"
)

func TestMain(m *testing.M) {
	if err := config.LoadConfig("./../../conf.yaml"); err != nil {
		log.Fatalf("Error loading config in test: %v", err)
	}

	/* Run tests */
	m.Run()
}

func TestNewPostUpdateText_Internal(t *testing.T) {
	blogLink := "https://example.com/blog/post-123"
	blogSubject := "subject"
	blogBody := "This is the body of the blog post."

	expectedOutput := `View this post at: https://example.com/blog/post-123

This is the body of the blog post.

Unsubscribe http://xr0.localhost:7999/blogs/1/unsubscribe?token=123456789`

	blogParams := BlogParams{
		ID:        1,
		From:      "no-reply@xr0.dev",
		Subdomain: "xr0",
	}
	subscriberParams := SubscriberParams{
		To:               "b@xr0.dev",
		UnsubscribeToken: "123456789",
	}
	postParams := PostParams{
		Link:    blogLink,
		Subject: blogSubject,
		Body:    blogBody,
	}

	result, err := newPostUpdateText(NewPostUpdateParams{
		Blog:       blogParams,
		Subscriber: subscriberParams,
		Post:       postParams,
	})

	/* verify no error */
	require.NoError(t, err)
	/* verify expected output */
	require.Equal(t, expectedOutput, result)
}
