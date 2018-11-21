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

	"github.com/elastic/localvolume/pkg/driver/model"
)

const protocol = "unix"

func NewSocketHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial(protocol, model.UnixSocket)
			},
		},
	}
}

func buildURL(urlPath string) string {
	fullPath := path.Join(protocol, urlPath)
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

func Get(path string) (string, error) {
	resp, err := NewSocketHTTPClient().Get(buildURL(path))
	if err != nil {
		return "", err
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func Post(path string, reqBody interface{}) (string, error) {
	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	resp, err := NewSocketHTTPClient().Post(buildURL(path), "", bytes.NewReader(jsonBytes))
	if err != nil {
		return "", err
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
