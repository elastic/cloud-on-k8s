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

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client/test_fixtures"
)

func fakeEsClient(healthRespErr bool, stateRespErr bool) client.Client {
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
		wantState  bool
	}{
		{
			name:       "both state and health ok",
			wantHealth: true,
			wantState:  true,
		},
		{
			name:       "state error",
			wantHealth: true,
			wantState:  false,
		},
		{
			name:       "health error",
			wantHealth: false,
			wantState:  true,
		},
		{
			name:       "health and state error",
			wantHealth: false,
			wantState:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fakeEsClient(!tt.wantHealth, !tt.wantState)
			state := RetrieveState(context.Background(), &client)
			if tt.wantHealth {
				require.NotNil(t, state.ClusterHealth)
				require.Equal(t, state.ClusterHealth.NumberOfNodes, 3)
			}
			if tt.wantState {
				require.NotNil(t, state.ClusterState)
				require.Equal(t, state.ClusterState.ClusterUUID, "LyyITZoWSlO1NYEOQ6qYsA")
			}
		})
	}
}
