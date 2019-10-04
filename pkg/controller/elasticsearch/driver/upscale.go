// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type upscaleCtx struct {
	k8sClient     k8s.Client
	es            v1beta1.Elasticsearch
	scheme        *runtime.Scheme
	observedState observer.State
	esState       ESState
	expectations  *expectations.Expectations
}

// HandleUpscaleAndSpecChanges reconciles expected NodeSet resources.
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
) (sset.StatefulSetList, error) {
	// adjust expected replicas to control nodes creation and deletion
	adjusted, err := adjustResources(ctx, actualStatefulSets, expectedResources)
	if err != nil {
		return nil, err
	}
	// reconcile all resources
	for _, res := range adjusted {
		if err := settings.ReconcileConfig(ctx.k8sClient, ctx.es, res.StatefulSet.Name, res.Config); err != nil {
			return nil, err
		}
		if _, err := common.ReconcileService(ctx.k8sClient, ctx.scheme, &res.HeadlessService, &ctx.es); err != nil {
			return nil, err
		}
		reconciled, err := sset.ReconcileStatefulSet(ctx.k8sClient, ctx.scheme, ctx.es, res.StatefulSet, ctx.expectations)
		if err != nil {
			return nil, err
		}
		// update actual with the reconciled ones for next steps to work with up-to-date information
		actualStatefulSets = actualStatefulSets.WithStatefulSet(reconciled)
	}
	return actualStatefulSets, nil
}

func adjustResources(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) (nodespec.ResourcesList, error) {
	upscaleState := newUpscaleState(ctx, actualStatefulSets, expectedResources)
	adjustedResources := make(nodespec.ResourcesList, 0, len(expectedResources))
	for _, nodeSpecRes := range expectedResources {
		adjusted, err := adjustStatefulSetReplicas(*upscaleState, actualStatefulSets, *nodeSpecRes.StatefulSet.DeepCopy())
		if err != nil {
			return nil, err
		}
		nodeSpecRes.StatefulSet = adjusted
		adjustedResources = append(adjustedResources, nodeSpecRes)
	}
	// adapt resources configuration to match adjusted replicas
	if err := adjustZenConfig(ctx.k8sClient, ctx.es, adjustedResources); err != nil {
		return nil, err
	}
	return adjustedResources, nil
}

func adjustZenConfig(k8sClient k8s.Client, es v1beta1.Elasticsearch, resources nodespec.ResourcesList) error {
	// patch configs to consider zen1 minimum master nodes
	if err := zen1.SetupMinimumMasterNodesConfig(k8sClient, es, resources); err != nil {
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
	upscaleState upscaleState,
	actualStatefulSets sset.StatefulSetList,
	expected appsv1.StatefulSet,
) (appsv1.StatefulSet, error) {
	actual, alreadyExists := actualStatefulSets.GetByName(expected.Name)
	expectedReplicas := sset.GetReplicas(expected)
	actualReplicas := sset.GetReplicas(actual)

	if actualReplicas < expectedReplicas {
		return upscaleState.limitNodesCreation(actual, expected)
	}

	if alreadyExists && expectedReplicas < actualReplicas {
		// this is a downscale.
		// We still want to update the sset spec to the newest one, but leave scaling down as it's done later.
		nodespec.UpdateReplicas(&expected, actual.Spec.Replicas)
	}

	return expected, nil
}

// isReplicaIncrease returns true if expected replicas are higher than actual replicas.
func isReplicaIncrease(actual appsv1.StatefulSet, expected appsv1.StatefulSet) bool {
	return sset.GetReplicas(expected) > sset.GetReplicas(actual)
}
