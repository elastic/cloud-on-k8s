// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type upscaleStateBuilder struct {
	once         *sync.Once
	upscaleState *upscaleState
}

func (o *upscaleStateBuilder) InitOnce(c k8s.Client, es v1alpha1.Elasticsearch, esState ESState) (*upscaleState, error) {
	if o.once == nil {
		o.once = &sync.Once{}
	}
	var err error
	o.once.Do(func() {
		o.upscaleState, err = newUpscaleState(c, es, esState)
	})
	return o.upscaleState, err
}

type upscaleState struct {
	isBootstrapped      bool
	allowMasterCreation bool
}

func newUpscaleState(c k8s.Client, es v1alpha1.Elasticsearch, esState ESState) (*upscaleState, error) {
	state := &upscaleState{
		isBootstrapped:      AnnotatedForBootstrap(es),
		allowMasterCreation: true,
	}
	if !state.isBootstrapped {
		return state, nil
	}
	// is there a master node creation in progress already?
	masters, err := sset.GetActualMastersForCluster(c, es)
	if err != nil {
		return nil, err
	}
	for _, masterNodePod := range masters {
		isJoining, err := isMasterNodeJoining(masterNodePod, esState)
		if err != nil {
			return nil, err
		}
		if isJoining {
			state.recordMasterNodeCreation()
		}
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
}

func (s *upscaleState) canAddMasterNode() bool {
	return s.allowMasterCreation
}

func (s *upscaleState) limitMasterNodesCreation(
	actualStatefulSets sset.StatefulSetList,
	ssetToApply appsv1.StatefulSet,
) appsv1.StatefulSet {
	if !label.IsMasterNodeSet(ssetToApply) {
		return ssetToApply
	}

	targetReplicas := sset.GetReplicas(ssetToApply)
	actual, alreadyExists := actualStatefulSets.GetByName(ssetToApply.Name)
	actualReplicas := int32(0)
	if alreadyExists {
		actualReplicas = sset.GetReplicas(actual)
	}

	nodespec.UpdateReplicas(&ssetToApply, common.Int32(actualReplicas))
	for rep := actualReplicas + 1; rep <= targetReplicas; rep++ {
		if !s.canAddMasterNode() {
			ssetLogger(ssetToApply).Info(
				"Limiting master nodes creation to one at a time",
				"target", targetReplicas,
				"current", sset.GetReplicas(ssetToApply),
			)
			break
		}
		// allow one more master node to be created
		nodespec.UpdateReplicas(&ssetToApply, common.Int32(rep))
		s.recordMasterNodeCreation()
	}

	return ssetToApply
}
