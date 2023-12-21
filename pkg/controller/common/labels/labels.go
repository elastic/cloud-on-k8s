// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package labels

import (
	"strconv"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

const (
	credentialsLabel = "eck.k8s.elastic.co/credentials" //nolint:gosec
	// StackConfigPolicyOnDeleteLabelName is a label used to indicate if an object should be reset or deleted on deletion of its stack config policy.
	StackConfigPolicyOnDeleteLabelName = "asset.policy.k8s.elastic.co/on-delete"
	// OrphanSecretResetOnPolicyDelete is used to set the data field of a secret to an empty map when the associated StackConfigPolicy or Elasticsearch is deleted.
	OrphanSecretResetOnPolicyDelete = "reset"
	// OrphanSecretDeleteOnPolicyDelete is used to delete the secret when the associated StackConfigPolicy or Elasticsearch is deleted.
	OrphanSecretDeleteOnPolicyDelete = "delete"
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

// AddCredentialsLabel adds a label used to describe a resource which contains some credentials, either a clear-text password or a token.
func AddCredentialsLabel(original map[string]string) map[string]string {
	return maps.Merge(map[string]string{credentialsLabel: "true"}, original)
}
