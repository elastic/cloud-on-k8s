// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/pkg/errors"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/migration"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/restart"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"

	corev1 "k8s.io/api/core/v1"
)

type PodDeletionHandler struct {
	es                 v1alpha1.Elasticsearch
	performableChanges *mutation.PerformableChanges
	results            *reconciler.Results
	observedState      observer.State
	reconcileState     *reconcile.State
	esClient           esclient.Client
	resourcesState     *reconcile.ResourcesState
	defaultDriver      *defaultDriver
}

func (d *PodDeletionHandler) HandleDeletions() error {
	if len(d.performableChanges.ToDelete) == 0 {
		// nothing to do
		return nil
	}

	// Start by deleting pods whose PVC should be kept around
	if err := d.deleteForPVCReuse(d.performableChanges.ToDelete.WithPVCReuse()); err != nil {
		return err
	}

	// Then delete pods which will not be replaced hence need data to be migrated
	if err := d.deleteWithDataMigration(d.performableChanges.ToDelete.WithoutPVCReuse()); err != nil {
		return err
	}

	return nil
}

func (d *PodDeletionHandler) deleteForPVCReuse(pods mutation.PodsToDelete) error {
	if len(pods) == 0 {
		return nil
	}

	// Safety check we can move on with pod deletion
	// TODO: check in-between nodes if there is more than 1 to delete, otherwise that
	//  check may not be valid for the 2nd node after the first one is deleted
	if !d.clusterReadyForNodesRestart() {
		d.results.WithResult(defaultRequeue) // try again later
		return nil
	}

	// Prepare cluster for nodes restart
	if err := restart.PrepareClusterForNodesStop(d.esClient); err != nil {
		return err
	}

	// Delete pods while keeping PVCs for reuse
	if err := d.deletePods(pods, false); err != nil {
		return err
	}

	return nil
}

func (d *PodDeletionHandler) deleteWithDataMigration(pods mutation.PodsToDelete) error {
	if len(pods) == 0 {
		return nil
	}

	// Start migrating data away from all pods to be deleted
	leavingNodeNames := pod.PodListToNames(d.performableChanges.ToDelete.WithoutPVCReuse().Pods())
	if err := migration.MigrateData(d.esClient, leavingNodeNames); err != nil {
		return errors.Wrap(err, "error during migrate data")
	}

	// Only delete pods whose data migration is over
	canDelete := make(mutation.PodsToDelete, 0, len(pods))
	for _, p := range pods {
		if migration.IsMigratingData(d.observedState, p.Pod, pods.Pods()) {
			log.Info("Skipping deletion because data migration is not over yet", "pod", p.Pod.Name)
			d.reconcileState.UpdateElasticsearchMigrating(*d.resourcesState, d.observedState)
			// Requeue to make sure that node is eventually deleted
			d.results.WithResult(defaultRequeue)
			continue
		}
		canDelete = append(canDelete, p)
	}

	// Delete pods and PVCs
	if err := d.deletePods(canDelete, true); err != nil {
		return err
	}

	return nil
}

func (d *PodDeletionHandler) deletePods(toDelete mutation.PodsToDelete, deletePVC bool) error {
	// propagate resources state to each individual deletion,
	// accounting for previous pods being deleted
	newState := make([]corev1.Pod, len(d.resourcesState.CurrentPods))
	copy(newState, d.resourcesState.CurrentPods.Pods())
	for _, p := range toDelete {
		newState = removePodFromList(newState, p.Pod)

		// update discovery settings right before removing the pod
		preDelete := func() error {

			// update zen1 settings
			if d.defaultDriver.zen1SettingsUpdater != nil {
				requeue, err := d.defaultDriver.zen1SettingsUpdater(
					d.es,
					d.defaultDriver.Client,
					d.esClient,
					newState,
					d.performableChanges.ToCreate,
					d.reconcileState)
				if err != nil {
					return err
				}
				if requeue {
					d.results.WithResult(defaultRequeue)
				}
			}
			return nil
		}

		esRef := k8s.ExtractNamespacedName(&d.es)
		// expect a pod deletion in the cache
		d.defaultDriver.PodsExpectations.ExpectDeletion(esRef)
		// delete the pod
		log.Info("Deleting pod", "pod", p.Pod.Name, "deletePvc", deletePVC)
		result, err := deleteElasticsearchPod(
			d.defaultDriver.Client,
			d.reconcileState,
			*d.resourcesState,
			p.Pod,
			preDelete,
			deletePVC,
		)
		if err != nil {
			// pod was not deleted, cancel our expectation by marking it observed
			d.defaultDriver.PodsExpectations.DeletionObserved(esRef)
			return err
		}
		d.results.WithResult(result)
	}
	return nil
}

// canDeletePodsForPVCReuse inspects the cluster state to decide if we can proceed with
// pod deletion for PVC reuse.
func (d *PodDeletionHandler) clusterReadyForNodesRestart() bool {
	// Safety check: move on with nodes shutdown only if the cluster is green
	// TODO: optimize by doing a per-node check instead of a cluster-wide health check:
	//  are there other copies of the node shards elsewhere?
	if d.observedState.ClusterHealth == nil {
		return false
	}
	health := v1alpha1.ElasticsearchHealth(d.observedState.ClusterHealth.Status)
	// This does not account for indices with no replicas: cluster would still be green even though
	// no other copy of the shard exist. At this point we can consider this is the user's responsibility.
	if health != v1alpha1.ElasticsearchGreenHealth {
		log.Info("Waiting for green cluster health before deleting pods for PVC reuse", "health", health)
		return false
	}
	return true
}

// removePodFromList removes a single pod from the list, matching by pod name.
func removePodFromList(pods []corev1.Pod, pod corev1.Pod) []corev1.Pod {
	for i, p := range pods {
		if p.Name == pod.Name {
			return append(pods[:i], pods[i+1:]...)
		}
	}
	return pods
}
