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
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
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

func TestRetrieveState(t *testing.T) {
	tests := []struct {
		name        string
		wantHealth  bool
		wantState   bool
		wantLicense bool
	}{
		{
			name:        "state, health and license ok",
			wantHealth:  true,
			wantState:   true,
			wantLicense: true,
		},
		{
			name:        "state error",
			wantHealth:  true,
			wantState:   false,
			wantLicense: true,
		},
		{
			name:        "health error",
			wantHealth:  false,
			wantState:   true,
			wantLicense: true,
		},
		{
			name:        "license error",
			wantHealth:  false,
			wantState:   false,
			wantLicense: true,
		},
		{
			name:        "health and state error",
			wantHealth:  false,
			wantState:   false,
			wantLicense: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrapClient(fake.NewFakeClient())
			esClient := fakeEsClient(!tt.wantHealth, !tt.wantState, !tt.wantLicense)
			cluster := types.NamespacedName{Namespace: "ns1", Name: "es1"}
			state := RetrieveState(context.Background(), k8sClient, cluster, esClient, nil, nil)
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
		})
	}
}
