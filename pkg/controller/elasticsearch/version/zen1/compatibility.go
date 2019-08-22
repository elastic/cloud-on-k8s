// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	appsv1 "k8s.io/api/apps/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
)

// versionCompatibleWithZen1 returns true if the given Elasticsearch version is compatible with zen1.
func versionCompatibleWithZen1(v version.Version) bool {
	return v.Major < 7
}

// IsCompatibleWithZen1 returns true if the given StatefulSet is compatible with zen1.
func IsCompatibleWithZen1(statefulSet appsv1.StatefulSet) bool {
	return sset.ESVersionMatch(statefulSet, versionCompatibleWithZen1)
}

// AtLeastOneNodeCompatibleWithZen1 returns true if the given StatefulSetList contains
// at least one StatefulSet compatible with zen1.
func AtLeastOneNodeCompatibleWithZen1(statefulSets sset.StatefulSetList) bool {
	return sset.AtLeastOneESVersionMatch(statefulSets, versionCompatibleWithZen1)
}
