// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"net/http"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/dev/portforward"
)

// NewHTTPClient creates a new HTTP client that is aware of any port forwarding configuration.
func NewHTTPClient() http.Client {
	client := http.Client{
		Timeout: 60 * time.Second,
	}
	if *autoPortForward {
		client.Transport = &http.Transport{
			DialContext: portforward.NewForwardingDialer().DialContext,
		}
	}
	return client
}
