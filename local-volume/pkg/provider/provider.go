// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package provider

const (
	// Name of our persistent volume provider implementation
	Name = "volumes.k8s.elastic.co/elastic-local"
	// NodeAffinityLabel is the key for the label applied on Persistent Volumes once mounted on a node
	NodeAffinityLabel = "volumes.k8s.elastic.co/node-affinity"
)
