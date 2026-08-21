// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// DownwardNodeLabelsAnnotation holds an optional comma-separated list of expected node labels to
// be set as annotations on the Pods managed by an ECK resource. It is a user-facing API annotation
// shared by all ECK resource types, so the canonical definition lives here.
const DownwardNodeLabelsAnnotation = "eck.k8s.elastic.co/downward-node-labels"

// ParseDownwardNodeLabels normalizes a comma-separated node labels annotation value into a sorted,
// deduplicated slice. An empty or whitespace-only value returns nil.
func ParseDownwardNodeLabels(annotationValue string) []string {
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

// DownwardNodeLabelsFromAnnotations returns the list of downward node labels declared via the
// DownwardNodeLabelsAnnotation annotation.
func DownwardNodeLabelsFromAnnotations(annotations map[string]string) []string {
	return ParseDownwardNodeLabels(annotations[DownwardNodeLabelsAnnotation])
}
