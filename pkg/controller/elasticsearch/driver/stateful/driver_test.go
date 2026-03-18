// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

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

func TestDriver_Reconcile_ClientAuthenticationWithoutEnterpriseLicense(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es",
			Namespace: "default",
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.17.0",
			HTTP: commonv1.HTTPConfigWithClientOptions{
				TLS: commonv1.TLSWithClientOptions{
					Client: commonv1.ClientOptions{
						Authentication: true,
					},
				},
			},
			NodeSets: []esv1.NodeSet{
				{
					Name:  "default",
					Count: 1,
				},
			},
		},
	}

	reconcileState, err := reconcile.NewState(es)
	require.NoError(t, err)

	d := &Driver{
		BaseDriver: driver.BaseDriver{
			Parameters: driver.Parameters{
				ES:             es,
				Client:         k8s.NewFakeClient(&es),
				LicenseChecker: commonlicense.MockLicenseChecker{EnterpriseEnabled: false},
				ReconcileState: reconcileState,
				OperatorParameters: operator.Parameters{
					IPFamily: corev1.IPv4Protocol,
				},
			},
		},
	}

	results := d.Reconcile(context.Background())
	result, err := results.Aggregate()

	require.NoError(t, err, "should not return an error, just requeue")
	assert.Equal(t, enterpriseFeaturesDisabledRequeue, result.RequeueAfter, "should requeue after 5 minutes")

	events := reconcileState.Events()
	require.Len(t, events, 1, "expected exactly one event")
	assert.Equal(t, "ReconciliationError", events[0].Reason)
	assert.Equal(t, enterpriseFeaturesDisabledMsg, events[0].Message)
}
