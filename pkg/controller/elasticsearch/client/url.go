// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

type URLProvider interface {
	// URL returns a URL to route traffic to (can fall back to a k8s service URL).
	URL() (string, error)
	// Equals returns true if the other URLProvider is equal to the one in the receiver.
	Equals(other URLProvider) bool
	// HasEndpoints returns true if the provider has currently any endpoints/URLs to return.
	// Makes sense for implementations that do not return a static URL.
	HasEndpoints() bool
}

// NewStaticURLProvider is a static implementation of the URL provider interface for testing purposes.
func NewStaticURLProvider(url string) URLProvider {
	return &staticURLProvider{
		url: url,
	}
}

type staticURLProvider struct {
	url string
}

// URL implements URLProvider.
func (s *staticURLProvider) URL() (string, error) {
	return s.url, nil
}

// Equals implements URLProvider.
func (s *staticURLProvider) Equals(other URLProvider) bool {
	otherStatic, ok := other.(*staticURLProvider)
	if !ok {
		return false
	}
	return s.url == otherStatic.url
}

func (s *staticURLProvider) HasEndpoints() bool {
	return true
}

var _ URLProvider = &staticURLProvider{}
