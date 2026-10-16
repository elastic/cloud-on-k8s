// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// Test_Driver_maybeForceUpgradeWithShutdowns checks that the forced upgrade does not depend on retrieving the nodes,
// which the shutdown interface requires: if they cannot be retrieved, the draining Pods are the ones reported in the
// downscale status, and the error stops the reconciliation after the forced upgrade.
func Test_Driver_maybeForceUpgradeWithShutdowns(t *testing.T) {
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0}
	statefulSet := func(replicas int32) appsv1.StatefulSet {
		status := outdated
		status.Replicas = replicas
		return sset.TestSset{Name: "data", Namespace: TestEsNamespace, ClusterName: TestEsName, Replicas: replicas, Data: true, Status: status}.Build()
	}
	nodesErr := errors.New("cannot retrieve the nodes")
	tests := []struct {
		name     string
		actual   int32
		expected int32
		// readyPods are not bootlooping, the other Pods are
		readyPods []string
		// reportedShutdowns are the nodes with a shutdown in the downscale status, the other leaving nodes are reported as
		// not started
		reportedShutdowns []string
		esReachable       bool
		getNodesErr       error
		wantShutdowns     bool
		wantStop          bool
		wantErr           error
		wantRemainingPods []string
	}{
		{
			name:              "the forced upgrade is attempted if the nodes cannot be retrieved",
			actual:            2,
			expected:          2,
			esReachable:       true,
			getNodesErr:       nodesErr,
			wantStop:          true,
			wantRemainingPods: nil,
		},
		{
			name:        "draining Pods are not deleted if the nodes cannot be retrieved, based on the reported shutdowns",
			actual:      3,
			expected:    1,
			esReachable: true,
			getNodesErr: nodesErr,
			// data-1 has no shutdown to protect
			reportedShutdowns: []string{"data-2"},
			wantStop:          true,
			wantRemainingPods: []string{"data-2"},
		},
		{
			name:              "the error is returned if no forced upgrade is attempted",
			actual:            2,
			expected:          1,
			readyPods:         []string{"data-0", "data-1"},
			esReachable:       true,
			getNodesErr:       nodesErr,
			wantStop:          true,
			wantErr:           nodesErr,
			wantRemainingPods: []string{"data-0", "data-1"},
		},
		{
			name:              "the shutdown interface is returned if no forced upgrade is attempted",
			actual:            2,
			expected:          1,
			readyPods:         []string{"data-0", "data-1"},
			esReachable:       true,
			wantShutdowns:     true,
			wantRemainingPods: []string{"data-0", "data-1"},
		},
		{
			name:              "there is no shutdown interface if Elasticsearch is not reachable",
			actual:            2,
			expected:          1,
			readyPods:         []string{"data-0", "data-1"},
			wantRemainingPods: []string{"data-0", "data-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			actual := es_sset.StatefulSetList{statefulSet(tt.actual)}
			desiredLeaving := desiredLeavingNodeNames(ctx, es_sset.StatefulSetList{statefulSet(tt.expected)}, actual)
			es := esv1.Elasticsearch{Name: TestEsName, Namespace: TestEsNamespace}
			for _, node := range desiredLeaving.AsSortedSlice() {
				status := string(esclient.ShutdownNotStarted)
				if slices.Contains(tt.reportedShutdowns, node) {
					status = string(esclient.ShutdownInProgress)
				}
				es.Status.DownscaleOperation.Nodes = append(es.Status.DownscaleOperation.Nodes, esv1.DownscaledNode{Name: node, ShutdownStatus: status})
			}
			objs := []crclient.Object{&es}
			for i := range tt.actual {
				name := sset.PodName("data", i)
				pod := sset.TestPod{
					Name: name, Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "data",
					Revision: "rev-a", Data: true, Ready: slices.Contains(tt.readyPods, name), RestartCount: 1,
				}.Build()
				objs = append(objs, &pod)
			}
			k8sClient := k8s.NewFakeClient(objs...)
			reconcileState := reconcile.MustNewState(es)
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcileState,
			}
			esClient := &fakeESClient{
				version:     version.MustParse("8.15.0"),
				nodes:       esclient.Nodes{Nodes: map[string]esclient.Node{}},
				GetNodesErr: tt.getNodesErr,
			}

			leaving, stop, err := d.maybeForceUpgradeWithShutdowns(ctx, tt.esReachable, esClient, NewMemoizingESState(ctx, esClient), reconcileState, actual, desiredLeaving)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantStop, stop)
			require.Equal(t, tt.wantShutdowns, leaving != nil && leaving.nodeShutdown != nil)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(ctx, &pods))
			require.ElementsMatch(t, tt.wantRemainingPods, names(pods.Items))
		})
	}
}
