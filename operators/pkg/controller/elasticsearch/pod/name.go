// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	// typeSuffix represents the Elasticsearch shortened suffix that is
	// interpolated in NewNodeName.
	typeSuffix = "-es"
	// randomSuffixLength represents the length of the random suffix that is
	// appended in NewNodeName.
	randomSuffixLength = 10
	// k8s object name has a maximum length of 63, we're substracting the
	// randomSuffix and the interpolated type suffix +1 which accounts
	// for the extra `-` in the interpolation.
	maxPrefixLength = 63 - randomSuffixLength - 1 - len(typeSuffix)
)

// NewNodeName forms an Elasticsearch node name. Returning a unique node
// node name to be used for the Elasticsearch cluster node.
func NewNodeName(clusterName string) string {
	var prefix = clusterName
	if len(prefix) > maxPrefixLength {
		prefix = prefix[:maxPrefixLength]
	}

	return stringsutil.Concat(
		prefix,
		typeSuffix,
		"-",
		rand.String(randomSuffixLength),
	)
}
