// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func reconcileSuspendedPods(c k8s.Client, es esv1.Elasticsearch, e *expectations.Expectations) error {
	// let's make sure we observe any deletions in the cache to avoid redundant deletion
	deletionsSatisfied, err := e.DeletionsSatisfied();
	if err != nil {
		return err
	}

	// suspendedPodNames as indicated by the user on the Elasticsearch resource via an annotation
	suspendedPodNames := es.SuspendedPodNames()

	// all known Pods, this is mostly to fine tune the reconciliation to the current state of the Pods, see below
	statefulSets, err := sset.RetrieveActualStatefulSets(c, k8s.ExtractNamespacedName(&es))
	if err != nil {
		return err
	}
	knownPods, err := statefulSets.GetActualPods(c)
	if err != nil {
		return err
	}

	for i, pod := range knownPods {
		// Pod should be suspended and we have seen all deletions i.e. cache is up to date
		if suspendedPodNames.Has(pod.Name) && deletionsSatisfied {
			for _, s := range pod.Status.ContainerStatuses {
				// delete the Pod without grace period if the main Elasticsearch container is running
				if s.Name == esv1.ElasticsearchContainerName && s.State.Running != nil {
					log.Info("Deleting suspended pod", "pod_name", pod.Name, "pod_uid", pod.UID,
						"namespace", es.Namespace, "es_name", es.Name)
					if err := c.Delete(context.Background(), &knownPods[i], client.GracePeriodSeconds(0)); err != nil {
						return err
					}
					// record the expected deletion
					e.ExpectDeletion(pod)				}
			}
		// Pod is suspended but it should not be
		} else if isSuspended(pod) {
			// try to speed up propagation of config map entries so that it can start up again. Without this it can take
			// minutes to until the config map file in the Pod's filesystem is updated with the current state
			annotation.MarkPodAsUpdated(c, pod)
		}
	}
	return nil
}

func isSuspended(pod corev1.Pod) bool {
	for _, s := range pod.Status.InitContainerStatuses {
		if s.Name == initcontainer.SuspendContainerName && s.State.Running != nil {
			return true
		}
	}
	return false
}
