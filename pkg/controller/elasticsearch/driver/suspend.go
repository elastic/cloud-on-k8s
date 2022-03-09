// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// reconcileSuspendedPods implements the operator side of activating the Pod suspension mechanism:
// - Users annotate the Elasticsearch resource with names of Pods they want to suspend for debugging purposes.
// - Each Pod has an initContainer that runs a shell script to check a file backed by a configMap for its own Pod name.
// - If the name of the Pod is found in the file the initContainer enters a loop preventing termination until the name
//   of the Pod is removed from the file again. The Pod is now "suspended".
// - This function handles the case where the Pod is either already running the main container or it is currently suspended.
// - If the Pod is already running but should be suspended we want to delete the Pod so that the recreated Pod can run
//   the initContainer again.
// - If the Pod is suspended in the initContainer but should be running we update the Pods metadata to accelerate the
//   propagation of the configMap values. This is just an optimisation and not essential for the correct operation of
//   the feature.
func reconcileSuspendedPods(c k8s.Client, es esv1.Elasticsearch, e *expectations.Expectations) error {
	// let's make sure we observe any deletions in the cache to avoid redundant deletion
	pendingPodDeletions, err := e.PendingPodDeletions()
	if err != nil {
		return err
	}
	deletionsSatisfied := len(pendingPodDeletions) == 0

	// suspendedPodNames as indicated by the user on the Elasticsearch resource via an annotation
	// the corresponding configMap has already been reconciled prior to that function
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
		// Pod should be suspended
		if suspendedPodNames.Has(pod.Name) {
			for _, s := range pod.Status.ContainerStatuses {
				// delete the Pod without grace period if the main Elasticsearch container is running
				// and we have seen all expected deletions in the cache
				if deletionsSatisfied && s.Name == esv1.ElasticsearchContainerName && s.State.Running != nil {
					log.Info("Deleting suspended pod", "pod_name", pod.Name, "pod_uid", pod.UID,
						"namespace", es.Namespace, "es_name", es.Name)
					// the precondition serves as an additional barrier in addition to the expectation mechanism to
					// not accidentally deleting Pods we do not intent to delete (because our view of the world is out of sync)
					preconditions := client.Preconditions{
						UID:             &pod.UID,
						ResourceVersion: &pod.ResourceVersion,
					}
					if err := c.Delete(context.Background(), &knownPods[i], preconditions, client.GracePeriodSeconds(0)); err != nil {
						return err
					}
					// record the expected deletion
					e.ExpectDeletion(pod)
				}
			}
		} else if isSuspended(pod) {
			// Pod is suspended. But it should not be. Try to speed up propagation of config map entries so that it can
			// start up again. Without this it can take minutes until the config map file in the Pod's filesystem is
			// updated with the current state.
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
