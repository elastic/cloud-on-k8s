// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package nodelabels provides the constant and parsing helpers used by the ECK resources to
// declare the set of Kubernetes node labels that must be copied to the annotations of the Pods
// managed by a given resource.
package nodelabels

import (
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// DownwardNodeLabelsAnnotation holds an optional comma-separated list of expected node labels to
// be set as annotations on the Pods managed by an ECK resource.
const DownwardNodeLabelsAnnotation = "eck.k8s.elastic.co/downward-node-labels"

// Parse normalizes a comma-separated node labels annotation value into a sorted, deduplicated
// slice. An empty or whitespace-only value returns nil.
func Parse(annotationValue string) []string {
	labels := set.Make()
	for label := range strings.SplitSeq(annotationValue, ",") {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		labels.Add(label)
	}
	if labels.Count() == 0 {
		return nil
	}
	return labels.AsSortedSlice()
}

// FromAnnotations returns the list of downward node labels declared via DownwardNodeLabelsAnnotation.
func FromAnnotations(annotations map[string]string) []string {
	return Parse(annotations[DownwardNodeLabelsAnnotation])
}
