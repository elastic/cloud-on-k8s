// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package metrics

import (
	"context"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	clmetrics "k8s.io/client-go/tools/metrics"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	rateLimiterLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "k8s_client_rate_limiter_duration_seconds",
			Help:    "Kubernetes client rate limiter latency in seconds. Broken down by verb, and host.",
			Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1.0, 2.0, 4.0, 8.0, 15.0, 30.0, 60.0},
		},
		[]string{"verb", "host"},
	)

	_ clmetrics.LatencyMetric = &latencyMetrics{}
)

func init() {
	// register the prometheus collector with the controller runtime registry
	crmetrics.Registry.MustRegister(rateLimiterLatency)

	adapter := latencyMetrics{
		rateLimiterLatency: rateLimiterLatency,
	}
	// register the metrics with client-go
	clmetrics.RateLimiterLatency = &adapter
}

// latencyMetrics implements the LatencyMetric interface from k8s client-go package
type latencyMetrics struct {
	rateLimiterLatency *prometheus.HistogramVec
}

func (c *latencyMetrics) Observe(_ context.Context, verb string, u url.URL, latency time.Duration) {
	c.rateLimiterLatency.WithLabelValues(verb, u.Host).Observe(latency.Seconds())
}
