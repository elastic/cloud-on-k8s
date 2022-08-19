// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	"github.com/go-logr/logr"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

func (d *defaultDriver) MaybeForceUpgrade(ctx context.Context, statefulSets sset.StatefulSetList) (bool, error) {
	// Get the pods to upgrade
	podsToUpgrade, err := podsToUpgrade(d.Client, statefulSets)
	if err != nil {
		return false, err
	}
	actualPods, err := statefulSets.GetActualPods(d.Client)
	if err != nil {
		return false, err
	}
	return d.maybeForceUpgradePods(ctx, actualPods, podsToUpgrade)
}

// maybeForceUpgradePods may attempt a forced upgrade of all podsToUpgrade if allowed to,
// in order to unlock situations where the reconciliation may otherwise be stuck
// (eg. no cluster formed, all nodes have a bad spec).
func (d *defaultDriver) maybeForceUpgradePods(ctx context.Context, actualPods []corev1.Pod, podsToUpgrade []corev1.Pod) (attempted bool, err error) {
	log := ulog.FromContext(ctx)
	actualBySset := podsByStatefulSetName(actualPods, log)
	toUpgradeBySset := podsByStatefulSetName(podsToUpgrade, log)

	attempted = false

	for ssetName, actual := range actualBySset {
		toUpgrade, exists := toUpgradeBySset[ssetName]
		if !exists || len(toUpgrade) == 0 {
			continue
		}
		if !shouldForceUpgrade(actual) {
			continue
		}
		attempted = true
		log.Info("Performing a forced rolling upgrade",
			"namespace", d.ES.Namespace, "es_name", d.ES.Name,
			"statefulset_name", ssetName,
			"pod_count", len(podsToUpgrade),
		)
		for _, pod := range toUpgrade {
			if err := deletePod(ctx, d.Client, d.ES, pod, d.Expectations, d.ReconcileState, "Deleting Pod for forced rolling upgrade"); err != nil {
				return attempted, err
			}
		}
	}

	return attempted, nil
}

func podsByStatefulSetName(pods []corev1.Pod, log logr.Logger) map[string][]corev1.Pod {
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
func shouldForceUpgrade(pods []corev1.Pod) bool {
	return allPodsPending(pods) || allPodsBootlooping(pods)
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
