// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

type upscaleState struct {
	// indicates how many creates, out of createsAllowed, were already recorded
	recordedCreates int32
	// indicates how many creates are allowed when taking into account maxSurge setting,
	// nil indicates that any number of pods can be created, negative value is not expected.
	createsAllowed  *int32
	ctx             upscaleCtx
	upscaleReporter *reconcile.UpscaleReporter
}

func newUpscaleState(
	ctx upscaleCtx,
	actualStatefulSets es_sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) *upscaleState {
	return &upscaleState{
		ctx: ctx,
		createsAllowed: calculateCreatesAllowed(
			ctx.es.Spec.UpdateStrategy.ChangeBudget.GetMaxSurgeOrDefault(),
			actualStatefulSets.ExpectedNodeCount(),
			expectedResources.ExpectedNodeCount()),
		upscaleReporter: ctx.upscaleReporter,
	}
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
	actual appsv1.StatefulSet,
	toApply appsv1.StatefulSet,
) (appsv1.StatefulSet, error) {
	actualReplicas := sset.GetReplicas(actual)
	targetReplicas := sset.GetReplicas(toApply)

	nodespec.UpdateReplicas(&toApply, ptr.To[int32](actualReplicas))
	replicasToCreate := targetReplicas - actualReplicas
	replicasToCreate = s.getMaxNodesToCreate(replicasToCreate)

	if replicasToCreate > 0 {
		nodespec.UpdateReplicas(&toApply, ptr.To[int32](actualReplicas+replicasToCreate))
		s.recordNodesCreation(replicasToCreate)
		s.loggerFor(toApply).Info(
			"Creating nodes",
			"actualReplicas", actualReplicas,
			"replicasToCreate", replicasToCreate,
		)
		s.upscaleReporter.UpdateNodesStatuses(
			esv1.NewNodeExpected,
			toApply.Name,
			fmt.Sprintf("Upscaling StatefulSet %s from %d to %d replicas", toApply.Name, actualReplicas, actualReplicas+replicasToCreate),
			actualReplicas+1,
			actualReplicas+replicasToCreate,
		)
	}
	if replicasToCreate+actualReplicas < targetReplicas {
		msg := "Limiting nodes creation to respect maxSurge setting"
		s.loggerFor(toApply).Info(
			msg,
			"target", targetReplicas,
			"actual", actualReplicas,
		)
		s.upscaleReporter.UpdateNodesStatuses(esv1.NewNodePending, toApply.Name, msg, actualReplicas+replicasToCreate+1, targetReplicas)
	}

	return toApply, nil
}

func (s *upscaleState) loggerFor(sset appsv1.StatefulSet) logr.Logger {
	if s.ctx.parentCtx != nil {
		return ssetLogger(s.ctx.parentCtx, sset)
	}
	// for testing with incomplete state
	return crlog.Log
}
