// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"net/http"
	"time"

	"github.com/elastic/go-elasticsearch/v8/estransport"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("client")

// esLogger implements estransport.Logger using the controller-runtime logging infrastructure
type esLogger struct{}

var (
	_ estransport.Logger = &esLogger{}
)

// LogRoundTrip should not modify the request or response, except for consuming and closing the body.
// Implementations have to check for nil values in request and response.
func (l *esLogger) LogRoundTrip(
	req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration,
) error {
	// TODO: consider logging according to ECS: https://github.com/elastic/ecs/blob/master/schemas/http.yml
	params := []interface{}{
		"event.duration", dur,
		"url.scheme", req.URL.Scheme,
		"url.domain", req.URL.Hostname(),
		"url.port", req.URL.Port(),
		"url.path", req.URL.Path,
		"url.query", req.URL.RawQuery,
		"http.request.method", req.Method,
		"http.response.status_code", res.StatusCode,
	}
	if err == nil {
		log.Info("Elasticsearch request", params...)
	} else {
		log.Error(err, "Elasticsearch request", params...)
	}

	return nil
}

// RequestBodyEnabled makes the client pass a copy of request body to the logger.
func (l *esLogger) RequestBodyEnabled() bool {
	return false
}

// ResponseBodyEnabled makes the client pass a copy of response body to the logger.
func (l *esLogger) ResponseBodyEnabled() bool {
	return false
}
