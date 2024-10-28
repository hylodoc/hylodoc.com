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

	/* record downstream request */
	metrics.RecordClientRequest(req.Method, req.URL.String())

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("error calling downstream: %v\n", err)
		/* record downstream error */
		metrics.RecordClientErrors(req.Method, req.URL.String(), "network_error")
		return nil, err
	}

	/* record downstream success */
	if resp.StatusCode > 400 {
		metrics.RecordClientErrors(req.Method, req.URL.String(), http.StatusText(resp.StatusCode))
	} else {
		metrics.RecordClientSuccess(req.Method, req.URL.String(), http.StatusText(resp.StatusCode))
	}

	/* record downstream call duration */
	duration := time.Since(start).Seconds()
	metrics.RecordClientDuration(req.Method, req.URL.String(), duration, http.StatusText(resp.StatusCode))

	return resp, nil
}
