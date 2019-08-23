// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import v1 "k8s.io/api/core/v1"

// ElasticsearchAuth contains auth config for Kibana to use with an Elasticsearch cluster
type ElasticsearchAuth struct {
	// SecretKeyRef is a secret that contains the credentials to use.
	SecretKeyRef *v1.SecretKeySelector `json:"secret,omitempty"`
}

// IsConfigured returns true if one of the possible auth mechanisms is configured.
func (ea ElasticsearchAuth) IsConfigured() bool {
	return ea.SecretKeyRef != nil
}
