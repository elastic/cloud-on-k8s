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

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
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
		Meta:              metadata.Metadata{},
		KeystoreResources: nil,
	}
	// Build the expected StatefulSets to use as actualSets in the "no diff" cases.
	// existingStatefulSets=nil means no existing ssets yet (new cluster).
	expectedResources, err := nodespec.BuildExpectedResources(
		context.Background(), k8sClient, elasticsearch, state.KeystoreResources,
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
		context.Background(), k8sClient, elasticsearch2, state.KeystoreResources,
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

			got, err := d.hasPendingSpecChanges(context.Background(), tt.actualSets, state, tt.resolvedConfig)
			if tt.wantErrMsg != "" {
				require.EqualError(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDriver_reconcileCriticalSteps(t *testing.T) {
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
		Meta:              metadata.Metadata{},
		KeystoreResources: nil,
	}

	expectedResources, err := nodespec.BuildExpectedResources(
		context.Background(), setupClient, elasticsearch, sharedState.KeystoreResources,
		nil, false, sharedState.Meta, resolvedConfig,
	)
	require.NoError(t, err)
	matchingStatefulSets := expectedResources.StatefulSets()

	tests := []struct {
		name              string
		k8sObjects        []crclient.Object // pre-existing StatefulSets beyond scriptsConfigMap
		resolvedConfig    nodespec.ResolvedConfig
		failK8sClient     bool
		wantErr           bool
		wantPhase         esv1.ElasticsearchOrchestrationPhase
		wantCondStatus    corev1.ConditionStatus
		wantCondMsgSubstr string
		wantEvents        []events.Event
	}{
		{
			name:              "actual StatefulSets match expected: no pending changes",
			k8sObjects:        statefulSetsAsObjects(matchingStatefulSets),
			resolvedConfig:    resolvedConfig,
			wantPhase:         esv1.ElasticsearchOrchestrationPaused,
			wantCondStatus:    corev1.ConditionTrue,
			wantCondMsgSubstr: "no pending spec changes detected",
		},
		{
			name:              "no actual StatefulSets: pending changes detected",
			resolvedConfig:    resolvedConfig,
			wantPhase:         esv1.ElasticsearchOrchestrationPaused,
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

			reconcileState := reconcile.MustNewState(elasticsearch)
			d := &Driver{
				BaseDriver: driver.BaseDriver{
					Parameters: driver.Parameters{
						Client:         k8sClient,
						ES:             elasticsearch,
						ReconcileState: reconcileState,
					},
				},
			}

			err := d.reconcileCriticalSteps(context.Background(), sharedState, tt.resolvedConfig)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Check OrchestrationPaused condition.
			condIdx := reconcileState.Index(esv1.OrchestrationPaused)
			require.GreaterOrEqual(t, condIdx, 0, "OrchestrationPaused condition should be set")
			cond := reconcileState.Conditions[condIdx]
			assert.Equal(t, tt.wantCondStatus, cond.Status)
			assert.Contains(t, cond.Message, tt.wantCondMsgSubstr)

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
			assert.Equal(t, tt.wantPhase, updatedES.Status.Phase)
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
			name: "same name but different replicas",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](5)},
				},
			},
			want: true,
		},
		{
			name: "multiple StatefulSets, all specs match",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset2"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset2"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
				},
			},
			want: false,
		},
		{
			name: "multiple StatefulSets, one has a spec diff",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset2"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset1"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](3)},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sset2"},
					Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](2)},
				},
			},
			want: true,
		},
		{
			name: "StatefulSet exists in both this and that and have equal specs",
			this: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sset1",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: ptr.To[int32](3),
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{},
						},
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "sset1-pvc",
									OwnerReferences: []metav1.OwnerReference{
										{
											APIVersion: "apps/v1",
											Kind:       "StatefulSet",
											Name:       "sset1",
											Controller: ptr.To[bool](true),
										},
										{
											APIVersion: "apps/v1",
											Kind:       "Elasticsearch",
											Name:       "es1",
											Controller: ptr.To[bool](true),
										},
									},
									Annotations: map[string]string{
										"foo": "bar",
										"bar": "foo",
									},
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
									VolumeMode:  ptr.To(corev1.PersistentVolumeBlock),
								},
							},
						},
						ServiceName:                          "sset1",
						PodManagementPolicy:                  appsv1.PodManagementPolicyType("ParallelOnly"),
						UpdateStrategy:                       appsv1.StatefulSetUpdateStrategy{},
						RevisionHistoryLimit:                 ptr.To[int32](3),
						MinReadySeconds:                      2,
						PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{},
						Ordinals:                             &appsv1.StatefulSetOrdinals{Start: 3},
					},
				},
			},
			that: es_sset.StatefulSetList{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sset1",
					},
					Spec: appsv1.StatefulSetSpec{
						Replicas: ptr.To[int32](3),
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{},
						},
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "sset1-pvc",
									OwnerReferences: []metav1.OwnerReference{
										{
											APIVersion: "apps/v1",
											Kind:       "StatefulSet",
											Name:       "sset1",
											Controller: ptr.To[bool](true),
										},
										{
											APIVersion: "apps/v1",
											Kind:       "Elasticsearch",
											Name:       "es1",
											Controller: ptr.To[bool](true),
										},
									},
									Annotations: map[string]string{
										"bar": "foo",
										"foo": "bar",
									},
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
									VolumeMode:  ptr.To(corev1.PersistentVolumeBlock),
								},
							},
						},
						ServiceName:                          "sset1",
						PodManagementPolicy:                  appsv1.PodManagementPolicyType("ParallelOnly"),
						UpdateStrategy:                       appsv1.StatefulSetUpdateStrategy{},
						RevisionHistoryLimit:                 ptr.To[int32](3),
						MinReadySeconds:                      2,
						PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{},
						Ordinals:                             &appsv1.StatefulSetOrdinals{Start: 3},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasSpecDiff(tt.this, tt.that)
			assert.Equal(t, tt.want, got)
		})
	}
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
