// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"math"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type upscaleState struct {
	isBootstrapped      bool
	allowMasterCreation bool
	recordedCreates     int32
	createsAllowed      int32
}

func newUpscaleState(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) (*upscaleState, error) {
	state := &upscaleState{
		isBootstrapped:      AnnotatedForBootstrap(ctx.es),
		allowMasterCreation: true,
		createsAllowed: calculateCreatesAllowed(
			ctx.es.Spec.UpdateStrategy.ChangeBudget,
			actualStatefulSets.ExpectedNodeCount(),
			expectedResources.StatefulSets().ExpectedNodeCount()),
	}

	if state.isBootstrapped {
		// is there a master node creation in progress already?
		masters, err := sset.GetActualMastersForCluster(ctx.k8sClient, ctx.es)
		if err != nil {
			return nil, err
		}
		for _, masterNodePod := range masters {
			isJoining, err := isMasterNodeJoining(masterNodePod, ctx.esState)
			if err != nil {
				return nil, err
			}
			if isJoining {
				state.recordMasterNodeCreation()
			}
		}
	}

	return state, nil
}

// calculateCreatesAllowed calculates how many replicas can we create according to desired state and maxSurge
func calculateCreatesAllowed(changeBudget *v1alpha1.ChangeBudget, actual, expected int32) int32 {
	var createsAllowed = int32(v1alpha1.DefaultChangeBudget.MaxSurge)
	if changeBudget != nil {
		createsAllowed = int32(changeBudget.MaxSurge)
		diff := expected - actual
		if diff > math.MaxInt32-createsAllowed {
			// we would overflow, so returning max here
			return math.MaxInt32
		}

		createsAllowed += diff
		if createsAllowed < 0 {
			createsAllowed = 0
		}
	}

	return createsAllowed
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
	left := s.createsAllowed - s.recordedCreates
	if left < noMoreThan {
		return left
	}

	return noMoreThan
}

func (s *upscaleState) limitNodesCreation(
	actual appsv1.StatefulSet,
	toApply appsv1.StatefulSet,
) appsv1.StatefulSet {
	if label.IsMasterNodeSet(toApply) {
		return s.limitMasterNodesCreation(actual, toApply)
	}

	actualReplicas := sset.GetReplicas(actual)
	targetReplicas := sset.GetReplicas(toApply)
	if actualReplicas == targetReplicas {
		return toApply
	}

	nodespec.UpdateReplicas(&toApply, common.Int32(actualReplicas))
	replicasToCreate := targetReplicas - actualReplicas
	replicasToCreate = s.getMaxNodesToCreate(replicasToCreate)
	if replicasToCreate > 0 {
		nodespec.UpdateReplicas(&toApply, common.Int32(actualReplicas+replicasToCreate))
		s.recordNodesCreation(replicasToCreate)
		ssetLogger(toApply).Info(
			"Creating nodes",
			"actualReplicas", actualReplicas,
			"replicasToCreate", replicasToCreate,
		)
	} else {
		ssetLogger(toApply).Info(
			"Limiting nodes creation to respect MaxSurge setting",
			"target", targetReplicas,
			"actual", actualReplicas,
		)
	}

	return toApply
}

func (s *upscaleState) limitMasterNodesCreation(
	actual appsv1.StatefulSet,
	toApply appsv1.StatefulSet,
) appsv1.StatefulSet {
	actualReplicas := sset.GetReplicas(actual)
	targetReplicas := sset.GetReplicas(toApply)

	nodespec.UpdateReplicas(&toApply, common.Int32(actualReplicas))
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
		nodespec.UpdateReplicas(&toApply, common.Int32(rep))
		s.recordMasterNodeCreation()
		ssetLogger(toApply).Info(
			"Creating master node",
			"actualReplicas", actualReplicas,
			"targetReplicas", rep,
		)
	}

	return toApply
}
