// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"maps"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// PropagateAnnotationsAnnotation is the annotation used to indicate which annotations should be propagated to child objects.
	PropagateAnnotationsAnnotation = "eck.k8s.alpha.elastic.co/propagate-annotations"
	// PropagateLabelsAnnotation is the annotation used to indicate which labels should be propagated to child objects.
	PropagateLabelsAnnotation = "eck.k8s.alpha.elastic.co/propagate-labels"
)

var eckAnnotationsRegex = regexp.MustCompile(`.*\.k8s\.(.*\.)*elastic\.co/.*`)

// MetadataToPropagate holds the annotations and labels that should be propagated to children.
type MetadataToPropagate struct {
	Annotations map[string]string
	Labels      map[string]string
}

// GetMetadataToPropagate returns the annotations and labels that should be propagated.
// The annotations and labels to propagate are determined by the values of the eck.k8s.elastic.co/propagate-(annotations|labels) annotations.
// Users are expected to provide a comma separated list of annotations or labels they wish to have propagated. The special value of "*" signals
// that all existing annotations/labels should be propagated.
// When propagating annotations, any ECK annotations and the kubectl last applied configuration annotation are ignored.
func GetMetadataToPropagate(obj metav1.Object) MetadataToPropagate {
	md := MetadataToPropagate{}

	if obj == nil || len(obj.GetAnnotations()) == 0 {
		return md
	}

	if annotationsToPropagate, ok := obj.GetAnnotations()[PropagateAnnotationsAnnotation]; ok {
		md.Annotations = parseList(annotationsToPropagate, removeExcludedAnnotations(obj.GetAnnotations()))
	}

	if labelsToPropagate, ok := obj.GetAnnotations()[PropagateLabelsAnnotation]; ok {
		md.Labels = parseList(labelsToPropagate, obj.GetLabels())
	}

	return md
}

func removeExcludedAnnotations(annotations map[string]string) map[string]string {
	m := make(map[string]string, len(annotations))
	for k, v := range annotations {
		if k == corev1.LastAppliedConfigAnnotation || eckAnnotationsRegex.MatchString(k) {
			continue
		}
		m[k] = v
	}
	return m
}

func parseList(listStr string, existingValues map[string]string) map[string]string {
	listStr = strings.TrimSpace(listStr)
	if listStr == "" {
		return nil
	}

	if listStr == "*" {
		return maps.Clone(existingValues)
	}

	keys := strings.Split(listStr, ",")
	values := make(map[string]string, len(keys))

	for _, key := range keys {
		k := strings.TrimSpace(key)
		if v, ok := existingValues[k]; ok {
			values[k] = v
		}
	}

	return values
}
