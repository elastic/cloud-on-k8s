// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

// NotAllowedNodesLabelMsg is returned when a node label requested via the downward-node-labels
// annotation is not allowed by the operator's exposed-node-labels policy.
const NotAllowedNodesLabelMsg = "Node label not in the exposed node labels list"

// NodeLabels is the compiled form of the operator-level exposed-node-labels policy. An empty value
// disables propagation of any node label.
type NodeLabels []*regexp.Regexp

// NewExposedNodeLabels compiles the given patterns into a NodeLabels policy.
func NewExposedNodeLabels(exposedNodeLabels []string) (NodeLabels, error) {
	if len(exposedNodeLabels) == 0 {
		return nil, nil
	}
	compiled := make([]*regexp.Regexp, len(exposedNodeLabels))
	for i, p := range exposedNodeLabels {
		r, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("exposed node label %q cannot be compiled as a regular expression: %w", p, err)
		}
		compiled[i] = r
	}
	return compiled, nil
}

// IsAllowed returns whether the given node label is permitted by the policy.
func (n NodeLabels) IsAllowed(nodeLabel string) bool {
	for _, r := range n {
		if r.MatchString(nodeLabel) {
			return true
		}
	}
	return false
}

// ValidateAnnotation checks that every label declared via the downward-node-labels annotation
// on the resource is allowed by the operator's exposed-node-labels policy. The check is a no-op
// when no annotation is set.
func ValidateAnnotation(annotations map[string]string, exposedNodeLabels NodeLabels) field.ErrorList {
	var errs field.ErrorList
	for _, nodeLabel := range commonv1.DownwardNodeLabelsFromAnnotations(annotations) {
		if exposedNodeLabels.IsAllowed(nodeLabel) {
			continue
		}
		errs = append(errs, field.Invalid(
			field.NewPath("metadata").Child("annotations", commonv1.DownwardNodeLabelsAnnotation),
			nodeLabel,
			NotAllowedNodesLabelMsg,
		))
	}
	return errs
}
