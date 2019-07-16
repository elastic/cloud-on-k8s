// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	appsv1 "k8s.io/api/apps/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
)

// zen2VersionMatch returns true if the given Elasticsearch version is compatible with zen2.
func zen2VersionMatch(v version.Version) bool {
	return v.Major >= 7
}

// IsCompatibleForZen1 returns true if the given StatefulSet is compatible with zen2.
func IsCompatibleForZen2(statefulSet appsv1.StatefulSet) bool {
	return sset.ESVersionMatch(statefulSet, zen2VersionMatch)
}

// IsCompatibleForZen2 returns true if the given StatefulSetList contains at least one StatefulSet compatible with zen2.
func AtLeastOneNodeCompatibleForZen2(statefulSets sset.StatefulSetList) bool {
	return sset.AtLeastOneESVersionMatch(statefulSets, zen2VersionMatch)
}
