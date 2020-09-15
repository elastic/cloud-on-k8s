// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"sync"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type upscaleState struct {
	isBootstrapped      bool
	allowMasterCreation bool
	// indicates how many creates, out of createsAllowed, were already recorded
	recordedCreates int32
	// indicates how many creates are allowed when taking into account maxSurge setting,
	// nil indicates that any number of pods can be created, negative value is not expected.
	createsAllowed *int32
	ctx            upscaleCtx
	once           *sync.Once
}

func newUpscaleState(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) *upscaleState {
	return &upscaleState{
		once: &sync.Once{},
		ctx:  ctx,
		createsAllowed: calculateCreatesAllowed(
			ctx.es.Spec.UpdateStrategy.ChangeBudget.GetMaxSurgeOrDefault(),
			actualStatefulSets.ExpectedNodeCount(),
			expectedResources.StatefulSets().ExpectedNodeCount()),
	}
}

func buildOnce(s *upscaleState) error {
	if s.once == nil {
		return nil
	}

	var result error
	s.once.Do(func() {
		s.isBootstrapped = bootstrap.AnnotatedForBootstrap(s.ctx.es)
		s.allowMasterCreation = true

		if s.isBootstrapped {
			// is there a master node creation in progress already?
			masters, err := sset.GetActualMastersForCluster(s.ctx.k8sClient, s.ctx.es)
			if err != nil {
				result = err
				return
			}
			for _, masterNodePod := range masters {
				var isJoining bool
				isJoining, err = isMasterNodeJoining(masterNodePod, s.ctx.esState)
				if err != nil {
					result = err
					return
				}
				if isJoining {
					s.recordMasterNodeCreation()
				}
			}
		}
	})

	return result
}

// calculateCreatesAllowed calculates how many replicas can we create according to desired state and maxSurge
func calculateCreatesAllowed(maxSurge *int32, actual, expected int32) *int32 {
	if maxSurge == nil {
		// unbounded
		return nil
	}

	createsAllowed := *maxSurge + expected - actual
	if createsAllowed < 0 {
		createsAllowed = 0
	}

	return &createsAllowed
}

func isMasterNodeJoining(pod corev1.Pod, esState ESState) (bool, error) {
	// Consider a master node to be in the process of joining the cluster if either:

	// - Pending (pod not started yet)
	if pod.Status.Phase == corev1.PodPending {
		return true, nil
	}

	// - Running but not Ready (ES process still starting)
	if pod.Status.Phase == corev1.PodRunning && !k8s.IsPodReady(pod) {
		return true, nil
	}

	// - Running & Ready but not part of the cluster
	if pod.Status.Phase == corev1.PodRunning && k8s.IsPodReady(pod) {
		// This does a synchronous request to Elasticsearch.
		// Relying instead on a previous (out of date) observed ES state would risk a mismatch
		// if a node was removed then re-added into the cluster.
		inCluster, err := esState.NodesInCluster([]string{pod.Name})
		if err != nil {
			return false, err
		}
		if !inCluster {
			return true, nil
		}
	}

	// Otherwise, consider the pod is not in the process of joining the cluster.
	// It's either already running (and has joined), or in an error state.
	return false, nil
}

func (s *upscaleState) recordMasterNodeCreation() {
	// if the cluster is already formed, don't allow more master nodes to be created
	if s.isBootstrapped {
		s.allowMasterCreation = false
	}
	s.recordNodesCreation(1)
}

func (s *upscaleState) canCreateMasterNode() bool {
	return s.getMaxNodesToCreate(1) == 1 && s.allowMasterCreation
}

func (s *upscaleState) recordNodesCreation(count int32) {
	s.recordedCreates += count
}

func (s *upscaleState) getMaxNodesToCreate(noMoreThan int32) int32 {
	if s.createsAllowed == nil {
		// unbounded, so allow all that was requested
		return noMoreThan
	}

	left := *s.createsAllowed - s.recordedCreates
	if left < noMoreThan {
		return left
	}

	return noMoreThan
}

// limitNodesCreation decreases replica count in specs as needed, assumes an upscale is requested
func (s *upscaleState) limitNodesCreation(
	actualReplicas int32,
	toApply appsv1.StatefulSet,
) (appsv1.StatefulSet, error) {
	if err := buildOnce(s); err != nil {
		return appsv1.StatefulSet{}, err
	}

	if label.IsMasterNodeSet(toApply) {
		return s.limitMasterNodesCreation(actualReplicas, toApply)
	}

	targetReplicas := sset.GetReplicas(toApply)

	nodespec.UpdateReplicas(&toApply, pointer.Int32(actualReplicas))
	replicasToCreate := targetReplicas - actualReplicas
	replicasToCreate = s.getMaxNodesToCreate(replicasToCreate)

	if replicasToCreate > 0 {
		nodespec.UpdateReplicas(&toApply, pointer.Int32(actualReplicas+replicasToCreate))
		s.recordNodesCreation(replicasToCreate)
		ssetLogger(toApply).Info(
			"Creating nodes",
			"actualReplicas", actualReplicas,
			"replicasToCreate", replicasToCreate,
		)
	} else {
		ssetLogger(toApply).Info(
			"Limiting nodes creation to respect maxSurge setting",
			"target", targetReplicas,
			"actual", actualReplicas,
		)
	}

	return toApply, nil
}

func (s *upscaleState) limitMasterNodesCreation(
	actualReplicas int32,
	toApply appsv1.StatefulSet,
) (appsv1.StatefulSet, error) {
	targetReplicas := sset.GetReplicas(toApply)

	nodespec.UpdateReplicas(&toApply, pointer.Int32(actualReplicas))
	for rep := actualReplicas + 1; rep <= targetReplicas; rep++ {
		if !s.canCreateMasterNode() {
			ssetLogger(toApply).Info(
				"Limiting master nodes creation to one at a time",
				"target", targetReplicas,
				"actual", actualReplicas,
			)
			break
		}
		// allow one more master node to be created
		nodespec.UpdateReplicas(&toApply, pointer.Int32(rep))
		s.recordMasterNodeCreation()
		ssetLogger(toApply).Info(
			"Creating master node",
			"actualReplicas", actualReplicas,
			"targetReplicas", rep,
		)
	}

	return toApply, nil
}
