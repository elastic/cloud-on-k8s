// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// maybeForceUpgrade may attempt a forced upgrade of all podsToUpgrade if allowed to,
// in order to unlock situations where the reconciliation may otherwise be stuck
// (eg. no cluster formed, all nodes have a bad spec).
func (d *defaultDriver) maybeForceUpgrade(actualPods []corev1.Pod, podsToUpgrade []corev1.Pod) (attempted bool, err error) {
	attempted = false
	if len(podsToUpgrade) == 0 {
		return attempted, nil
	}
	if !shouldForceUpgrade(actualPods) {
		return attempted, nil
	}

	attempted = true
	log.Info("Performing a forced rolling upgrade since no Pod is ready",
		"namespace", d.ES.Namespace, "es_name", d.ES.Name,
		"pod_count", len(podsToUpgrade),
	)
	for _, pod := range podsToUpgrade {
		if err := deletePod(d.Client, d.ES, pod, d.Expectations); err != nil {
			return attempted, err
		}
	}
	return attempted, nil
}

// shouldForceUpgrade returns true if all existing Pods can be safely upgraded,
// without further safety checks.
// In practice, we're OK to force-upgrade if all Pods are non-ready.
// /!\ race condition: since the readiness is based on a cached value, we may allow
// a forced rolling upgrade to go through based on out-of-date Pod data.
func shouldForceUpgrade(pods []corev1.Pod) bool {
	for _, p := range pods {
		if k8s.IsPodReady(p) {
			// at least one pod is ready
			return false
		}
	}
	// all pods are not ready
	return true
}
