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

func fakeEsClient(healthRespErr bool) client.Client {
	return client.NewMockClient(version.MustParse("6.8.0"), func(req *http.Request) *http.Response {
		statusCode := 200
		var respBody io.ReadCloser

		if strings.Contains(req.URL.RequestURI(), "health") {
			respBody = ioutil.NopCloser(bytes.NewBufferString(fixtures.HealthSample))
			if healthRespErr {
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
		name       string
		wantHealth bool
	}{
		{
			name:       "health ok",
			wantHealth: true,
		},
		{
			name:       "health error",
			wantHealth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := types.NamespacedName{Namespace: "ns1", Name: "es1"}
			esClient := fakeEsClient(!tt.wantHealth)
			state := RetrieveState(context.Background(), cluster, esClient)
			if tt.wantHealth {
				require.NotNil(t, state.ClusterHealth)
				require.Equal(t, 3, state.ClusterHealth.NumberOfNodes)
			}
		})
	}
}
