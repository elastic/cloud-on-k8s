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

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func fakeEsClient(healthRespErr, infoRespErr, licenseRespErr bool) client.Client {
	return client.NewMockClient(version.MustParse("6.8.0"), func(req *http.Request) *http.Response {
		statusCode := 200
		var respBody io.ReadCloser

		if strings.Contains(req.URL.RequestURI(), "health") {
			respBody = ioutil.NopCloser(bytes.NewBufferString(fixtures.HealthSample))
			if healthRespErr {
				statusCode = 500
			}
		}

		if req.URL.RequestURI() == "/" {
			respBody = ioutil.NopCloser(bytes.NewBufferString(fixtures.InfoSample))
			if infoRespErr {
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
		wantInfo    bool
		wantLicense bool
	}{
		{
			name:        "health, license and keystore ok",
			wantHealth:  true,
			wantInfo:    true,
			wantLicense: true,
		},
		{
			name:        "info error",
			wantHealth:  true,
			wantInfo:    false,
			wantLicense: true,
		},
		{
			name:        "health error",
			wantHealth:  false,
			wantInfo:    true,
			wantLicense: true,
		},
		{
			name:        "license error",
			wantHealth:  false,
			wantInfo:    false,
			wantLicense: true,
		},
		{
			name:        "info and state error",
			wantHealth:  false,
			wantInfo:    false,
			wantLicense: true,
		},
		{
			name:        "keystore error",
			wantHealth:  true,
			wantInfo:    true,
			wantLicense: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := types.NamespacedName{Namespace: "ns1", Name: "es1"}
			esClient := fakeEsClient(!tt.wantHealth, !tt.wantInfo, !tt.wantLicense)
			state := RetrieveState(context.Background(), cluster, esClient)
			if tt.wantHealth {
				require.NotNil(t, state.ClusterHealth)
				require.Equal(t, 3, state.ClusterHealth.NumberOfNodes)
			}
			if tt.wantInfo {
				require.NotNil(t, state.ClusterInfo)
				require.Equal(t, "LGA3VblKTNmzP6Q6SWxfkw", state.ClusterInfo.ClusterUUID)
			}
			if tt.wantLicense {
				require.NotNil(t, state.ClusterLicense)
				require.Equal(t, "893361dc-9749-4997-93cb-802e3d7fa4xx", state.ClusterLicense.UID)
			}
		})
	}
}
