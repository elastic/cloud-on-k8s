// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

//
// func TestDriver_hasPendingSpecChanges(t *testing.T) {
//	state := &shared.ReconcileState{
//		Meta: metadata.Metadata{
//			Labels: map[string]string{
//				commonv1.TypeLabelName:         "elasticsearch",
//				label.ClusterNameLabelName:     "test-cluster",
//				label.StatefulSetNameLabelName: "test-cluster-es-nodeset",
//			},
//			Annotations: nil,
//		},
//		KeystoreResources: &keystore.Resources{},
//	}
//
//	resolvedConfig := nodespec.ResolvedConfig{
//		NodeSetConfigs: map[string]essettings.CanonicalConfig{
//			"test-cluster-es-nodeset": {CanonicalConfig: settings.MustCanonicalConfig(commonv1.Config{Data: map[string]any{"user": "true"}})},
//		},
//	}
//
//	elasticsearch := esv1.Elasticsearch{
//		ObjectMeta: metav1.ObjectMeta{
//			Name: "test-cluster",
//		},
//		Spec: esv1.ElasticsearchSpec{
//			Version: "9.3.1",
//			NodeSets: []esv1.NodeSet{
//				{
//					Name:          "test-cluster-es-nodeset",
//					Config:        nil,
//					Count:         0,
//					ZoneAwareness: nil,
//					PodTemplate:   corev1.PodTemplateSpec{},
//					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
//						{
//							ObjectMeta: metav1.ObjectMeta{},
//						},
//					},
//				},
//			},
//		},
//	}
//
//	tests := []struct {
//		name            string
//		buildActualSets func(client k8s.Client) es_sset.StatefulSetList
//		k8sClient       k8s.Client
//		expected        bool
//		expectedErr     error
//	}{
//		{
//			name: "actual stateful sets are equal to expected stateful sets returns false",
//			buildActualSets: func(client k8s.Client) es_sset.StatefulSetList {
//				statefulSet, err := nodespec.BuildStatefulSet(
//					context.Background(),
//					client,
//					elasticsearch,
//					elasticsearch.Spec.NodeSets[0],
//					essettings.CanonicalConfig{
//						CanonicalConfig: settings.MustCanonicalConfig(commonv1.Config{Data: map[string]any{"user": "true"}}),
//					},
//					state.KeystoreResources,
//					es_sset.StatefulSetList{},
//					true,
//					nodespec.PolicyConfig{},
//					state.Meta,
//					"trigger",
//					true,
//				)
//				require.NoError(t, err)
//
//				return es_sset.StatefulSetList{statefulSet}
//			},
//			k8sClient: k8s.NewFakeClient(
//				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-es-scripts"}},
//			),
//			expected:    false,
//			expectedErr: nil,
//		},
//		{
//			name:        "actual stateful sets are different than expected stateful sets returns true",
//			k8sClient:   k8s.NewFakeClient(),
//			expected:    true,
//			expectedErr: nil,
//		},
//		{
//			name:        "error getting expected stateful sets returns error",
//			k8sClient:   k8s.NewFakeClient(),
//			expected:    false,
//			expectedErr: errors.New("error getting expected stateful sets"),
//		},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			d := &Driver{
//				BaseDriver: driver.BaseDriver{
//					Parameters: driver.Parameters{
//						Client: tt.k8sClient,
//						ES:     elasticsearch,
//						OperatorParameters: operator.Parameters{
//							SetDefaultSecurityContext: true,
//						},
//					},
//				},
//			}
//
//			actualSets := tt.buildActualSets(tt.k8sClient)
//
//			hasChanged, err := d.hasPendingSpecChanges(context.Background(), actualSets, state, resolvedConfig)
//			if tt.expectedErr != nil {
//				assert.EqualErrorf(t, err, tt.expectedErr.Error(), "expected error %s but got %s", tt.expectedErr.Error(), err)
//			} else {
//				assert.NoError(t, err)
//			}
//			assert.Equal(t, tt.expected, hasChanged)
//		})
//	}
//}
//
// func TestDriver_reconcileCriticalSteps(t *testing.T) {
//	tests := []struct {
//		name        string
//		actualSets  es_sset.StatefulSetList
//		meta        metadata.Metadata
//		k8sClient   k8s.Client
//		expected    bool
//		expectedErr error
//	}{
//		{},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//
//		})
//	}
//}

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
