// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeletionDriver is a controller that given a context uses a plugable strategy
// to determine which Pods can be safely deleted.
type DeletionDriver interface {
	// Delete goes thought the candidates and actually deletes the ones
	Delete(candidates []*corev1.Pod) (deletedPods []*corev1.Pod, err error)
}

// DefaultDeletionDriver is the default deletion driver.
type DefaultDeletionDriver struct {
	client       k8s.Client
	esClient     esclient.Client
	es           *v1alpha1.Elasticsearch
	statefulSets sset.StatefulSetList
	state        ESState
	healthyPods  map[string]corev1.Pod
	strategy     DeletionStrategy
	expectations *expectations.Expectations
}

// NewDeletionDriver creates a new default deletion driver.
func NewDeletionDriver(
	client k8s.Client,
	esClient esclient.Client,
	es *v1alpha1.Elasticsearch,
	statefulSets sset.StatefulSetList,
	state ESState,
	masterNodesNames []string,
	healthyPods map[string]corev1.Pod,
	podsToUpgrade []corev1.Pod,
	expectations *expectations.Expectations,
) *DefaultDeletionDriver {
	return &DefaultDeletionDriver{
		client:       client,
		esClient:     esClient,
		es:           es,
		statefulSets: statefulSets,
		state:        state,
		healthyPods:  healthyPods,
		expectations: expectations,
		strategy:     NewDefaultDeletionStrategy(state, healthyPods, podsToUpgrade, masterNodesNames),
	}
}

// Delete runs through a list of potential candidates and select the ones that can be deleted.
// Do not run this function unless driver expectations are met.
func (d *DefaultDeletionDriver) Delete(candidates []corev1.Pod) (deletedPods []corev1.Pod, err error) {
	if len(candidates) == 0 {
		return deletedPods, nil
	}
	es := k8s.ExtractNamespacedName(d.es)

	// Check if we are not over disruption budget
	// Upscale is done, we should have the required number of Pods
	statefulSets, err := sset.RetrieveActualStatefulSets(d.client, es)
	if err != nil {
		return deletedPods, err
	}
	expectedPods := statefulSets.PodNames()
	unhealthyPods := len(expectedPods) - len(d.healthyPods)
	maxUnavailable := 1
	// TODO: use GroupingDefinition
	if d.es.Spec.UpdateStrategy.ChangeBudget != nil {
		maxUnavailable = d.es.Spec.UpdateStrategy.ChangeBudget.MaxUnavailable
	}
	allowedDeletions := maxUnavailable - unhealthyPods
	// If maxUnavailable is reached the deletion driver still allows one unhealthy Pod to be restarted.
	// TODO: Should we make the difference between MaxUnavailable and MaxConcurrentRestarting ?
	maxUnavailableReached := (maxUnavailable - unhealthyPods) <= 0

	// Step 2. Sort the Pods to get the ones with the higher priority
	err = d.strategy.SortFunction()(candidates, d.state)
	if err != nil {
		return deletedPods, err
	}

	// Step 3: Apply predicates
	predicates := d.strategy.Predicates()
	for _, candidate := range candidates {
		if ok, err := runPredicates(candidate, deletedPods, predicates, maxUnavailableReached); err != nil {
			return deletedPods, err
		} else if ok {
			candidate := candidate
			// Remove from healthy nodes if it was there
			delete(d.healthyPods, candidate.Name)
			// Append to the deletedPods list
			deletedPods = append(deletedPods, candidate)
			allowedDeletions = allowedDeletions - 1
			if allowedDeletions <= 0 {
				break
			}
		}
	}

	if len(deletedPods) == 0 {
		log.Info("no pod deleted", "es_name", d.es.Name, "es_namespace", d.es.Namespace)
		return deletedPods, nil
	}

	// Disable shard allocation
	if err := d.prepareClusterForNodeRestart(d.esClient, d.state); err != nil {
		return deletedPods, err
	}
	// TODO: If master is changed into a data node (or the opposite) it must be excluded or we should update m_m_n
	for _, deletedPod := range deletedPods {
		d.expectations.ExpectDeletion(deletedPod)
		err := d.delete(&deletedPod)
		if err != nil {
			d.expectations.CancelExpectedDeletion(deletedPod)
			return deletedPods, err
		}
	}

	return deletedPods, nil
}

func (d *DefaultDeletionDriver) delete(pod *corev1.Pod) error {
	uid := pod.UID
	log.Info("delete pod", "es_name", d.es.Name, "es_namespace", d.es.Namespace, "pod_name", pod.Name, "pod_uid", pod.UID)
	return d.client.Delete(pod, func(options *client.DeleteOptions) {
		if options.Preconditions == nil {
			options.Preconditions = &metav1.Preconditions{}
		}
		options.Preconditions.UID = &uid
	})
}

func runPredicates(
	candidate corev1.Pod,
	deletedPods []corev1.Pod,
	predicates map[string]Predicate,
	maxUnavailableReached bool,
) (bool, error) {
	for name, predicate := range predicates {
		canDelete, err := predicate(candidate, deletedPods, maxUnavailableReached)
		if err != nil {
			return false, err
		}
		if !canDelete {
			log.Info("predicate failed", "pod_name", candidate.Name, "predicate_name", name)
			//skip this Pod, it can't be deleted for the moment
			return false, nil
		}
	}
	// All predicates passed !
	return true, nil
}

func (d *DefaultDeletionDriver) prepareClusterForNodeRestart(esClient esclient.Client, esState ESState) error {
	// Disable shard allocations to avoid shards moving around while the node is temporarily down
	shardsAllocationEnabled, err := esState.ShardAllocationsEnabled()
	if err != nil {
		return err
	}
	if shardsAllocationEnabled {
		log.Info("Disabling shards allocation", "es_name", d.es.Name, "es_namespace", d.es.Namespace)
		if err := disableShardsAllocation(esClient); err != nil {
			return err
		}
	}

	// Request a sync flush to optimize indices recovery when the node restarts.
	if err := doSyncFlush(esClient); err != nil {
		return err
	}

	// TODO: halt ML jobs on that node
	return nil
}
