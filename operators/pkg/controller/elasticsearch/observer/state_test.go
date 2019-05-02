// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func fakeEsClient(healthRespErr, stateRespErr, licenseRespErr bool) client.Client {
	return client.NewMockClient(version.MustParse("6.7.0"), func(req *http.Request) *http.Response {
		statusCode := 200
		var respBody io.ReadCloser

		if strings.Contains(req.URL.RequestURI(), "health") {
			respBody = ioutil.NopCloser(bytes.NewBufferString(fixtures.HealthSample))
			if healthRespErr {
				statusCode = 500
			}
		}

		if strings.Contains(req.URL.RequestURI(), "state") {
			respBody = ioutil.NopCloser(bytes.NewBufferString(fixtures.ClusterStateSample))
			if stateRespErr {
				statusCode = 500
			}
		}

		if strings.Contains(req.URL.RequestURI(), "license") {
			respBody = ioutil.NopCloser(bytes.NewBufferString(fixtures.LicenseGetSample))
			if licenseRespErr {
				statusCode = 500
			}

		}

		return &http.Response{
			StatusCode: statusCode,
			Body:       respBody,
			Header:     make(http.Header),
			Request:    req,
		}
	})
}

func createMockPMClient() processmanager.Client {
	return processmanager.NewMockClient(processmanager.ProcessStatus{State: processmanager.Started}, nil)
}

func TestRetrieveState(t *testing.T) {
	tests := []struct {
		name                 string
		wantHealth           bool
		wantState            bool
		wantLicense          bool
		wantKeystoreStatuses bool
	}{
		{
			name:                 "state, health, license and keystore ok",
			wantHealth:           true,
			wantState:            true,
			wantLicense:          true,
			wantKeystoreStatuses: true,
		},
		{
			name:                 "state error",
			wantHealth:           true,
			wantState:            false,
			wantLicense:          true,
			wantKeystoreStatuses: true,
		},
		{
			name:                 "health error",
			wantHealth:           false,
			wantState:            true,
			wantLicense:          true,
			wantKeystoreStatuses: true,
		},
		{
			name:                 "license error",
			wantHealth:           false,
			wantState:            false,
			wantLicense:          true,
			wantKeystoreStatuses: true,
		},
		{
			name:                 "health and state error",
			wantHealth:           false,
			wantState:            false,
			wantLicense:          true,
			wantKeystoreStatuses: true,
		},
		{
			name:                 "keystore error",
			wantHealth:           true,
			wantState:            true,
			wantLicense:          true,
			wantKeystoreStatuses: false,
		},
	}

	ns := "ns1"
	clusterName := "es1"
	pod1 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      clusterName + "-es-azertyuiop",
			Labels: map[string]string{
				label.ClusterNameLabelName: clusterName,
			},
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{Type: v1.PodReady, Status: v1.ConditionTrue},
				{Type: v1.ContainersReady, Status: v1.ConditionTrue},
			},
		},
	}
	pod2 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      clusterName + "-es-qsdfghjklm",
			Labels: map[string]string{
				label.ClusterNameLabelName: clusterName,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrapClient(fake.NewFakeClient(&pod1, &pod2))
			esClient := fakeEsClient(!tt.wantHealth, !tt.wantState, !tt.wantLicense)
			cluster := types.NamespacedName{Namespace: ns, Name: clusterName}
			state := RetrieveState(context.Background(), cluster, esClient, k8sClient, createMockPMClient)
			if tt.wantHealth {
				require.NotNil(t, state.ClusterHealth)
				require.Equal(t, state.ClusterHealth.NumberOfNodes, 3)
			}
			if tt.wantState {
				require.NotNil(t, state.ClusterState)
				require.Equal(t, state.ClusterState.ClusterUUID, "LyyITZoWSlO1NYEOQ6qYsA")
			}
			if tt.wantLicense {
				require.NotNil(t, state.ClusterLicense)
				require.Equal(t, state.ClusterLicense.UID, "893361dc-9749-4997-93cb-802e3d7fa4xx")
			}
			if tt.wantKeystoreStatuses {
				require.NotNil(t, state.KeystoreStatuses)
				require.Equal(t, 2, len(state.KeystoreStatuses))
				require.Equal(t, keystore.RunningState, state.KeystoreStatuses[0].State)
				require.Equal(t, keystore.WaitingState, state.KeystoreStatuses[1].State)
			}
		})
	}
}
