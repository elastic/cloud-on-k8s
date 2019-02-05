package client

import (
	"io/ioutil"
	"net/http"
	"strings"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewMockClient(fn RoundTripFunc) Client {
	return Client{
		HTTP: &http.Client{
			Transport: RoundTripFunc(fn),
		},
		Endpoint: "http://example.com",
	}
}

func NewMockResponse(statusCode int, r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    r,
	}
}
