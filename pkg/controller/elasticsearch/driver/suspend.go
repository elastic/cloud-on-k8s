// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func reconcileSuspendedPods(c k8s.Client, es esv1.Elasticsearch, e *expectations.Expectations) error {
	if satisfied, err := e.DeletionsSatisfied(); err != nil || !satisfied {
		if !satisfied {
			log.Info("Not reconciling Pod suspensions as deletion expectations are not satisfied")
		}
		return err
	}

	suspendedPodNames := es.SuspendedPodNames()

	statefulSets, err := sset.RetrieveActualStatefulSets(c, k8s.ExtractNamespacedName(&es))
	if err != nil {
		return err
	}
	knownPods, err := statefulSets.GetActualPods(c)
	if err != nil {
		return err
	}

	for _, pod := range knownPods {
		if suspendedPodNames.Has(pod.Name) {
			for _, s := range pod.Status.ContainerStatuses {
				// delete the Pod without grace period if the main container is running
				if s.Name == esv1.ElasticsearchContainerName && s.State.Running != nil {
					log.Info("Deleting suspended pod", "pod_name", pod.Name, "pod_uid", pod.UID,
						"namespace", es.Namespace, "es_name", es.Name)
					e.ExpectDeletion(pod)
					if err := c.Delete(context.Background(), &pod, client.GracePeriodSeconds(0)); err != nil {
						return err
					}
				}
			}
		} else if isSuspended(pod) {
			// try to speed up propagation of configmap entries
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
