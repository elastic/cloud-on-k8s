// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import "strconv"

const (
	// TypeLabelName used to represent a resource type in k8s resources
	TypeLabelName = "common.k8s.elastic.co/type"
)

// TrueFalseLabel is a label that has a true/false value.
type TrueFalseLabel string

// Set sets the given value of this label in the provided map
func (l TrueFalseLabel) Set(value bool, labels map[string]string) {
	labels[string(l)] = strconv.FormatBool(value)
}

// HasValue returns true if this label has the specified value in the provided map
func (l TrueFalseLabel) HasValue(value bool, labels map[string]string) bool {
	return labels[string(l)] == strconv.FormatBool(value)
}

// AsMap is a convenience method to create a map with this label set to a specific value
func (l TrueFalseLabel) AsMap(value bool) map[string]string {
	return map[string]string{
		string(l): strconv.FormatBool(value),
	}
}
