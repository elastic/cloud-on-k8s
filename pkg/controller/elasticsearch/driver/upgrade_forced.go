// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func (d *defaultDriver) MaybeForceUpgrade(actualStatefulSets sset.StatefulSetList, expectedMasters []string) (bool, error) {
	// Get the pods to upgrade
	podsToUpgrade, err := podsToUpgrade(d.Client, actualStatefulSets)
	if err != nil {
		return false, err
	}
	actualPods, err := actualStatefulSets.GetActualPods(d.Client)
	if err != nil {
		return false, err
	}

	return d.maybeForceUpgradePods(actualPods, podsToUpgrade, expectedMasters)
}

// isNonHACluster returns true if the expected and actual number of master nodes indicates that the quorum of that cluster
// does not allow the loss of any node in which case a regular rolling upgrade might not be possible especially when doing
// a major version upgrade.
func isNonHACluster(actualPods []corev1.Pod, expectedMasters []string) bool {
	if len(expectedMasters) > 2 {
		return false
	}
	actualMasters := label.FilterMasterNodePods(actualPods)
	return len(actualMasters) <= 2
}

// maybeForceUpgradePods may attempt a forced upgrade of all podsToUpgrade if allowed to,
// in order to unlock situations where the reconciliation may otherwise be stuck
// (eg. no cluster formed, all nodes have a bad spec).
func (d *defaultDriver) maybeForceUpgradePods(actualPods []corev1.Pod, podsToUpgrade []corev1.Pod, expectedMasters []string) (attempted bool, err error) {
	actualBySset := podsByStatefulSetName(actualPods)
	toUpgradeBySset := podsByStatefulSetName(podsToUpgrade)
	isNonHACluster := isNonHACluster(actualPods, expectedMasters)

	attempted = false

	for ssetName, actual := range actualBySset {
		toUpgrade, exists := toUpgradeBySset[ssetName]
		if !exists || len(toUpgrade) == 0 {
			continue
		}
		if !shouldForceUpgrade(actual, isNonHACluster) {
			continue
		}
		attempted = true
		log.Info("Performing a forced rolling upgrade",
			"namespace", d.ES.Namespace, "es_name", d.ES.Name,
			"statefulset_name", ssetName,
			"pod_count", len(podsToUpgrade),
		)
		for _, pod := range toUpgrade {
			if err := deletePod(d.Client, d.ES, pod, d.Expectations); err != nil {
				return attempted, err
			}
		}
	}

	return attempted, nil
}

func podsByStatefulSetName(pods []corev1.Pod) map[string][]corev1.Pod {
	byStatefulSet := map[string][]corev1.Pod{}
	for _, p := range pods {
		ssetName, exists := p.Labels[label.StatefulSetNameLabelName]
		if !exists {
			log.Error(
				pkgerrors.Errorf("expected label %s not set", label.StatefulSetNameLabelName),
				"skipping forced upgrade",
				"namespace", p.Namespace, "pod_name", p.Name)
			continue
		}
		if _, exists := byStatefulSet[ssetName]; !exists {
			byStatefulSet[ssetName] = []corev1.Pod{}
		}
		byStatefulSet[ssetName] = append(byStatefulSet[ssetName], p)
	}
	return byStatefulSet
}

// shouldForceUpgrade returns true if all existing Pods can be safely upgraded,
// without further safety checks.
// /!\ race condition: since the readiness is based on a cached value, we may allow
// a forced rolling upgrade to go through based on out-of-date Pod data.
func shouldForceUpgrade(pods []corev1.Pod, nonHACluster bool) bool {
	return nonHACluster || allPodsPending(pods) || allPodsBootlooping(pods)
}

func allPodsPending(pods []corev1.Pod) bool {
	for _, p := range pods {
		if p.Status.Phase != corev1.PodPending {
			return false
		}
	}
	return true
}

func allPodsBootlooping(pods []corev1.Pod) bool {
	for _, p := range pods {
		if k8s.IsPodReady(p) {
			// the Pod seems healthy
			return false
		}
		for _, containerStatus := range p.Status.ContainerStatuses {
			if containerStatus.Name == esv1.ElasticsearchContainerName &&
				containerStatus.RestartCount == 0 {
				// the Pod may not be healthy, but it has not restarted (yet)
				return false
			}
		}
	}
	return true
}
