// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

// Monitoring holds references to both the metrics, and logs Elasticsearch clusters for
// configuring stack monitoring.
type Monitoring struct {
	// Metrics holds references to Elasticsearch clusters which receive monitoring data from this resource.
	// +kubebuilder:validation:Optional
	Metrics MetricsMonitoring `json:"metrics,omitempty"`
	// Logs holds references to Elasticsearch clusters which receive log data from an associated resource.
	// +kubebuilder:validation:Optional
	Logs LogsMonitoring `json:"logs,omitempty"`
}

// MetricsMonitoring holds a list of Elasticsearch clusters which receive monitoring data from
// associated resources.
type MetricsMonitoring struct {
	// ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster.
	// Due to existing limitations, only a single Elasticsearch cluster is currently supported.
	// +kubebuilder:validation:Required
	ElasticsearchRefs []ObjectSelector `json:"elasticsearchRefs,omitempty"`
}

// LogsMonitoring holds a list of Elasticsearch clusters which receive logs data from
// associated resources.
type LogsMonitoring struct {
	// ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster.
	// Due to existing limitations, only a single Elasticsearch cluster is currently supported.
	// +kubebuilder:validation:Required
	ElasticsearchRefs []ObjectSelector `json:"elasticsearchRefs,omitempty"`
}
