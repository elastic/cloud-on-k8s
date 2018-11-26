package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
)

const networkProtocol = "unix"

var defaultClientTransport = &http.Transport{
	DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial(networkProtocol, protocol.UnixSocket)
	},
}

// Caller wraps an HTTP client so that it can be tweaked.
type Caller struct {
	client *http.Client
}

// NewCaller returns a fully initialized HTTP Caller
func NewCaller(c *http.Client) Caller {
	return Caller{client: c}
}

// NewSocketHTTPClient creates a new http.Client with either an explicit set of
// transport settings or de default ones if nil
func NewSocketHTTPClient(t http.RoundTripper) *http.Client {
	if t == nil {
		t = defaultClientTransport
	}
	return &http.Client{Transport: t}
}

// Get performs a GET call to the specified path.
func (c *Caller) Get(path string) (string, error) {
	resp, err := c.client.Get(buildURL(path))
	if err != nil {
		return "", err
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// Post performs a POST call to the specified path and body.
func (c *Caller) Post(path string, reqBody interface{}) (string, error) {
	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Post(buildURL(path), "", bytes.NewReader(jsonBytes))
	if err != nil {
		return "", err
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func buildURL(urlPath string) string {
	fullPath := path.Join(networkProtocol, urlPath)
	return fmt.Sprintf("http://%s", fullPath)
}

func readBody(resp *http.Response) (string, error) {
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", err
	}
	return string(body), nil
}
