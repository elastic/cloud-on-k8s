// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"math"
	"sync"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type upscaleStateBuilder struct {
	once         *sync.Once
	upscaleState *upscaleState
}

type upscaleState struct {
	isBootstrapped      bool
	allowMasterCreation bool
	accountedNodes      int32
	nodesToCreate       int32
}

func newUpscaleState(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) (*upscaleState, error) {
	state := &upscaleState{
		isBootstrapped:      AnnotatedForBootstrap(ctx.es),
		allowMasterCreation: true,
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

	// count max replica count allowed by desired state and maxSurge
	if ctx.es.Spec.UpdateStrategy.ChangeBudget != nil {
		state.nodesToCreate = int32(ctx.es.Spec.UpdateStrategy.ChangeBudget.MaxSurge)
		for _, nodeSpecRes := range expectedResources {
			state.nodesToCreate += sset.GetReplicas(nodeSpecRes.StatefulSet)
		}
		for _, nodeSpecRes := range actualStatefulSets {
			state.nodesToCreate -= sset.GetReplicas(nodeSpecRes)
		}
		if state.nodesToCreate < 0 {
			state.nodesToCreate = 0
		}
	} else {
		state.nodesToCreate = math.MaxInt32
	}

	return state, nil
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

func (s *upscaleState) canAddMasterNode() bool {
	return s.getMaxNodesToAdd(1) == 1 && s.allowMasterCreation
}

func (s *upscaleState) recordNodesCreation(count int32) {
	s.accountedNodes += count
}

func (s *upscaleState) getMaxNodesToAdd(noMoreThan int32) int32 {
	left := s.nodesToCreate - s.accountedNodes
	if left < noMoreThan {
		return left
	}

	return noMoreThan
}

func (s *upscaleState) limitNodesCreation(
	actual appsv1.StatefulSet,
	toApply appsv1.StatefulSet,
) appsv1.StatefulSet {
	actualReplicas := sset.GetReplicas(actual)
	targetReplicas := sset.GetReplicas(toApply)
	if actualReplicas == targetReplicas {
		return toApply
	}

	nodespec.UpdateReplicas(&toApply, common.Int32(actualReplicas))
	if label.IsMasterNodeSet(toApply) {
		for rep := actualReplicas + 1; rep <= targetReplicas; rep++ {
			if !s.canAddMasterNode() {
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
		}
	} else {
		replicasToCreate := targetReplicas - actualReplicas
		replicasToCreate = s.getMaxNodesToAdd(replicasToCreate)
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
				"Limiting nodes creation",
				"target", targetReplicas,
				"actual", actualReplicas,
			)
		}
	}

	return toApply
}
