// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"fmt"
	"regexp"
)

type NodeLabels []*regexp.Regexp

func NewExposedNodeLabels(exposedNodeLabels []string) (NodeLabels, error) {
	if len(exposedNodeLabels) == 0 {
		return nil, nil
	}
	compiledNodeLabels := make([]*regexp.Regexp, len(exposedNodeLabels))
	for i, exposedNodeLabel := range exposedNodeLabels {
		r, err := regexp.Compile(exposedNodeLabel)
		if err != nil {
			return nil, fmt.Errorf("exposed node label \"%s\" cannot be compiled as a regular expression: %w", exposedNodeLabel, err)
		}
		compiledNodeLabels[i] = r
	}
	return compiledNodeLabels, nil
}

func (n NodeLabels) IsAllowed(nodeLabel string) bool {
	for _, r := range n {
		if r.MatchString(nodeLabel) {
			return true
		}
	}
	return false
}
