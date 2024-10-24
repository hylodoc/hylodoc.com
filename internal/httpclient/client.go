package httpclient

import (
	"net/http"
	"time"
)

/* wrapper around standard client */
type Client struct {
	client *http.Client
}

func NewHttpClient(timeout time.Duration) *Client {
	return &Client{
		client: &http.Client{Timeout: timeout},
	}
}

/* sends request and returns response */
func (h *Client) Do(req *http.Request) (*http.Response, error) {
	/* XXX: retries and metrics */
	return h.client.Do(req)
}
