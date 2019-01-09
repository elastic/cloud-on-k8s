package client

import "net/http"

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
