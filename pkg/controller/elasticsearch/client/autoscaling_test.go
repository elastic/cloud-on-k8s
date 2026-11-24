// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
)

func TestClient_CreateAutoscalingPolicy(t *testing.T) {
	tests := []struct {
		name         string
		policyName   string
		expectedPath string
		policy       v1alpha1.AutoscalingPolicy
		wantBody     string
	}{
		{
			name:         "clear deciders",
			policyName:   "di",
			expectedPath: "/_autoscaling/policy/di",
			policy:       v1alpha1.AutoscalingPolicy{Roles: []string{"data", "ingest"}},
			wantBody:     `{"roles":["data","ingest"],"deciders":{}}`,
		},
		{
			name:         "set deciders",
			policyName:   "di",
			expectedPath: "/_autoscaling/policy/di",
			policy: v1alpha1.AutoscalingPolicy{
				Roles:    []string{"ml"},
				Deciders: map[string]v1alpha1.DeciderSettings{"fixed": {"memory": "4gb", "nodes": "1"}},
			},
			wantBody: `{"roles":["ml"],"deciders":{"fixed":{"memory":"4gb","nodes":"1"}}}`,
		},
		{
			name:         "clear roles",
			policyName:   "di",
			expectedPath: "/_autoscaling/policy/di",
			wantBody:     `{"roles":[],"deciders":{}}`,
		},
		{
			name:         "escape policy name",
			policyName:   "data#1",
			expectedPath: "/_autoscaling/policy/data%231",
			policy:       v1alpha1.AutoscalingPolicy{Roles: []string{"data"}},
			wantBody:     `{"roles":["data"],"deciders":{}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testClient := client.NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
				require.Equal(t, http.MethodPut, req.Method)
				require.Equal(t, tt.expectedPath, req.URL.EscapedPath())
				require.Empty(t, req.URL.Fragment)
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.JSONEq(t, tt.wantBody, string(body))
				return client.NewMockResponse(http.StatusOK, req, `{"acknowledged":true}`)
			})
			require.NoError(t, testClient.CreateAutoscalingPolicy(context.Background(), tt.policyName, tt.policy))
		})
	}
}

func TestClient_DeleteAutoscalingPolicy(t *testing.T) {
	tests := []struct {
		policyName   string
		expectedPath string
	}{
		{
			policyName:   "di",
			expectedPath: "/_autoscaling/policy/di",
		},
		{
			policyName:   "data#1",
			expectedPath: "/_autoscaling/policy/data%231",
		},
	}
	for _, tt := range tests {
		t.Run(tt.policyName, func(t *testing.T) {
			testClient := client.NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
				require.Equal(t, http.MethodDelete, req.Method)
				require.Equal(t, tt.expectedPath, req.URL.EscapedPath())
				return client.NewMockResponse(http.StatusOK, req, `{"acknowledged":true}`)
			})
			require.NoError(t, testClient.DeleteAutoscalingPolicy(context.Background(), tt.policyName))
		})
	}
}

func TestClient_GetAutoscalingCapacity(t *testing.T) {
	testClient := client.NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
		require.Equal(t, "/_autoscaling/capacity", req.URL.Path)
		fixture, err := os.ReadFile(filepath.Join("testdata", "autoscaling.json"))
		assert.NoError(t, err)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(fixture)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	got, err := testClient.GetAutoscalingCapacity(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(got.Policies))

	// Get response from the data decider
	dataCapacity, hasDataCapacity := got.Policies["di"]
	assert.True(t, hasDataCapacity)

	// Required capacity
	assert.Equal(
		t,
		client.AutoscalingCapacityInfo{
			Node: client.AutoscalingResources{
				Processors: newCapacity("2.5"),
				Storage:    newCapacity("165155770"),
				Memory:     nil, // No memory capacity expected for the data deciders
			},
			Total: client.AutoscalingResources{
				Processors: newCapacity("5.0"),
				Storage:    newCapacity("3069911040"),
				Memory:     nil, // No memory capacity expected for the data deciders
			},
		},
		dataCapacity.RequiredCapacity,
	)

	// Observed capacity
	assert.Equal(
		t,
		client.AutoscalingCapacityInfo{
			Node: client.AutoscalingResources{
				Storage: newCapacity("1023303680"),
				Memory:  newCapacity("2147483648"),
			},
			Total: client.AutoscalingResources{
				Storage: newCapacity("3069911040"),
				Memory:  newCapacity("6442450944"),
			},
		},
		dataCapacity.CurrentCapacity,
	)

	// Observed data/ingest nodes
	assert.ElementsMatch(
		t,
		[]client.AutoscalingNodeInfo{{"mldi-sample-es-di-0"}, {"mldi-sample-es-di-1"}, {"mldi-sample-es-di-2"}},
		dataCapacity.CurrentNodes,
	)

	// Get response from the ml decider
	mlCapacity, hasMLCapacity := got.Policies["ml"]
	assert.True(t, hasMLCapacity)

	// Required ML capacity
	assert.Equal(
		t,
		mlCapacity.RequiredCapacity,
		client.AutoscalingCapacityInfo{
			Node: client.AutoscalingResources{
				Storage: nil, // No storage capacity expected from the ML decider
				Memory:  newCapacity("3221225472"),
			},
			Total: client.AutoscalingResources{
				Storage: nil, // No storage capacity expected from the ML decider
				Memory:  newCapacity("6442450944"),
			},
		},
	)

	// Observed ML capacity
	assert.Equal(
		t,
		client.AutoscalingCapacityInfo{
			Node: client.AutoscalingResources{
				Storage: nil,
				Memory:  newCapacity("3221225472"),
			},
			Total: client.AutoscalingResources{
				Storage: nil,
				Memory:  newCapacity("6442450944"),
			},
		},
		mlCapacity.CurrentCapacity,
	)

	// Observed ML nodes
	assert.ElementsMatch(
		t,
		[]client.AutoscalingNodeInfo{{"mldi-sample-es-ml-0"}, {"mldi-sample-es-ml-1"}},
		mlCapacity.CurrentNodes,
	)
}

func newCapacity(q string) *client.AutoscalingCapacity {
	v := client.AutoscalingCapacity{Quantity: resource.MustParse(q)}
	return &v
}
