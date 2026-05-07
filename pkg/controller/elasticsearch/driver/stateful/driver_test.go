// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

func TestDriver_hasPendingSpecChanges(t *testing.T) {
	const esName = "test-cluster"
	const namespace = "test-ns"
	const nodeSetName = "default"
	const nodeSetName2 = "data"

	scriptsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.ScriptsConfigMap(esName),
			Namespace: namespace,
		},
	}

	// --- Single-NodeSet (Count=3) setup ---
	elasticsearch := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esName,
			Namespace: namespace,
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.17.0",
			NodeSets: []esv1.NodeSet{
				{Name: nodeSetName, Count: 3},
			},
		},
	}
	// BuildPodTemplateSpec fetches the scripts ConfigMap; it must exist for BuildExpectedResources to succeed.
	k8sClient := k8s.NewFakeClient(scriptsConfigMap)
	resolvedConfig, err := ResolveConfig(context.Background(), k8sClient, elasticsearch, corev1.IPv4Protocol, false)
	require.NoError(t, err)
	state := &shared.ReconcileState{
		Meta: metadata.Metadata{},
	}
	// Build the expected StatefulSets to use as actualSets in the "no diff" cases.
	// existingStatefulSets=nil means no existing ssets yet (new cluster).
	expectedResources, err := nodespec.BuildExpectedResources(
		context.Background(), k8sClient, elasticsearch, nil,
		nil, false, state.Meta, resolvedConfig,
	)
	require.NoError(t, err)
	matchingActualSets := expectedResources.StatefulSets()

	// --- Two-NodeSet setup ---
	elasticsearch2 := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esName,
			Namespace: namespace,
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.17.0",
			NodeSets: []esv1.NodeSet{
				{Name: nodeSetName, Count: 3},
				{Name: nodeSetName2, Count: 2},
			},
		},
	}
	resolvedConfig2, err := ResolveConfig(context.Background(), k8sClient, elasticsearch2, corev1.IPv4Protocol, false)
	require.NoError(t, err)
	expectedResources2, err := nodespec.BuildExpectedResources(
		context.Background(), k8sClient, elasticsearch2, nil,
		nil, false, state.Meta, resolvedConfig2,
	)
	require.NoError(t, err)
	matchingActualSets2 := expectedResources2.StatefulSets()

	// Build a two-NodeSet actualSets where the second sset has a modified replica count.
	diffActualSets2 := make(es_sset.StatefulSetList, len(matchingActualSets2))
	for i, s := range matchingActualSets2 {
		diffActualSets2[i] = *s.DeepCopy()
	}
	diffActualSets2[1].Spec.Replicas = ptr.To[int32](99)
	diffActualSets2[1].Labels = hash.SetTemplateHashLabel(diffActualSets2[1].Labels, diffActualSets2[1].Spec)

	tests := []struct {
		name                      string
		elasticsearch             esv1.Elasticsearch
		k8sClient                 k8s.Client
		setDefaultSecurityContext bool
		actualSets                es_sset.StatefulSetList
		resolvedConfig            nodespec.ResolvedConfig
		want                      bool
		wantErrMsg                string
	}{
		{
			name:           "actual sets match expected: no pending changes",
			elasticsearch:  elasticsearch,
			k8sClient:      k8sClient,
			actualSets:     matchingActualSets,
			resolvedConfig: resolvedConfig,
			want:           false,
		},
		{
			name:           "actual sets differ from expected (length mismatch): pending changes detected",
			elasticsearch:  elasticsearch,
			k8sClient:      k8sClient,
			actualSets:     es_sset.StatefulSetList{},
			resolvedConfig: resolvedConfig,
			want:           true,
		},
		{
			name: "same-name sset with different replica count: pending changes detected",
			elasticsearch: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Name: esName, Namespace: namespace},
				Spec: esv1.ElasticsearchSpec{
					Version:  "8.17.0",
					NodeSets: []esv1.NodeSet{{Name: nodeSetName, Count: 5}},
				},
			},
			k8sClient:      k8sClient,
			actualSets:     matchingActualSets, // built with Count=3
			resolvedConfig: resolvedConfig,
			want:           true,
		},
		{
			// BuildPodTemplateSpec adds FSGroup+SeccompProfile for ES 8+ when enabled.
			// actualSets built with false; driver re-computes with true → spec differs.
			name:                      "SetDefaultSecurityContext enabled when actualSets lacks security context: pending changes detected",
			elasticsearch:             elasticsearch,
			k8sClient:                 k8sClient,
			setDefaultSecurityContext: true,
			actualSets:                matchingActualSets,
			resolvedConfig:            resolvedConfig,
			want:                      true,
		},
		{
			name:           "multiple NodeSets all match: no pending changes",
			elasticsearch:  elasticsearch2,
			k8sClient:      k8sClient,
			actualSets:     matchingActualSets2,
			resolvedConfig: resolvedConfig2,
			want:           false,
		},
		{
			name:           "multiple NodeSets one differs: pending changes detected",
			elasticsearch:  elasticsearch2,
			k8sClient:      k8sClient,
			actualSets:     diffActualSets2,
			resolvedConfig: resolvedConfig2,
			want:           true,
		},
		{
			// GetActualPodsRestartTriggerAnnotationForCluster reads the annotation from live pods
			// and embeds it in the expected pod template. If actualSets was built without the pod
			// the annotation is absent, creating a diff.
			name:          "pod restart annotation present but absent from actualSets: pending changes detected",
			elasticsearch: elasticsearch,
			k8sClient: k8s.NewFakeClient(
				scriptsConfigMap,
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:        esName + "-es-" + nodeSetName + "-0",
						Namespace:   namespace,
						Labels:      map[string]string{label.ClusterNameLabelName: esName},
						Annotations: map[string]string{esv1.RestartTriggerAnnotation: "v1"},
					},
				},
			),
			actualSets:     matchingActualSets, // built without the pod, so annotation is absent
			resolvedConfig: resolvedConfig,
			want:           true,
		},
		{
			name:          "BuildExpectedResources fails: error propagated",
			elasticsearch: elasticsearch,
			k8sClient:     k8sClient,
			actualSets:    es_sset.StatefulSetList{},
			resolvedConfig: nodespec.ResolvedConfig{
				NodeSetConfigs: map[string]settings.CanonicalConfig{},
			},
			want:       false,
			wantErrMsg: fmt.Sprintf("no pre-computed config for NodeSet %s", nodeSetName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				BaseDriver: driver.BaseDriver{
					Parameters: driver.Parameters{
						Client: tt.k8sClient,
						ES:     tt.elasticsearch,
						OperatorParameters: operator.Parameters{
							SetDefaultSecurityContext: tt.setDefaultSecurityContext,
						},
					},
				},
			}

			got, err := d.hasPendingSpecChanges(context.Background(), tt.actualSets, state, tt.resolvedConfig, nil)
			if tt.wantErrMsg != "" {
				require.EqualError(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

type fakeESShutdownClient struct {
	esclient.Client
	restartedNodeIDs []string
	removedNodeIDs   []string
	deletedNodeIDs   map[string]struct{}
}

func (f *fakeESShutdownClient) GetNodes(_ context.Context) (esclient.Nodes, error) {
	nodes := make(map[string]esclient.Node, len(f.restartedNodeIDs)+len(f.removedNodeIDs))
	for _, id := range f.restartedNodeIDs {
		nodes[id] = esclient.Node{Name: id}
	}
	for _, id := range f.removedNodeIDs {
		nodes[id] = esclient.Node{Name: id}
	}
	return esclient.Nodes{
		Nodes: nodes,
	}, nil
}

func (f *fakeESShutdownClient) GetShutdown(_ context.Context, _ *string) (esclient.ShutdownResponse, error) {
	shutdownNodes := make([]esclient.NodeShutdown, 0, len(f.restartedNodeIDs)+len(f.removedNodeIDs))
	for _, nodeID := range f.restartedNodeIDs {
		shutdownNodes = append(shutdownNodes, esclient.NodeShutdown{NodeID: nodeID, Type: string(esclient.Restart), Status: esclient.ShutdownComplete})
	}
	for _, nodeID := range f.removedNodeIDs {
		shutdownNodes = append(shutdownNodes, esclient.NodeShutdown{NodeID: nodeID, Type: string(esclient.Remove)})
	}

	return esclient.ShutdownResponse{
		Nodes: shutdownNodes,
	}, nil
}

func (f *fakeESShutdownClient) DeleteShutdown(_ context.Context, nodeID string) error {
	if f.deletedNodeIDs == nil {
		f.deletedNodeIDs = map[string]struct{}{}
	}
	f.deletedNodeIDs[nodeID] = struct{}{}
	return nil
}

func (f *fakeESShutdownClient) Version() version.Version {
	return version.MustParse("9.3.1")
}

func TestDriver_reconcileCriticalStepsWhilePaused(t *testing.T) {
	const esName = "test-cluster"
	const namespace = "test-ns"
	const nodeSetName = "default"

	scriptsConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.ScriptsConfigMap(esName),
			Namespace: namespace,
		},
	}

	elasticsearch := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esName,
			Namespace: namespace,
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.17.0",
			NodeSets: []esv1.NodeSet{
				{Name: nodeSetName, Count: 3},
			},
		},
	}

	// Pre-compute matching StatefulSets for the "no pending changes" case. These are used as
	// pre-existing k8s objects so that RetrieveActualStatefulSets returns them and hasPendingSpecChanges
	// finds no diff against the re-computed expected resources.
	setupClient := k8s.NewFakeClient(scriptsConfigMap)
	resolvedConfig, err := ResolveConfig(context.Background(), setupClient, elasticsearch, corev1.IPv4Protocol, false)
	require.NoError(t, err)

	sharedState := &shared.ReconcileState{
		Meta: metadata.Metadata{},
	}

	expectedResources, err := nodespec.BuildExpectedResources(
		context.Background(), setupClient, elasticsearch, nil,
		nil, false, sharedState.Meta, resolvedConfig,
	)
	require.NoError(t, err)
	matchingStatefulSets := expectedResources.StatefulSets()

	tests := []struct {
		name                 string
		k8sObjects           []crclient.Object // pre-existing StatefulSets beyond scriptsConfigMap
		resolvedConfig       nodespec.ResolvedConfig
		failK8sClient        bool
		restartedNodeIDs     []string
		removedNodeIDs       []string
		wantErr              bool
		wantRequeue          bool
		wantCondStatus       corev1.ConditionStatus
		wantCondMsgSubstr    string
		wantEvents           []events.Event
		wantClearedShutdowns []string
	}{
		{
			name:           "actual StatefulSets match expected: no pending changes",
			k8sObjects:     statefulSetsAsObjects(matchingStatefulSets),
			resolvedConfig: resolvedConfig,
		},
		{
			name:              "no actual StatefulSets: pending changes detected",
			resolvedConfig:    resolvedConfig,
			wantRequeue:       true,
			wantCondStatus:    corev1.ConditionTrue,
			wantCondMsgSubstr: "spec changes are pending and will be applied on resume",
			wantEvents: []events.Event{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    events.EventReasonPaused,
					Action:    events.EventActionPendingOrchestrationChanges,
				},
			},
		},
		{
			name:                 "restarted pods are cleared from shutdown API and removed pods are not",
			resolvedConfig:       resolvedConfig,
			restartedNodeIDs:     []string{"node-to-restart"},
			removedNodeIDs:       []string{"node-to-remove"},
			wantClearedShutdowns: []string{"node-to-restart"},
			wantCondStatus:       corev1.ConditionTrue,
			wantCondMsgSubstr:    "spec changes are pending and will be applied on resume",
			wantEvents: []events.Event{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    events.EventReasonPaused,
					Action:    events.EventActionPendingOrchestrationChanges,
				},
			},
		},
		{
			name: "BuildExpectedResources fails: error is propagated, condition not set",
			resolvedConfig: nodespec.ResolvedConfig{
				NodeSetConfigs: map[string]settings.CanonicalConfig{},
			},
			wantErr: true,
		},
		{
			name:           "k8s client failure: error is propagated",
			failK8sClient:  true,
			resolvedConfig: resolvedConfig,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var k8sClient k8s.Client
			if tt.failK8sClient {
				k8sClient = k8s.NewFailingClient(errors.New("k8s client failed"))
			} else {
				objects := append([]crclient.Object{scriptsConfigMap}, tt.k8sObjects...)
				k8sClient = k8s.NewFakeClient(objects...)
			}

			shutdownClient := &fakeESShutdownClient{
				restartedNodeIDs: tt.restartedNodeIDs,
				removedNodeIDs:   tt.removedNodeIDs,
			}
			sharedState.ESClient = shutdownClient

			reconcileState := reconcile.MustNewState(elasticsearch)
			d := &Driver{
				BaseDriver: driver.BaseDriver{
					Parameters: driver.Parameters{
						Client:         k8sClient,
						ES:             elasticsearch,
						ReconcileState: reconcileState,
						Expectations: &expectations.Expectations{
							ExpectedGenerations:  expectations.NewExpectedGenerations(k8sClient, nil),
							ExpectedPodDeletions: expectations.NewExpectedPodDeletions(k8sClient),
						},
					},
				},
			}

			results := d.reconcileCriticalStepsWhilePaused(context.Background(), sharedState, tt.resolvedConfig, nil)
			if tt.wantErr {
				require.Truef(t, results.HasError(), "expected error to exist")
				return
			}
			require.False(t, results.HasError(), "expected error to not exist")

			// Check events (inspected before Apply() to avoid the health-degraded check).
			recordedEvents := reconcileState.Events()
			require.Len(t, recordedEvents, len(tt.wantEvents))
			for i, want := range tt.wantEvents {
				assert.Equal(t, want.EventType, recordedEvents[i].EventType)
				assert.Equal(t, want.Reason, recordedEvents[i].Reason)
				assert.Equal(t, want.Action, recordedEvents[i].Action)
			}

			// Check phase via Apply(), which surfaces the internal status.
			_, updatedES := reconcileState.Apply()
			require.NotNil(t, updatedES)

			idx := updatedES.Status.Conditions.Index(commonv1.OrchestrationPaused)
			assert.Equal(t, idx >= 0, tt.wantCondStatus != "", "index of OrchestrationPaused condition was [%d] when wantCondStatus was [%s]", idx, tt.wantCondStatus)
			if tt.wantCondStatus != "" {
				require.GreaterOrEqual(t, idx, 0, "expected OrchestrationPaused condition")
				assert.Equal(t, tt.wantCondStatus, updatedES.Status.Conditions[idx].Status)
				assert.Contains(t, updatedES.Status.Conditions[idx].Message, tt.wantCondMsgSubstr)
			}
			if tt.wantRequeue {
				requeue, _ := results.Aggregate()
				assert.NotZero(t, requeue.RequeueAfter, "expected requeue")
			}

			// Check cleared shutdowns
			assert.Lenf(t, shutdownClient.deletedNodeIDs, len(tt.wantClearedShutdowns), "Expected %d nodes to be shutdown but got %d", len(tt.wantClearedShutdowns), len(shutdownClient.deletedNodeIDs))
			for _, want := range tt.wantClearedShutdowns {
				_, cleared := shutdownClient.deletedNodeIDs[want]
				assert.Truef(t, cleared, "%s was not cleared as expected", want)
			}
		})
	}
}

func statefulSetsAsObjects(sets es_sset.StatefulSetList) []crclient.Object {
	objects := make([]crclient.Object, len(sets))
	for i := range sets {
		objects[i] = &sets[i]
	}
	return objects
}

func Test_hasSpecDiff(t *testing.T) {
	tests := []struct {
		name string
		this es_sset.StatefulSetList
		that es_sset.StatefulSetList
		want bool
	}{
		{
			name: "both this and that statefulset are nil",
			this: nil,
			that: nil,
			want: false,
		},
		{
			name: "both this and that statefulset are empty",
			this: es_sset.StatefulSetList{},
			that: es_sset.StatefulSetList{},
			want: false,
		},
		{
			name: "one statefulset exists in this but not that",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sset1",
					},
					Spec: appsv1.StatefulSetSpec{},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sset2",
					},
					Spec: appsv1.StatefulSetSpec{},
				},
			},
			want: true,
		},
		{
			name: "this is non-empty, that is nil",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{},
				},
			},
			that: nil,
			want: true,
		},
		{
			name: "this is nil, that is non-empty",
			this: nil,
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{},
				},
			},
			want: true,
		},
		{
			name: "same name but different ordinals",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Ordinals: &appsv1.StatefulSetOrdinals{Start: 3}}),
					},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Ordinals: &appsv1.StatefulSetOrdinals{Start: 5}}),
					},
				},
			},
			want: true,
		},
		{
			name: "multiple StatefulSets, all specs match",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)}),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset2",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)}),
					},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)}),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset2",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)}),
					},
				},
			},
			want: false,
		},
		{
			name: "multiple StatefulSets, one has a different number of replicas",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)}),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset2",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)}),
					},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)}),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset2",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](2)}),
					},
				},
			},
			want: true,
		},
		{
			name: "StatefulSet exists in both this and that and have equal specs",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)}),
					},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "sset1",
						Labels: hash.SetTemplateHashLabel(nil, appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)}),
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasSpecDiff(context.Background(), tt.this, tt.that)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDriver_maybeResetPausedCondition(t *testing.T) {
	elasticsearch := esv1.Elasticsearch{
		Status: esv1.ElasticsearchStatus{
			Conditions: make(commonv1.Conditions, 0, 1),
		},
	}

	d := &Driver{
		BaseDriver: driver.BaseDriver{
			Parameters: driver.Parameters{
				ES:             elasticsearch,
				ReconcileState: reconcile.MustNewState(elasticsearch),
			},
		},
	}

	t.Run("maybeResetPausedCondition called when OrchestrationPaused has never been set should remain unset", func(t *testing.T) {
		d.maybeResetPausedCondition()
		assert.Equal(t, -1, elasticsearch.Status.Conditions.Index(commonv1.OrchestrationPaused), "OrchestrationPaused should not be set")
	})

	t.Run("maybeResetPausedCondition called when OrchestrationPaused is already False should remain False", func(t *testing.T) {
		d.ES.Status.Conditions = commonv1.Conditions{{Type: commonv1.OrchestrationPaused, Status: corev1.ConditionFalse}}
		d.ReconcileState.Conditions = make(commonv1.Conditions, 0, 1)

		d.maybeResetPausedCondition()
		idx := d.ReconcileState.Conditions.Index(commonv1.OrchestrationPaused)
		require.Equal(t, 0, idx, "OrchestrationPaused should now be set on ReconcileState conditions")
		condition := d.ReconcileState.Conditions[idx]
		assert.Equal(t, corev1.ConditionFalse, condition.Status, "OrchestrationPaused condition should still be set to False on ReconcileState")
		assert.NotEmptyf(t, condition.Message, "OrchestrationPaused condition message should have been set")
	})

	t.Run("maybeResetPausedCondition called when OrchestrationPaused is True should reset to False", func(t *testing.T) {
		d.ES.Status.Conditions = commonv1.Conditions{{Type: commonv1.OrchestrationPaused, Status: corev1.ConditionTrue}}

		d.maybeResetPausedCondition()
		idx := d.ReconcileState.Conditions.Index(commonv1.OrchestrationPaused)
		require.Equal(t, 0, idx, "OrchestrationPaused should now be set on ReconcileState conditions")
		condition := d.ReconcileState.Conditions[idx]
		assert.Equal(t, corev1.ConditionFalse, condition.Status, "OrchestrationPaused condition should be reset to False")
		assert.NotEmptyf(t, condition.Message, "OrchestrationPaused condition message should have been set")
	})
}

func Test_allNodesRunningServiceAccounts(t *testing.T) {
	type args struct {
		saTokens       user.ServiceAccountTokens
		allPods        set.StringSet
		securityClient esclient.SecurityClient
	}
	tests := []struct {
		name    string
		args    args
		want    *bool
		wantErr bool
	}{
		{
			name: "All nodes are running with expected tokens",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"}),
				allPods: set.Make("elasticsearch-sample-es-default-1", "elasticsearch-sample-es-default-0"),
			},
			want: ptr.To[bool](true),
		},
		{
			name: "One node is not running with an expected token",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0"}),
				allPods: set.Make("elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"),
			},
			want: ptr.To[bool](false),
		},
		{
			name: "More nodes running with tokens than expected",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"}),
				allPods: set.Make("elasticsearch-sample-es-default-0"),
			},
			want: ptr.To[bool](true),
		},
		{
			name: "No expected tokens",
			args: args{
				saTokens:       []user.ServiceAccountToken{},
				securityClient: newFakeSecurityClient(),
				allPods:        set.Make("elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-0"),
			},
			want: nil,
		},
		{
			name: "No Pods",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"}),
				allPods: set.Make(),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := allNodesRunningServiceAccounts(context.TODO(), tt.args.saTokens, tt.args.allPods, tt.args.securityClient)
			if (err != nil) != tt.wantErr {
				t.Errorf("Driver.isServiceAccountsReady() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

type fakeSecurityClient struct {
	// namespacedService -> ServiceAccountCredential
	serviceAccountCredentials map[string]esclient.ServiceAccountCredential
	apiKeys                   map[string]esclient.APIKey
}

var _ esclient.SecurityClient = (*fakeSecurityClient)(nil)

func (f *fakeSecurityClient) GetServiceAccountCredentials(_ context.Context, namespacedService string) (esclient.ServiceAccountCredential, error) {
	serviceAccountCredential := f.serviceAccountCredentials[namespacedService]
	return serviceAccountCredential, nil
}

func (f *fakeSecurityClient) GetAPIKeysByName(_ context.Context, name string) (esclient.APIKeyList, error) {
	apiKeys := esclient.APIKeyList{
		APIKeys: []esclient.APIKey{f.apiKeys[name]},
	}
	return apiKeys, nil
}

func (f *fakeSecurityClient) CreateAPIKey(_ context.Context, request esclient.APIKeyCreateRequest) (esclient.APIKeyCreateResponse, error) {
	apiKey := esclient.APIKeyCreateResponse{
		ID:      f.apiKeys[request.Name].ID,
		Name:    f.apiKeys[request.Name].Name,
		APIKey:  f.apiKeys[request.Name].APIKey,
		Encoded: f.apiKeys[request.Name].Encoded,
	}
	return apiKey, nil
}

func (f *fakeSecurityClient) InvalidateAPIKeys(_ context.Context, request esclient.APIKeysInvalidateRequest) (esclient.APIKeysInvalidateResponse, error) {
	response := esclient.APIKeysInvalidateResponse{
		InvalidatedAPIKeys: request.IDs,
	}
	return response, nil
}

func newFakeSecurityClient() *fakeSecurityClient {
	return &fakeSecurityClient{
		serviceAccountCredentials: make(map[string]esclient.ServiceAccountCredential),
		apiKeys:                   make(map[string]esclient.APIKey),
	}
}

func (f *fakeSecurityClient) withFileTokens(namespacedService, tokenName string, nodes []string) *fakeSecurityClient {
	serviceAccountCredential, exists := f.serviceAccountCredentials[namespacedService]
	if !exists {
		serviceAccountCredential.NodesCredentials = esclient.NodesCredentials{
			FileTokens: make(map[string]esclient.FileToken),
		}
	}

	serviceAccountCredential.NodesCredentials.FileTokens[tokenName] = esclient.FileToken{
		Nodes: nodes,
	}
	f.serviceAccountCredentials[namespacedService] = serviceAccountCredential
	return f
}
