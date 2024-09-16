package util

import (
	"bytes"
	"net/http"
	"net/url"
	"fmt"
)

type RequestBuilder struct {
	method      string
	url         string
	headers     map[string]string
	body        []byte
	queryParams url.Values
	formParams  url.Values
}

func NewRequestBuilder(method, url string) *RequestBuilder {
	return &RequestBuilder{
		method:      method,
		url:         url,
		headers:     make(map[string]string),
		queryParams: make(map[string][]string),
		formParams:  make(map[string][]string),
	}
}

func (b *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	b.headers[key] = value
	return b
}

func (b *RequestBuilder) WithBody(body []byte) *RequestBuilder {
	b.body = body
	return b
}

func (b *RequestBuilder) WithQueryParam(key, value string) *RequestBuilder {
	b.queryParams.Add(key, value)
	return b
}

func (b *RequestBuilder) WithFormParam(key, value string) *RequestBuilder {
	b.formParams.Add(key, value)
	return b
}

func (b *RequestBuilder) Build() (*http.Request, error) {
	// Append query parameters to URL
	urlWithParams := b.url
	if len(b.queryParams) > 0 {
		urlWithParams += "?" + b.queryParams.Encode()
	}

	var body *bytes.Buffer
	if len(b.formParams) > 0 {
		// Encode form parameters
		body = bytes.NewBufferString(b.formParams.Encode())
	} else {
		body = bytes.NewBuffer(b.body)
	}

	req, err := http.NewRequest(b.method, urlWithParams, body)
	if err != nil {
		return nil, err
	}
	for key, value := range b.headers {
		req.Header.Set(key, value)
	}
	if len(b.formParams) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	return req, nil
}

/* util for getting query parameters from map */
func GetQueryParam(params map[string][]string, key string) (string, error) {
	valuesList, ok := params[key]
	if !ok {
		return "", fmt.Errorf("missing query parameter")
	}

	if len(valuesList) == 0 {
		return "", fmt.Errorf("query parameter %s is empty", key)
	}
	return valuesList[0], nil
}
