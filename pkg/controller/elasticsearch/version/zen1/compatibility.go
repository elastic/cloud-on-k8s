// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// versionCompatibleWithZen1 returns true if the given Elasticsearch version is compatible with zen1.
func versionCompatibleWithZen1(v version.Version) bool {
	return v.Major < 7
}

// IsCompatibleWithZen1 returns true if the given StatefulSet is compatible with zen1.
func IsCompatibleWithZen1(statefulSet appsv1.StatefulSet) bool {
	return sset.ESVersionMatch(statefulSet, versionCompatibleWithZen1)
}

// AtLeastOneNodeCompatibleWithZen1 returns true if at least one of the following conditions is true:
// 1. There is at least one 6.x node in the actual masters.
// 2. The given StatefulSetList contains at least one StatefulSet compatible with zen1.
func AtLeastOneNodeCompatibleWithZen1(
	statefulSets sset.StatefulSetList,
	c k8s.Client,
	es esv1.Elasticsearch,
) (bool, error) {
	actualMasters, err := sset.GetActualMastersForCluster(c, es)
	if err != nil {
		return false, err
	}
	zen1PodExists, err := atLeasOnePodCompatibleWithZen1(actualMasters)
	if err != nil {
		return false, err
	}
	if zen1PodExists {
		return true, nil
	}
	return sset.AtLeastOneESVersionMatch(statefulSets, versionCompatibleWithZen1), nil
}

func atLeasOnePodCompatibleWithZen1(pods []corev1.Pod) (bool, error) {
	for _, pod := range pods {
		version, err := label.ExtractVersion(pod.Labels)
		if err != nil {
			return false, err
		}
		if versionCompatibleWithZen1(*version) {
			return true, nil
		}
	}
	return false, nil
}
