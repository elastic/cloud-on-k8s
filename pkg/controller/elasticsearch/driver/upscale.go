// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

type upscaleCtx struct {
	parentCtx            context.Context
	k8sClient            k8s.Client
	es                   esv1.Elasticsearch
	esState              ESState
	expectations         *expectations.Expectations
	validateStorageClass bool
	upscaleReporter      *reconcile.UpscaleReporter
}

type UpscaleResults struct {
	ActualStatefulSets sset.StatefulSetList
	Requeue            bool
}

// HandleUpscaleAndSpecChanges reconciles expected NodeSet resources.
// It does:
// - create any new StatefulSets
// - update existing StatefulSets specification, to be used for future pods rotation
// - upscale StatefulSet for which we expect more replicas
// - limit master node creation to one at a time
// - resize (inline) existing PVCs to match new StatefulSet storage reqs and schedule the StatefulSet recreation
// It does not:
// - perform any StatefulSet downscale (left for downscale phase)
// - perform any pod upgrade (left for rolling upgrade phase)
func HandleUpscaleAndSpecChanges(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) (UpscaleResults, error) {
	results := UpscaleResults{}

	// Set the list of expected new nodes in the status early. This is to ensure that the list of expected nodes to be
	// created is surfaced in the status even if an error occurs later in the upscale process.
	ctx.upscaleReporter.RecordNewNodes(podsToCreate(actualStatefulSets, expectedResources.StatefulSets()))

	// adjust expected replicas to control nodes creation and deletion
	adjusted, err := adjustResources(ctx, actualStatefulSets, expectedResources)
	if err != nil {
		return results, fmt.Errorf("adjust resources: %w", err)
	}
	// reconcile all resources
	for _, res := range adjusted {
		if err := settings.ReconcileConfig(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet.Name, res.Config); err != nil {
			return results, fmt.Errorf("reconcile config: %w", err)
		}
		if _, err := common.ReconcileService(ctx.parentCtx, ctx.k8sClient, &res.HeadlessService, &ctx.es); err != nil {
			return results, fmt.Errorf("reconcile service: %w", err)
		}
		if actualSset, exists := actualStatefulSets.GetByName(res.StatefulSet.Name); exists {
			recreateSset, err := handleVolumeExpansion(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet, actualSset, ctx.validateStorageClass)
			if err != nil {
				return results, fmt.Errorf("handle volume expansion: %w", err)
			}
			if recreateSset {
				// The StatefulSet is scheduled for recreation: let's requeue before attempting any further spec change.
				results.Requeue = true
				continue
			}
		}
		reconciled, err := sset.ReconcileStatefulSet(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet, ctx.expectations)
		if err != nil {
			return results, fmt.Errorf("reconcile StatefulSet: %w", err)
		}
		// update actual with the reconciled ones for next steps to work with up-to-date information
		actualStatefulSets = actualStatefulSets.WithStatefulSet(reconciled)
	}
	results.ActualStatefulSets = actualStatefulSets
	return results, nil
}

func podsToCreate(
	actualStatefulSets, expectedStatefulSets sset.StatefulSetList,
) []string {
	var pods []string
	for _, expectedStatefulSet := range expectedStatefulSets {
		actualSset, _ := actualStatefulSets.GetByName(expectedStatefulSet.Name)
		expectedReplicas := sset.GetReplicas(expectedStatefulSet)
		for expectedReplicas > sset.GetReplicas(actualSset) {
			pods = append(pods, sset.PodName(expectedStatefulSet.Name, expectedReplicas-1))
			expectedReplicas--
		}
	}
	return pods
}

func adjustResources(
	ctx upscaleCtx,
	actualStatefulSets sset.StatefulSetList,
	expectedResources nodespec.ResourcesList,
) (nodespec.ResourcesList, error) {
	upscaleState := newUpscaleState(ctx, actualStatefulSets, expectedResources)
	adjustedResources := make(nodespec.ResourcesList, 0, len(expectedResources))
	for _, nodeSpecRes := range expectedResources {
		adjusted, err := adjustStatefulSetReplicas(upscaleState, actualStatefulSets, *nodeSpecRes.StatefulSet.DeepCopy())
		if err != nil {
			return nil, err
		}
		nodeSpecRes.StatefulSet = adjusted
		adjustedResources = append(adjustedResources, nodeSpecRes)
	}
	// adapt resources configuration to match adjusted replicas
	if err := adjustZenConfig(ctx.parentCtx, ctx.k8sClient, ctx.es, adjustedResources); err != nil {
		return nil, fmt.Errorf("adjust discovery config: %w", err)
	}
	return adjustedResources, nil
}

func adjustZenConfig(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, resources nodespec.ResourcesList) error {
	// patch configs to consider zen1 minimum master nodes
	if err := zen1.SetupMinimumMasterNodesConfig(ctx, k8sClient, es, resources); err != nil {
		return err
	}
	// patch configs to consider zen2 initial master nodes
	return zen2.SetupInitialMasterNodes(ctx, es, k8sClient, resources)
}

// adjustStatefulSetReplicas updates the replicas count in expected according to
// what is allowed by the upscaleState, that may be mutated as a result.
func adjustStatefulSetReplicas(
	upscaleState *upscaleState,
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
