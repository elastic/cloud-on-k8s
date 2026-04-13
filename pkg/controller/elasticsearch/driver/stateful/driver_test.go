// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
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
