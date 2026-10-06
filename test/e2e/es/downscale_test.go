// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

const (
	// pinnedIndex is kept on the leaving nodes so that their remove-type shutdowns stay registered
	pinnedIndex = "pinned-to-leaving-nodes"
	// shutdownsKeptFor is how long the shutdowns of the leaving nodes must not be cancelled. Their Pods become not ready
	// about 15 seconds after the shutdowns are registered, which is when the downscale used to cancel them.
	shutdownsKeptFor = time.Minute
)

// TestDownscaleKeepsShutdownsWhileAnotherPodIsNotReady removes a NodeSet while the Pod of another NodeSet cannot be
// scheduled. The readiness probe makes the leaving nodes not ready as soon as their remove-type shutdown is registered,
// which must not make the downscale use up its budget and cancel the shutdowns it is waiting for.
func TestDownscaleKeepsShutdownsWhileAnotherPodIsNotReady(t *testing.T) {
	// nodes with a registered shutdown are only reported not ready by the readiness port, used after 8.2.0
	if version.MustParse(test.Ctx().ElasticStackVersion).LE(esv1.MinReadinessPortVersion) {
		t.SkipNow()
	}

	ctx := t.Context()
	k := test.NewK8sClientOrFatal()

	initial := elasticsearch.NewBuilder("test-downscale-keep-shutdowns").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data", elasticsearch.DefaultResources)

	// rename the data NodeSet, which removes the data nodes once their data is migrated to the new ones
	renamed := initial.WithNoESTopology().
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithNamedESDataNodes(2, "data-new", elasticsearch.DefaultResources)

	// add a NodeSet whose Pod cannot be scheduled, so that one Pod is not ready during the removal
	pendingNodeSet := esv1.NodeSet{
		Name:        "pending",
		Count:       1,
		Config:      &commonv1.Config{Data: elasticsearch.DataRoleCfg(initial.Elasticsearch.Spec.Version)},
		PodTemplate: elasticsearch.ESPodTemplate(elasticsearch.DefaultResources),
	}
	pendingNodeSet.PodTemplate.Spec.NodeSelector = map[string]string{"cannot": "be-scheduled"}
	renamedWithPending := renamed.DeepCopy().WithNodeSet(pendingNodeSet)

	// eventually let the Pod be scheduled: a NodeSet whose Pods never joined the cluster cannot be removed
	fixedNodeSet := *pendingNodeSet.DeepCopy()
	fixedNodeSet.PodTemplate.Spec.NodeSelector = nil
	fixed := renamed.DeepCopy().WithNodeSet(fixedNodeSet)

	esName := initial.Elasticsearch.Name
	esNamespace := initial.Elasticsearch.Namespace
	pendingPod := sset.PodName(esv1.StatefulSet(esName, "pending"), 0)
	leavingNodes := []string{
		sset.PodName(esv1.StatefulSet(esName, "data"), 0),
		sset.PodName(esv1.StatefulSet(esName, "data"), 1),
	}
	var esClient esclient.Client
	leavingPods := test.NewPodRestartChecker("leaving Elasticsearch", test.ESPodListOptionsByNodeSet(esNamespace, esName, "data")...)
	var reconciledGeneration int64

	test.StepList{}.
		WithSteps(initial.InitTestSteps(k)).
		WithSteps(initial.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initial, k)).
		WithStep(test.Step{
			Name: "Create an index that cannot be migrated to the new data nodes",
			Test: test.Eventually(func() error {
				var err error
				if esClient, err = elasticsearch.NewElasticsearchClient(initial.Elasticsearch, k); err != nil {
					return err
				}
				// the index may already exist if a previous attempt only failed to read the response
				if _, err := elasticsearch.DoRequest(ctx, esClient, http.MethodGet, "/"+pinnedIndex, nil); err == nil {
					return nil
				}
				// one primary on each leaving node, without replicas: Elasticsearch considers a node holding only a
				// replica that cannot move as safe to remove
				settings := fmt.Sprintf(
					`{"settings": {"number_of_shards": 2, "number_of_replicas": 0, "index.routing.allocation.total_shards_per_node": 1, "index.routing.allocation.exclude._name": "%s-*"}}`,
					esv1.StatefulSet(esName, "data-new"),
				)
				_, err = elasticsearch.DoRequest(ctx, esClient, http.MethodPut, "/"+pinnedIndex, []byte(settings))
				return err
			}),
		}).
		WithStep(elasticsearch.CheckClusterHealth(initial, k)).
		WithSteps(leavingPods.RecordUIDs(k)).
		WithSteps(renamedWithPending.UpgradeTestSteps(k)).
		WithStep(test.Step{
			Name: "Leaving nodes should have a remove-type shutdown",
			Test: test.Eventually(func() error {
				return checkRemoveShutdowns(ctx, esClient, leavingNodes)
			}),
		}).
		WithStep(test.Step{
			Name: "Leaving nodes should not be ready",
			Test: test.Eventually(func() error {
				return checkPodsNotReady(k, esNamespace, leavingNodes)
			}),
		}).
		WithStep(test.Step{
			// a spec change that does not affect the Pods, to make sure the operator reconciles while the leaving Pods
			// are not ready rather than the shutdowns being kept because it is not reconciling
			Name: "Change the StatefulSets revision history limit",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(ctx, k8s.ExtractNamespacedName(&initial.Elasticsearch), &es); err != nil {
					return err
				}
				es.Spec.RevisionHistoryLimit = new(int32(5))
				if err := k.Client.Update(ctx, &es); err != nil {
					return err
				}
				reconciledGeneration = es.Generation
				return nil
			}),
		}).
		WithStep(test.Step{
			Name: "The operator should reconcile the change",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(ctx, k8s.ExtractNamespacedName(&initial.Elasticsearch), &es); err != nil {
					return err
				}
				if es.Status.ObservedGeneration < reconciledGeneration {
					return fmt.Errorf("observed generation %d, expected %d", es.Status.ObservedGeneration, reconciledGeneration)
				}
				return nil
			}),
		}).
		WithStep(test.Step{
			// the shutdowns used to be cancelled by the next reconciliation once their Pods were not ready, then
			// requested again, so the state is sampled over several reconciliations
			Name: "Shutdowns of the leaving nodes should not be cancelled",
			Test: func(t *testing.T) {
				t.Helper()
				require.Never(t, func() bool {
					err := checkRemoveShutdowns(ctx, esClient, leavingNodes)
					if err == nil {
						err = checkPodsNotReady(k, esNamespace, leavingNodes)
					}
					if err != nil {
						t.Log(err)
					}
					// errors while querying Elasticsearch or the API server are not considered as a cancellation
					return errors.Is(err, errNoRemoveShutdown) || errors.Is(err, errPodReady)
				}, shutdownsKeptFor, 2*time.Second, "a shutdown of the leaving nodes was cancelled")
			},
		}).
		WithStep(test.Step{
			Name: "Leaving Pods should not have been restarted",
			Test: func(t *testing.T) {
				t.Helper()
				require.NoError(t, leavingPods.CheckNotRestarted(ctx, k))
			},
		}).
		WithStep(test.Step{
			Name: "Allow the index to be migrated to the new data nodes",
			Test: test.Eventually(func() error {
				_, err := elasticsearch.DoRequest(ctx, esClient, http.MethodPut, "/"+pinnedIndex+"/_settings",
					[]byte(`{"index.routing.allocation.exclude._name": null}`))
				return err
			}),
		}).
		WithStep(test.Step{
			Name: "Leaving nodes should be removed while the pending Pod is still not ready",
			// the removal must not wait for the pending Pod: stop retrying as soon as it is ready
			Test: test.RetryOnError(func() error {
				if err := checkPodsNotReady(k, esNamespace, []string{pendingPod}); err != nil {
					return err
				}
				var statefulSet appsv1.StatefulSet
				err := k.Client.Get(ctx, types.NamespacedName{Namespace: esNamespace, Name: esv1.StatefulSet(esName, "data")}, &statefulSet)
				if apierrors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("StatefulSet %s still exists with %d replicas", statefulSet.Name, statefulSet.Status.Replicas)
			}, func(err error) bool { return !errors.Is(err, errPodReady) }, test.Ctx().TestTimeout),
		}).
		WithSteps(fixed.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(fixed, k)).
		WithSteps(fixed.DeletionTestSteps(k)).
		RunSequential(t)
}

var (
	// errNoRemoveShutdown is returned by checkRemoveShutdowns when a node has no remove-type shutdown.
	errNoRemoveShutdown = errors.New("no remove-type shutdown")
	// errPodReady is returned by checkPodsNotReady when a Pod is ready.
	errPodReady = errors.New("pod is ready")
)

// checkRemoveShutdowns returns an error wrapping errNoRemoveShutdown if one of the given nodes has no remove-type
// shutdown, or is not in the cluster anymore.
func checkRemoveShutdowns(ctx context.Context, esClient esclient.Client, nodeNames []string) error {
	nodes, err := esClient.GetNodes(ctx)
	if err != nil {
		return err
	}
	shutdowns, err := esClient.GetShutdown(ctx, nil)
	if err != nil {
		return err
	}
	removing := map[string]bool{}
	for _, s := range shutdowns.Nodes {
		if s.Is(esclient.Remove) {
			removing[s.NodeID] = true
		}
	}
	nameToID := map[string]string{}
	for id, node := range nodes.Nodes {
		nameToID[node.Name] = id
	}
	for _, name := range nodeNames {
		id, inCluster := nameToID[name]
		if !inCluster {
			return fmt.Errorf("node %s is not in the cluster: %w", name, errNoRemoveShutdown)
		}
		if !removing[id] {
			return fmt.Errorf("node %s: %w", name, errNoRemoveShutdown)
		}
	}
	return nil
}

// checkPodsNotReady returns an error if one of the given Pods is ready.
func checkPodsNotReady(k *test.K8sClient, namespace string, podNames []string) error {
	for _, name := range podNames {
		pod, err := k.GetPod(namespace, name)
		if err != nil {
			return err
		}
		if k8s.IsPodReady(pod) {
			return fmt.Errorf("%s: %w", name, errPodReady)
		}
	}
	return nil
}
