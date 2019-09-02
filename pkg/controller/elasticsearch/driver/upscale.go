// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type upscaleCtx struct {
	k8sClient           k8s.Client
	es                  v1alpha1.Elasticsearch
	scheme              *runtime.Scheme
	observedState       observer.State
	esState             ESState
	upscaleStateBuilder *upscaleStateBuilder
}

// HandleUpscaleAndSpecChanges reconciles expected NodeSpec resources.
// It does:
// - create any new StatefulSets
// - update existing StatefulSets specification, to be used for future pods rotation
// - upscale StatefulSet for which we expect more replicas
// - limit master node creation to one at a time
// It does not:
// - perform any StatefulSet downscale (left for downscale phase)
// - perform any pod upgrade (left for rolling upgrade phase)
func HandleUpscaleAndSpecChanges(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) error {
	// adjust expected replicas to control nodes creation and deletion
	adjusted, err := adjustResources(ctx, actualStatefulSets, expectedResources)
	if err != nil {
		return err
	}
	// reconcile all resources
	for _, res := range adjusted {
		if err := settings.ReconcileConfig(ctx.k8sClient, ctx.es, res.StatefulSet.Name, res.Config); err != nil {
			return err
		}
		if _, err := common.ReconcileService(ctx.k8sClient, ctx.scheme, &res.HeadlessService, &ctx.es); err != nil {
			return err
		}
		if err := sset.ReconcileStatefulSet(ctx.k8sClient, ctx.scheme, ctx.es, res.StatefulSet); err != nil {
			return err
		}
	}
	return nil
}

func adjustResources(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) (nodespec.ResourcesList, error) {
	adjustedResources := make(nodespec.ResourcesList, 0, len(expectedResources))
	for _, nodeSpecRes := range expectedResources {
		adjustedSset, err := adjustStatefulSetReplicas(ctx, actualStatefulSets, *nodeSpecRes.StatefulSet.DeepCopy())
		if err != nil {
			return nil, err
		}
		nodeSpecRes.StatefulSet = adjustedSset
		adjustedResources = append(adjustedResources, nodeSpecRes)
	}
	// adapt resources configuration to match adjusted replicas
	if err := adjustZenConfig(ctx.es, adjustedResources); err != nil {
		return nil, err
	}
	return adjustedResources, nil
}

func adjustZenConfig(es v1alpha1.Elasticsearch, resources nodespec.ResourcesList) error {
	// patch configs to consider zen1 minimum master nodes
	if err := zen1.SetupMinimumMasterNodesConfig(resources); err != nil {
		return err
	}
	// patch configs to consider zen2 initial master nodes if cluster is not bootstrapped yet
	if !AnnotatedForBootstrap(es) {
		if err := zen2.SetupInitialMasterNodes(resources); err != nil {
			return err
		}
	}
	return nil
}

func adjustStatefulSetReplicas(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expected appsv1.StatefulSet,
) (appsv1.StatefulSet, error) {
	actual, alreadyExists := actualStatefulSets.GetByName(expected.Name)
	if alreadyExists {
		expected = adaptForExistingStatefulSet(actual, expected)
	}
	if alreadyExists && isReplicaIncrease(actual, expected) {
		upscaleState, err := ctx.upscaleStateBuilder.InitOnce(ctx.k8sClient, ctx.es, ctx.esState)
		if err != nil {
			return appsv1.StatefulSet{}, err
		}
		expected = upscaleState.limitMasterNodesCreation(actualStatefulSets, expected)
	}
	return expected, nil
}

// isReplicaIncrease returns true if expected replicas are higher than actual replicas.
func isReplicaIncrease(actual appsv1.StatefulSet, expected appsv1.StatefulSet) bool {
	return sset.GetReplicas(expected) > sset.GetReplicas(actual)
}

// adaptForExistingStatefulSet modifies ssetToApply to account for the existing StatefulSet.
// It avoids triggering downscales (done later), and makes sure new pods are created with the newest revision.
func adaptForExistingStatefulSet(actualSset appsv1.StatefulSet, ssetToApply appsv1.StatefulSet) appsv1.StatefulSet {
	if sset.GetReplicas(ssetToApply) < sset.GetReplicas(actualSset) {
		// This is a downscale.
		// We still want to update the sset spec to the newest one, but don't scale replicas down for now.
		nodespec.UpdateReplicas(&ssetToApply, actualSset.Spec.Replicas)
	}
	// Make sure new pods (with ordinal>partition) get created with the newest revision,
	// by setting the rollingUpdate partition to the actual StatefulSet replicas count.
	// Any ongoing rolling upgrade may temporarily pause here, but will go through again.
	nodespec.UpdatePartition(&ssetToApply, actualSset.Spec.Replicas)
	return ssetToApply
}
