package observer

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client/test_fixtures"
)

func fakeEsClient(healthRespErr, stateRespErr, licenseRespErr bool) client.Client {
	return client.NewMockClient(func(req *http.Request) *http.Response {
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
			client := fakeEsClient(!tt.wantHealth, !tt.wantState, !tt.wantLicense)
			state := RetrieveState(context.Background(), &client)
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
