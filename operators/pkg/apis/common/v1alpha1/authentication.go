// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import v1 "k8s.io/api/core/v1"

// ElasticsearchAuth contains auth config for Kibana to use with an Elasticsearch cluster
type ElasticsearchAuth struct {
	// Inline is auth provided as plaintext inline credentials.
	Inline *ElasticsearchInlineAuth `json:"inline,omitempty"`
	// SecretKeyRef is a secret that contains the credentials to use.
	SecretKeyRef *v1.SecretKeySelector `json:"secret,omitempty"`
}

// IsConfigured returns true if one of the possible auth mechanisms is configured.
func (ea ElasticsearchAuth) IsConfigured() bool {
	return ea.Inline != nil || ea.SecretKeyRef != nil
}

// ElasticsearchInlineAuth is a basic username/password combination.
type ElasticsearchInlineAuth struct {
	// User is the username to use.
	Username string `json:"username"`
	// Password is the password to use.
	Password string `json:"password"`
}
