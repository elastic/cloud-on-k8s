// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// RoundTripFunc is used to mock calls to the real network
type RoundTripFunc func(req *http.Request) (*http.Response, error)

// RoundTrip is the method tied to RoundTripFunc.
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewSocketHTTPClient(t *testing.T) {
	type args struct {
		t http.RoundTripper
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Verify transport is not empty when nil transport is specified",
			args: args{},
		},
		{
			name: "Verify transport is not empty when a transport is specified",
			args: args{
				t: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return nil, nil
				}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSocketHTTPClient(tt.args.t)
			if got.Transport == nil {
				t.Error("Transport cannot be empty")
			}
		})
	}
}

func TestCaller_Get(t *testing.T) {
	type fields struct {
		client *http.Client
	}
	type args struct {
		path string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
		err    string
	}{
		{
			name: "Succeeds",
			fields: fields{client: &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Body:       ioutil.NopCloser(strings.NewReader(`{"ok": true}`)),
					}, nil
				}),
			}},
			want: `{"ok": true}`,
		},
		{
			name: "Fails",
			fields: fields{client: &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("ERROR")
				}),
			}},
			err: "Get http://unix: ERROR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Caller{
				client: tt.fields.client,
			}
			got, err := c.Get(tt.args.path)
			if err != nil {
				assert.Equal(t, tt.err, err.Error())
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCaller_Post(t *testing.T) {
	type fields struct {
		client *http.Client
	}
	type args struct {
		path    string
		reqBody interface{}
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
		err    string
	}{
		{
			name: "Succeeds",
			fields: fields{client: &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Body:       ioutil.NopCloser(strings.NewReader(`{"ok": true}`)),
					}, nil
				}),
			}},
			want: `{"ok": true}`,
		},
		{
			name: "Fails",
			fields: fields{client: &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("ERROR")
				}),
			}},
			err: "Post http://unix: ERROR",
		},
		{
			name: "Fails due to no json Body",
			args: args{path: "/mount", reqBody: make(chan int)},
			fields: fields{client: &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return nil, nil
				}),
			}},
			err: `json: unsupported type: chan int`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Caller{
				client: tt.fields.client,
			}
			got, err := c.Post(tt.args.path, tt.args.reqBody)
			if err != nil {
				assert.Equal(t, tt.err, err.Error())
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
