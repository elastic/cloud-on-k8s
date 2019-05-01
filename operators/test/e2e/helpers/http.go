// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"flag"
	"net/http"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/dev/portforward"
)

const (
	// DefaultReqTimeout is the default timeout used when performing HTTP calls in end to end tests
	DefaultReqTimeout = 60 * time.Second
)

// if `--auto-port-forward` is passed to `go test`, then use a custom
// dialer that sets up port-forwarding to services running within k8s
// (useful when running tests on a dev env instead of as a batch job)
var autoPortForward = flag.Bool(
	"auto-port-forward", false,
	"enables automatic port-forwarding (for dev use only as it exposes "+
		"k8s resources on ephemeral ports to localhost)")

// NewHTTPClient creates a new HTTP client that is aware of any port forwarding configuration.
func NewHTTPClient() http.Client {
	client := http.Client{
		Timeout: DefaultReqTimeout,
	}
	if *autoPortForward {
		client.Transport = &http.Transport{
			DialContext: portforward.NewForwardingDialer().DialContext,
		}
	}
	return client
}
