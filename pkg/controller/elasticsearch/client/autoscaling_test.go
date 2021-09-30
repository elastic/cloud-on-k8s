// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	. "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_CreateAutoscalingPolicy(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
	}{
		{
			expectedPath: "/_autoscaling/policy/di",
		},
	}
	for _, tt := range tests {
		testClient := NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(`{"acknowledged": true}`)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		in := esv1.AutoscalingPolicy{
			Roles: []string{"data", "ingest"},
		}
		assert.NoError(t, testClient.CreateAutoscalingPolicy(context.Background(), "di", in))
	}
}

func TestClient_DeleteAutoscalingPolicies(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
	}{
		{
			expectedPath: "/_autoscaling/policy/*",
		},
	}
	for _, tt := range tests {
		testClient := NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(`{"acknowledged": true}`)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		assert.NoError(t, testClient.DeleteAutoscalingPolicies(context.Background()))
	}
}

func TestClient_GetAutoscalingCapacity(t *testing.T) {
	testClient := NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
		require.Equal(t, "/_autoscaling/capacity", req.URL.Path)
		fixture, err := ioutil.ReadFile(filepath.Join("testdata", "autoscaling.json"))
		assert.NoError(t, err)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader(fixture)),
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
		AutoscalingCapacityInfo{
			Node: AutoscalingResources{
				Storage: newCapacity(165155770),
				Memory:  nil, // No memory capacity expected for the data deciders
			},
			Total: AutoscalingResources{
				Storage: newCapacity(3069911040),
				Memory:  nil, // No memory capacity expected for the data deciders
			},
		},
		dataCapacity.RequiredCapacity,
	)

	// Observed capacity
	assert.Equal(
		t,
		AutoscalingCapacityInfo{
			Node: AutoscalingResources{
				Storage: newCapacity(1023303680),
				Memory:  newCapacity(2147483648),
			},
			Total: AutoscalingResources{
				Storage: newCapacity(3069911040),
				Memory:  newCapacity(6442450944),
			},
		},
		dataCapacity.CurrentCapacity,
	)

	// Observed data/ingest nodes
	assert.ElementsMatch(
		t,
		[]AutoscalingNodeInfo{{"mldi-sample-es-di-0"}, {"mldi-sample-es-di-1"}, {"mldi-sample-es-di-2"}},
		dataCapacity.CurrentNodes,
	)

	// Get response from the ml decider
	mlCapacity, hasMLCapacity := got.Policies["ml"]
	assert.True(t, hasMLCapacity)

	// Required ML capacity
	assert.Equal(
		t,
		mlCapacity.RequiredCapacity,
		AutoscalingCapacityInfo{
			Node: AutoscalingResources{
				Storage: nil, // No storage capacity expected from the ML decider
				Memory:  newCapacity(3221225472),
			},
			Total: AutoscalingResources{
				Storage: nil, // No storage capacity expected from the ML decider
				Memory:  newCapacity(6442450944),
			},
		},
	)

	// Observed ML capacity
	assert.Equal(
		t,
		AutoscalingCapacityInfo{
			Node: AutoscalingResources{
				Storage: nil,
				Memory:  newCapacity(3221225472),
			},
			Total: AutoscalingResources{
				Storage: nil,
				Memory:  newCapacity(6442450944),
			},
		},
		mlCapacity.CurrentCapacity,
	)

	// Observed ML nodes
	assert.ElementsMatch(
		t,
		[]AutoscalingNodeInfo{{"mldi-sample-es-ml-0"}, {"mldi-sample-es-ml-1"}},
		mlCapacity.CurrentNodes,
	)
}

func newCapacity(i int) *AutoscalingCapacity {
	v := AutoscalingCapacity(int64(i))
	return &v
}
