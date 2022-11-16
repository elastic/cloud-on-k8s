// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen2

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// zen2VersionMatch returns true if the given Elasticsearch versionCompatibleWithZen2 is compatible with zen2.
func versionCompatibleWithZen2(v version.Version) bool {
	return v.Major >= 7
}

// IsCompatibleWithZen2 returns true if the given StatefulSet is compatible with zen2.
func IsCompatibleWithZen2(ctx context.Context, statefulSet appsv1.StatefulSet) bool {
	return sset.ESVersionMatch(ctx, statefulSet, versionCompatibleWithZen2)
}

// AllMastersCompatibleWithZen2 returns true if all master nodes in the given cluster can use zen2 APIs.
// During a v6 -> v7 rolling upgrade, we can only call zen2 APIs once the current master is using v7,
// which would happen only if there is no more v6 master-eligible nodes in the cluster.
func AllMastersCompatibleWithZen2(c k8s.Client, es esv1.Elasticsearch) (bool, error) {
	masters, err := sset.GetActualMastersForCluster(c, es)
	if err != nil {
		return false, err
	}
	if len(masters) == 0 {
		return false, nil
	}
	for _, pod := range masters {
		v, err := label.ExtractVersion(pod.Labels)
		if err != nil {
			return false, err
		}
		if !versionCompatibleWithZen2(v) {
			return false, nil
		}
	}
	return true, nil
}
