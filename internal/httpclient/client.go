package httpclient

import (
	"log"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/metrics"
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
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("error calling downstream: %v\n", err)
		/* record downstream error */
		metrics.RecordClientErrors(req.Method, req.URL.String())
		return nil, err
	}

	duration := time.Since(start).Seconds()

	/* record downstream call duration */
	metrics.RecordClientDuration(req.Method, req.URL.String(), duration)

	return resp, nil
}
