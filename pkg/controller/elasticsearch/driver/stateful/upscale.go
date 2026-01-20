// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

type upscaleCtx struct {
	parentCtx            context.Context
	k8sClient            k8s.Client
	es                   esv1.Elasticsearch
	expectations         *expectations.Expectations
	validateStorageClass bool
	upscaleReporter      *reconcile.UpscaleReporter
	meta                 metadata.Metadata
}

type UpscaleResults struct {
	ActualStatefulSets es_sset.StatefulSetList
	Requeue            bool
}

// HandleUpscaleAndSpecChanges reconciles expected NodeSet resources.
// It does:
// - create any new StatefulSets
// - update existing StatefulSets specification, to be used for future pods rotation
// - upscale StatefulSet for which we expect more replicas
// - resize (inline) existing PVCs to match new StatefulSet storage reqs and schedule the StatefulSet recreation
// It does not:
// - perform any StatefulSet downscale (left for downscale phase)
// - perform any pod upgrade (left for rolling upgrade phase)
func HandleUpscaleAndSpecChanges(
	ctx upscaleCtx,
	actualStatefulSets es_sset.StatefulSetList,
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

	// Check if this is a version upgrade
	isVersionUpgrade, err := isVersionUpgrade(ctx.es)
	if err != nil {
		return results, fmt.Errorf("while checking for version upgrade: %w", err)
	}

	// If this is not a version upgrade, process all resources normally and return
	if !isVersionUpgrade {
		results, err = reconcileResources(ctx, actualStatefulSets, adjusted)
		if err != nil {
			return results, fmt.Errorf("while reconciling resources: %w", err)
		}
		return results, nil
	}

	// Version upgrade: separate master and non-master StatefulSets
	var masterResources, nonMasterResources []nodespec.Resources
	for _, res := range adjusted {
		if label.IsMasterNodeSet(res.StatefulSet) {
			masterResources = append(masterResources, res)
		} else {
			nonMasterResources = append(nonMasterResources, res)
		}
	}

	// Only attempt this upscale of master StatefulSets if there are any non-master StatefulSets to reconcile,
	// otherwise you will immediately upscale the masters, and then delete the new node you added to upgrade.
	if len(nonMasterResources) > 0 {
		// The only adjustment we want to make to master statefulSets before ensuring that all non-master
		// statefulSets have been reconciled is to potentially scale up the replicas.
		results, err = maybeUpscaleMasterResources(ctx, actualStatefulSets, masterResources)
		if err != nil {
			return results, fmt.Errorf("while scaling up master resources: %w", err)
		}
		actualStatefulSets = results.ActualStatefulSets
	}

	// First, reconcile all non-master resources
	results, err = reconcileResources(ctx, actualStatefulSets, nonMasterResources)
	if err != nil {
		return results, fmt.Errorf("while reconciling non-master resources: %w", err)
	}
	results.ActualStatefulSets = actualStatefulSets

	if results.Requeue {
		return results, nil
	}

	targetVersion, err := version.Parse(ctx.es.Spec.Version)
	if err != nil {
		return results, fmt.Errorf("while parsing Elasticsearch upgrade target version: %w", err)
	}

	// Check if all non-master StatefulSets have completed their upgrades before proceeding with master StatefulSets
	pendingNonMasterSTS, err := findPendingNonMasterStatefulSetUpgrades(
		ctx.k8sClient,
		actualStatefulSets,
		expectedResources.StatefulSets(),
		targetVersion,
		ctx.expectations,
	)
	if err != nil {
		return results, fmt.Errorf("while checking non-master upgrade status: %w", err)
	}

	ctx.upscaleReporter.RecordPendingNonMasterSTSUpgrades(pendingNonMasterSTS)

	if len(pendingNonMasterSTS) > 0 {
		// Non-master StatefulSets are still upgrading, skipping master StatefulSets temporarily.
		// This will cause a requeue in the caller, and master StatefulSets will attempt to be processed in the next reconciliation
		return results, nil
	}

	// All non-master StatefulSets are upgraded, now process master StatefulSets
	results, err = reconcileResources(ctx, actualStatefulSets, masterResources)
	if err != nil {
		return results, fmt.Errorf("while reconciling master resources: %w", err)
	}

	results.ActualStatefulSets = actualStatefulSets
	return results, nil
}

func maybeUpscaleMasterResources(ctx upscaleCtx, actualStatefulSets es_sset.StatefulSetList, masterResources []nodespec.Resources) (UpscaleResults, error) {
	results := UpscaleResults{
		ActualStatefulSets: actualStatefulSets,
	}
	// Upscale master StatefulSets using the adjusted resources and read the current StatefulSet
	// from k8s to get the latest state.
	for _, res := range masterResources {
		stsName := res.StatefulSet.Name

		// Read the current StatefulSet from k8s to get the latest state
		var actualSset appsv1.StatefulSet
		if err := ctx.k8sClient.Get(ctx.parentCtx, k8s.ExtractNamespacedName(&res.StatefulSet), &actualSset); err != nil {
			// If the StatefulSet is not found, it means that it has not been created yet, so we can skip it.
			// This should only happen when a user is upscaling the master nodes with a new NodeSet/StatefulSet.
			// We are only interested in scaling up the existing master StatefulSets in this case.
			if apierrors.IsNotFound(err) {
				continue
			}
			return results, fmt.Errorf("while getting master StatefulSet %s: %w", stsName, err)
		}

		actualReplicas := sset.GetReplicas(actualSset)
		targetReplicas := sset.GetReplicas(res.StatefulSet)

		if actualReplicas < targetReplicas {
			nodespec.UpdateReplicas(&actualSset, ptr.To(targetReplicas))
			reconciled, err := es_sset.ReconcileStatefulSet(ctx.parentCtx, ctx.k8sClient, ctx.es, actualSset, ctx.expectations)
			if err != nil {
				return results, fmt.Errorf("while reconciling master StatefulSet %s: %w", stsName, err)
			}
			results.ActualStatefulSets = results.ActualStatefulSets.WithStatefulSet(reconciled)
		}
	}
	return results, nil
}

func podsToCreate(
	actualStatefulSets, expectedStatefulSets es_sset.StatefulSetList,
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
	actualStatefulSets es_sset.StatefulSetList,
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
	// patch configs to consider zen2 initial master nodes
	return zen2.SetupInitialMasterNodes(ctx, es, k8sClient, resources)
}

// adjustStatefulSetReplicas updates the replicas count in expected according to
// what is allowed by the upscaleState, that may be mutated as a result.
func adjustStatefulSetReplicas(
	upscaleState *upscaleState,
	actualStatefulSets es_sset.StatefulSetList,
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

// reconcileResources handles the common StatefulSet reconciliation logic
// It returns:
// - the updated StatefulSets
// - whether a requeue is needed
// - any errors that occurred
func reconcileResources(
	ctx upscaleCtx,
	actualStatefulSets es_sset.StatefulSetList,
	resources []nodespec.Resources,
) (UpscaleResults, error) {
	results := UpscaleResults{
		ActualStatefulSets: actualStatefulSets,
	}
	ulog.FromContext(ctx.parentCtx).Info("Reconciling resources", "resource_size", len(resources))
	for _, res := range resources {
		res := res
		if err := settings.ReconcileConfig(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet.Name, res.Config, ctx.meta); err != nil {
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
				ulog.FromContext(ctx.parentCtx).Info("StatefulSet is scheduled for recreation, requeuing", "name", res.StatefulSet.Name)
				// The StatefulSet is scheduled for recreation: let's requeue before attempting any further spec change.
				results.Requeue = true
				continue
			}
		} else {
			ulog.FromContext(ctx.parentCtx).Info("StatefulSet does not exist", "name", res.StatefulSet.Name)
		}
		ulog.FromContext(ctx.parentCtx).Info("Reconciling StatefulSet", "name", res.StatefulSet.Name)
		reconciled, err := es_sset.ReconcileStatefulSet(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet, ctx.expectations)
		if err != nil {
			return results, fmt.Errorf("reconcile StatefulSet: %w", err)
		}
		// update actual with the reconciled ones for next steps to work with up-to-date information
		results.ActualStatefulSets = results.ActualStatefulSets.WithStatefulSet(reconciled)
	}
	ulog.FromContext(ctx.parentCtx).Info("Resources reconciled", "actualStatefulSets_size", len(results.ActualStatefulSets), "requeue", results.Requeue)
	return results, nil
}

// findPendingNonMasterStatefulSetUpgrades finds all non-master StatefulSets that have not completed their upgrades
func findPendingNonMasterStatefulSetUpgrades(
	client k8s.Client,
	actualStatefulSets es_sset.StatefulSetList,
	expectedStatefulSets es_sset.StatefulSetList,
	targetVersion version.Version,
	expectations *expectations.Expectations,
) ([]appsv1.StatefulSet, error) {
	pendingStatefulSets, err := expectations.ExpectedStatefulSetUpdates.PendingGenerations()
	if err != nil {
		return nil, err
	}

	pendingNonMasterSTS := make([]appsv1.StatefulSet, 0)
	for _, actualStatefulSet := range actualStatefulSets {
		expectedSset, _ := expectedStatefulSets.GetByName(actualStatefulSet.Name)

		// Skip master StatefulSets. We check both here because the master role may have been added
		// to a non-master StatefulSet during the upgrade spec change.
		if label.IsMasterNodeSet(actualStatefulSet) || label.IsMasterNodeSet(expectedSset) {
			continue
		}

		// If the expectations show this as a pending StatefulSet, add it to the list.
		if slices.Contains(pendingStatefulSets, actualStatefulSet.Name) {
			pendingNonMasterSTS = append(pendingNonMasterSTS, actualStatefulSet)
			continue
		}

		// If the StatefulSet is not at the target version, it is not upgraded
		// so don't even bother looking at the state/status of the StatefulSet.
		actualVersion, err := es_sset.GetESVersion(actualStatefulSet)
		if err != nil {
			return pendingNonMasterSTS, err
		}
		if actualVersion.LT(targetVersion) {
			pendingNonMasterSTS = append(pendingNonMasterSTS, actualStatefulSet)
			continue
		}

		if actualStatefulSet.Status.ObservedGeneration < actualStatefulSet.Generation {
			// The StatefulSet controller has not yet observed the latest generation.
			pendingNonMasterSTS = append(pendingNonMasterSTS, actualStatefulSet)
			continue
		}

		// Check if this StatefulSet has pending updates
		if actualStatefulSet.Status.UpdatedReplicas != actualStatefulSet.Status.Replicas {
			pendingNonMasterSTS = append(pendingNonMasterSTS, actualStatefulSet)
			continue
		}

		// Check if there are any pods that need to be upgraded
		pods, err := es_sset.GetActualPodsForStatefulSet(client, k8s.ExtractNamespacedName(&actualStatefulSet))
		if err != nil {
			return pendingNonMasterSTS, err
		}

		for _, pod := range pods {
			// Check if pod revision matches StatefulSet update revision
			if actualStatefulSet.Status.UpdateRevision != "" && sset.PodRevision(pod) != actualStatefulSet.Status.UpdateRevision {
				// This pod still needs to be upgraded
				pendingNonMasterSTS = append(pendingNonMasterSTS, actualStatefulSet)
				break
			}
		}
	}

	return pendingNonMasterSTS, nil
}
