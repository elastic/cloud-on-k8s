// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

type URLProvider interface {
	// PodURL is a url for a random pod (falls back to ServiceURL).
	PodURL() string
	// ServiceURL is the url for the Kubernetes service related to the Pod URLs provided.
	ServiceURL() string

	HasEndpoints() bool
}

func NewStaticURLProvider(url string) URLProvider {
	return &staticURLProvider{
		url: url,
	}
}

type staticURLProvider struct {
	url string
}

// PodURL implements URLProvider.
func (s *staticURLProvider) PodURL() string {
	return s.url
}

// ServiceURL implements URLProvider.
func (s *staticURLProvider) ServiceURL() string {
	return s.url
}

func (s *staticURLProvider) HasEndpoints() bool {
	return true
}

var _ URLProvider = &staticURLProvider{}
