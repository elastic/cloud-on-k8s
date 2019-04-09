// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	// randomSuffixLength represents the length of the random suffix that is appended in NewNodeName.
	randomSuffixLength = 10
)

// NewNodeName forms an Elasticsearch node name. Returning a unique node
// node name to be used for the Elasticsearch cluster node.
func NewNodeName(clusterName string) string {
	uniqueSuffix := stringsutil.Concat(name.PodSuffix, "-", rand.String(randomSuffixLength))
	return name.Suffix(clusterName, uniqueSuffix)
}
