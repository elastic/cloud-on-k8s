// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reserved

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestIsReservedLabelKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "elasticsearch subdomain reserved", key: "elasticsearch.k8s.elastic.co/cluster-name", want: true},
		{name: "common subdomain reserved", key: "common.k8s.elastic.co/type", want: true},
		{name: "association subdomain reserved", key: "association.k8s.elastic.co/es-conf", want: true},
		{name: "policy subdomain reserved", key: "policy.k8s.elastic.co/settings-hash", want: true},
		{name: "k8s.elastic.co subdomain reserved", key: "k8s.elastic.co/foo", want: true},
		{name: "alpha propagation annotation domain reserved as label key", key: "eck.k8s.alpha.elastic.co/propagate-labels", want: true},
		{name: "eck subdomain reserved", key: "eck.k8s.elastic.co/client-authentication-required", want: true},
		{name: "update subdomain reserved", key: "update.k8s.elastic.co/timestamp", want: true},
		{name: "empty string not reserved", key: "", want: false},
		{name: "no slash not reserved", key: "elasticsearch.k8s.elastic.co", want: false},
		{name: "lookalike suffix not reserved", key: "notk8s.elastic.co/foo", want: false},
		{name: "third-party label not reserved", key: "velero.io/exclude-from-backup", want: false},
		{name: "user label not reserved", key: "team", want: false},
		{name: "user label with prefix not reserved", key: "example.com/team", want: false},
		{name: "k8s lookalike domain not reserved", key: "foo.k8s.notelastic.co/bar", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReservedLabelKey(tt.key); got != tt.want {
				t.Errorf("IsReservedLabelKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestIsReservedAnnotationKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "last applied config", key: corev1.LastAppliedConfigAnnotation, want: true},
		{name: "eck alpha propagation key", key: "eck.k8s.alpha.elastic.co/propagate-labels", want: true},
		{name: "user annotation", key: "foo", want: false},
		{name: "user domain annotation", key: "example.com/config", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReservedAnnotationKey(tt.key); got != tt.want {
				t.Errorf("IsReservedAnnotationKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
