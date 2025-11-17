// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

type upscaleCtx struct {
	parentCtx            context.Context
	k8sClient            k8s.Client
	es                   esv1.Elasticsearch
	esState              ESState
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
// - limit master node creation to one at a time
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
		actualStatefulSets, requeue, err := reconcileResources(ctx, actualStatefulSets, adjusted)
		if err != nil {
			return results, fmt.Errorf("while reconciling resources: %w", err)
		}
		results.Requeue = requeue
		results.ActualStatefulSets = actualStatefulSets
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

	// The only adjustment we want to make to master statefulSets before ensuring that all non-master
	// statefulSets have been reconciled is to scale up the replicas to the expected number.
	if err = maybeUpscaleMasterResources(ctx, actualStatefulSets, masterResources); err != nil {
		return results, fmt.Errorf("while scaling up master resources: %w", err)
	}

	// First, reconcile all non-master resources
	actualStatefulSets, requeue, err := reconcileResources(ctx, actualStatefulSets, nonMasterResources)
	if err != nil {
		return results, fmt.Errorf("while reconciling non-master resources: %w", err)
	}
	results.ActualStatefulSets = actualStatefulSets

	if requeue {
		results.Requeue = true
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
		expectations.NewExpectations(ctx.k8sClient),
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
	actualStatefulSets, results.Requeue, err = reconcileResources(ctx, actualStatefulSets, masterResources)
	if err != nil {
		return results, fmt.Errorf("while reconciling master resources: %w", err)
	}

	results.ActualStatefulSets = actualStatefulSets
	return results, nil
}

func maybeUpscaleMasterResources(ctx upscaleCtx, actualStatefulSets es_sset.StatefulSetList, masterResources []nodespec.Resources) error {
	for _, sts := range masterResources {
		fmt.Println("Attempting upscale of master StatefulSet", sts.StatefulSet.Name)
		actualSset, found := actualStatefulSets.GetByName(sts.StatefulSet.Name)
		if !found {
			fmt.Println("Master StatefulSet not found in actualStatefulSets", sts.StatefulSet.Name)
			continue
		}
		actualReplicas := sset.GetReplicas(actualSset)
		targetReplicas := sset.GetReplicas(sts.StatefulSet)
		fmt.Println("Actual replicas", actualReplicas, "Target replicas", targetReplicas)
		if actualReplicas < targetReplicas {
			actualSset.Spec.Replicas = ptr.To[int32](targetReplicas)
			fmt.Println("Updating master StatefulSet replicas", actualSset.Spec.Replicas, "to", targetReplicas)
			if err := ctx.k8sClient.Update(ctx.parentCtx, &actualSset); err != nil {
				return fmt.Errorf("while upscaling master sts replicas: %w", err)
			}
		}
	}
	return nil
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
) (es_sset.StatefulSetList, bool, error) {
	requeue := false
	ulog.FromContext(ctx.parentCtx).Info("Reconciling resources", "resource_size", len(resources))
	for _, res := range resources {
		res := res
		if err := settings.ReconcileConfig(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet.Name, res.Config, ctx.meta); err != nil {
			return actualStatefulSets, false, fmt.Errorf("reconcile config: %w", err)
		}
		if _, err := common.ReconcileService(ctx.parentCtx, ctx.k8sClient, &res.HeadlessService, &ctx.es); err != nil {
			return actualStatefulSets, false, fmt.Errorf("reconcile service: %w", err)
		}
		if actualSset, exists := actualStatefulSets.GetByName(res.StatefulSet.Name); exists {
			recreateSset, err := handleVolumeExpansion(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet, actualSset, ctx.validateStorageClass)
			if err != nil {
				return actualStatefulSets, false, fmt.Errorf("handle volume expansion: %w", err)
			}
			if recreateSset {
				ulog.FromContext(ctx.parentCtx).Info("StatefulSet is scheduled for recreation, requeuing", "name", res.StatefulSet.Name)
				// The StatefulSet is scheduled for recreation: let's requeue before attempting any further spec change.
				requeue = true
				continue
			}
		} else if !exists {
			ulog.FromContext(ctx.parentCtx).Info("StatefulSet does not exist", "name", res.StatefulSet.Name)
		}
		ulog.FromContext(ctx.parentCtx).Info("Reconciling StatefulSet", "name", res.StatefulSet.Name)
		reconciled, err := es_sset.ReconcileStatefulSet(ctx.parentCtx, ctx.k8sClient, ctx.es, res.StatefulSet, ctx.expectations)
		if err != nil {
			return actualStatefulSets, false, fmt.Errorf("reconcile StatefulSet: %w", err)
		}
		// update actual with the reconciled ones for next steps to work with up-to-date information
		actualStatefulSets = actualStatefulSets.WithStatefulSet(reconciled)
	}
	ulog.FromContext(ctx.parentCtx).Info("Resources reconciled", "actualStatefulSets_size", len(actualStatefulSets), "requeue", requeue)
	return actualStatefulSets, requeue, nil
}

// findPendingNonMasterStatefulSetUpgrades finds all non-master StatefulSets that have not completed their upgrades
func findPendingNonMasterStatefulSetUpgrades(
	client k8s.Client,
	actualStatefulSets es_sset.StatefulSetList,
	expectedStatefulSets es_sset.StatefulSetList,
	targetVersion version.Version,
	expectations *expectations.Expectations,
) ([]appsv1.StatefulSet, error) {
	pendingStatefulSet, err := expectations.ExpectedStatefulSetUpdates.PendingGenerations()
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
		if slices.Contains(pendingStatefulSet, actualStatefulSet.Name) {
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
				continue
			}
		}
	}

	return pendingNonMasterSTS, nil
}
