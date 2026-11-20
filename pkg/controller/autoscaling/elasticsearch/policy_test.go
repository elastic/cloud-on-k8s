// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	toolsevents "k8s.io/client-go/tools/events"

	autoscalingv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoscaling/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

func Test_attemptOnlineReconciliation_policies(t *testing.T) {
	data := v1alpha1.AutoscalingPolicy{Roles: []string{"data", "ingest"}, Deciders: map[string]v1alpha1.DeciderSettings{}}
	ml := v1alpha1.AutoscalingPolicy{Roles: []string{"ml"}, Deciders: map[string]v1alpha1.DeciderSettings{}}
	fixed := v1alpha1.AutoscalingPolicy{
		Roles:    data.Roles,
		Deciders: map[string]v1alpha1.DeciderSettings{"fixed": {"storage": "19gb", "nodes": "3"}},
	}
	named := func(name string, policy v1alpha1.AutoscalingPolicy) v1alpha1.NamedAutoscalingPolicy {
		return v1alpha1.NamedAutoscalingPolicy{Name: name, AutoscalingPolicy: policy}
	}
	tests := []struct {
		name        string
		existing    map[string]v1alpha1.AutoscalingPolicy
		desired     []v1alpha1.NamedAutoscalingPolicy
		wantDeleted []string
	}{
		{
			name:    "create initial policies",
			desired: []v1alpha1.NamedAutoscalingPolicy{named("data-ingest", data), named("ml", ml)},
		},
		{
			name:    "create policy with escaped name",
			desired: []v1alpha1.NamedAutoscalingPolicy{named("data#1", data)},
		},
		{
			name:     "preserve existing policies with data first",
			existing: map[string]v1alpha1.AutoscalingPolicy{"data-ingest": data, "ml": ml},
			desired:  []v1alpha1.NamedAutoscalingPolicy{named("data-ingest", data), named("ml", ml)},
		},
		{
			name:     "preserve existing policies with data last",
			existing: map[string]v1alpha1.AutoscalingPolicy{"data-ingest": data, "ml": ml},
			desired:  []v1alpha1.NamedAutoscalingPolicy{named("ml", ml), named("data-ingest", data)},
		},
		{
			name:     "update deciders in place",
			existing: map[string]v1alpha1.AutoscalingPolicy{"data-ingest": data},
			desired:  []v1alpha1.NamedAutoscalingPolicy{named("data-ingest", fixed)},
		},
		{
			name:     "remove deciders in place",
			existing: map[string]v1alpha1.AutoscalingPolicy{"data-ingest": fixed},
			desired:  []v1alpha1.NamedAutoscalingPolicy{named("data-ingest", data)},
		},
		{
			name:     "update roles in place",
			existing: map[string]v1alpha1.AutoscalingPolicy{"tier": data},
			desired:  []v1alpha1.NamedAutoscalingPolicy{named("tier", ml)},
		},
		{
			name:        "create renamed policy before removing old policy",
			existing:    map[string]v1alpha1.AutoscalingPolicy{"old-data": data, "ml": ml},
			desired:     []v1alpha1.NamedAutoscalingPolicy{named("data-ingest", data), named("ml", ml)},
			wantDeleted: []string{"old-data"},
		},
		{
			name:        "remove policies absent from desired spec and autoscaler status",
			existing:    map[string]v1alpha1.AutoscalingPolicy{"data-ingest": data, "ml": ml, "orphan": data},
			desired:     []v1alpha1.NamedAutoscalingPolicy{named("data-ingest", data)},
			wantDeleted: []string{"ml", "orphan"},
		},
		{
			name:        "remove obsolete policy with escaped name",
			existing:    map[string]v1alpha1.AutoscalingPolicy{"data-ingest": data, "old#1": ml},
			desired:     []v1alpha1.NamedAutoscalingPolicy{named("data-ingest", data)},
			wantDeleted: []string{"old#1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := make(map[string]v1alpha1.AutoscalingPolicy)
			maps.Copy(current, tt.existing)
			autoscaler := &autoscalingv1alpha1.ElasticsearchAutoscaler{}
			wantPolicies := make(map[string]v1alpha1.AutoscalingPolicy)
			wantCalls := []string{"PUT /_cluster/settings"}
			for _, policy := range tt.desired {
				autoscaler.Spec.AutoscalingPolicySpecs = append(autoscaler.Spec.AutoscalingPolicySpecs,
					v1alpha1.AutoscalingPolicySpec{NamedAutoscalingPolicy: policy})
				wantPolicies[policy.Name] = policy.AutoscalingPolicy
				wantCalls = append(wantCalls, "PUT /_autoscaling/policy/"+policy.Name)
			}
			wantCalls = append(wantCalls, "GET /_autoscaling/capacity")
			var calls, deleted []string
			es := esclient.NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
				calls = append(calls, req.Method+" "+req.URL.Path)
				name := strings.TrimPrefix(req.URL.Path, "/_autoscaling/policy/")
				switch req.Method {
				case http.MethodGet:
					require.Equal(t, "/_autoscaling/capacity", req.URL.Path)
					capacity := esclient.AutoscalingCapacityResult{Policies: make(map[string]esclient.AutoscalingPolicyResult)}
					for name := range current {
						capacity.Policies[name] = esclient.AutoscalingPolicyResult{}
					}
					body, err := json.Marshal(capacity)
					require.NoError(t, err)
					return esclient.NewMockResponse(http.StatusOK, req, string(body))
				case http.MethodPut:
					if req.URL.Path == "/_cluster/settings" {
						return esclient.NewMockResponse(http.StatusOK, req, `{"acknowledged":true}`)
					}
					// Elasticsearch retains fields omitted from a PUT request.
					policy := current[name]
					var update v1alpha1.AutoscalingPolicy
					require.NoError(t, json.NewDecoder(req.Body).Decode(&update))
					if update.Roles != nil {
						policy.Roles = update.Roles
					}
					if update.Deciders != nil {
						policy.Deciders = update.Deciders
					}
					current[name] = policy
				case http.MethodDelete:
					require.NotContains(t, wantPolicies, name, "deleting a desired policy invalidates cached node information")
					require.Contains(t, current, name, "only delete individual policies discovered in Elasticsearch")
					deleted = append(deleted, name)
					delete(current, name)
				default:
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				}
				return esclient.NewMockResponse(http.StatusOK, req, `{"acknowledged":true}`)
			})

			// A second reconciliation must preserve the same policies without deleting them again.
			for iteration := range 2 {
				calls, deleted = nil, nil
				require.NoError(t, reconcilePoliciesForTest(t, autoscaler, es))
				assert.Equal(t, wantPolicies, current)
				require.GreaterOrEqual(t, len(calls), len(wantCalls))
				assert.Equal(t, wantCalls, calls[:len(wantCalls)], "put policies before the single capacity request and clean up afterward")
				if iteration == 0 {
					assert.ElementsMatch(t, tt.wantDeleted, deleted)
					assert.Len(t, calls, len(wantCalls)+len(tt.wantDeleted))
				} else {
					assert.Empty(t, deleted)
					assert.Len(t, calls, len(wantCalls))
				}
			}
		})
	}
}

func Test_attemptOnlineReconciliation_policyErrors(t *testing.T) {
	tests := []struct {
		name       string
		failAt     string
		statusCode int
		wantErr    bool
		wantCalls  []string
	}{
		{
			name:       "capacity failure skips cleanup",
			failAt:     "GET /_autoscaling/capacity",
			statusCode: http.StatusServiceUnavailable,
			wantErr:    true,
			wantCalls:  []string{"PUT /_cluster/settings", "PUT /_autoscaling/policy/data", "PUT /_autoscaling/policy/ml", "GET /_autoscaling/capacity"},
		},
		{
			name:       "failed replacement prevents cleanup",
			failAt:     "PUT /_autoscaling/policy/ml",
			statusCode: http.StatusServiceUnavailable,
			wantErr:    true,
			wantCalls:  []string{"PUT /_cluster/settings", "PUT /_autoscaling/policy/data", "PUT /_autoscaling/policy/ml"},
		},
		{
			name:       "cleanup failure does not fail resource calculation",
			failAt:     "DELETE /_autoscaling/policy/stale",
			statusCode: http.StatusServiceUnavailable,
			wantCalls:  []string{"PUT /_cluster/settings", "PUT /_autoscaling/policy/data", "PUT /_autoscaling/policy/ml", "GET /_autoscaling/capacity", "DELETE /_autoscaling/policy/stale"},
		},
		{
			name:       "already deleted policy is ignored",
			failAt:     "DELETE /_autoscaling/policy/stale",
			statusCode: http.StatusNotFound,
			wantCalls:  []string{"PUT /_cluster/settings", "PUT /_autoscaling/policy/data", "PUT /_autoscaling/policy/ml", "GET /_autoscaling/capacity", "DELETE /_autoscaling/policy/stale"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscaler := &autoscalingv1alpha1.ElasticsearchAutoscaler{}
			for _, name := range []string{"data", "ml"} {
				autoscaler.Spec.AutoscalingPolicySpecs = append(autoscaler.Spec.AutoscalingPolicySpecs,
					v1alpha1.AutoscalingPolicySpec{Name: name, Roles: []string{name}})
			}
			var calls []string
			es := esclient.NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
				call := req.Method + " " + req.URL.Path
				calls = append(calls, call)
				if call == tt.failAt {
					return esclient.NewMockResponse(tt.statusCode, req, `{"error":{"reason":"simulated failure"}}`)
				}
				if req.Method == http.MethodGet {
					return esclient.NewMockResponse(http.StatusOK, req, `{"policies":{"stale":{}}}`)
				}
				return esclient.NewMockResponse(http.StatusOK, req, `{"acknowledged":true}`)
			})
			err := reconcilePoliciesForTest(t, autoscaler, es)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalls, calls)
		})
	}
}

func reconcilePoliciesForTest(t *testing.T, autoscaler *autoscalingv1alpha1.ElasticsearchAutoscaler, esClient esclient.Client) error {
	t.Helper()
	es := esv1.Elasticsearch{Name: "testes", Namespace: "testns"}
	autoscaledNodeSets := esv1.AutoscaledNodeSets{}
	for i, policy := range autoscaler.Spec.AutoscalingPolicySpecs {
		nodeSet := esv1.NodeSet{Name: fmt.Sprintf("nodes-%d", i)}
		es.Spec.NodeSets = append(es.Spec.NodeSets, nodeSet)
		autoscaledNodeSets[policy.Name] = esv1.NodeSetList{nodeSet}
	}
	r := baseReconcileAutoscaling{
		esClientProvider: func(_ context.Context, _ k8s.Client, _ net.Dialer, _ esv1.Elasticsearch) (esclient.Client, error) {
			return esClient, nil
		},
		recorder: toolsevents.NewFakeRecorder(100),
	}
	_, err := r.attemptOnlineReconciliation(context.Background(), es, v1alpha1.NewAutoscalingStatusBuilder(), autoscaledNodeSets, autoscaler)
	return err
}

func Test_deleteObsoletePolicies(t *testing.T) {
	specs := v1alpha1.AutoscalingPolicySpecs{{Name: "data"}}
	capacity := esclient.AutoscalingCapacityResult{Policies: map[string]esclient.AutoscalingPolicyResult{
		"stale-c": {}, "data": {}, "stale-a": {}, "stale-b": {},
	}}
	allDeletes := []string{"stale-a", "stale-b", "stale-c"}
	tests := []struct {
		name       string
		failures   map[string]int
		canceled   bool
		wantCalls  []string
		wantFailed []string
	}{
		{
			name:      "delete all obsolete policies in name order and preserve the desired policy",
			wantCalls: allDeletes,
		},
		{
			name:       "failed deletes do not prevent deleting other obsolete policies",
			failures:   map[string]int{"stale-a": http.StatusServiceUnavailable, "stale-c": http.StatusServiceUnavailable},
			wantCalls:  allDeletes,
			wantFailed: []string{"stale-a", "stale-c"},
		},
		{
			name:      "ignore policies already deleted",
			failures:  map[string]int{"stale-b": http.StatusNotFound},
			wantCalls: allDeletes,
		},
		{
			name:     "stop when the context is canceled",
			canceled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.canceled {
				cancel()
			}
			var calls []string
			es := esclient.NewMockClient(version.MustParse("7.11.0"), func(req *http.Request) *http.Response {
				require.Equal(t, http.MethodDelete, req.Method)
				name := strings.TrimPrefix(req.URL.Path, "/_autoscaling/policy/")
				calls = append(calls, name)
				if statusCode, exists := tt.failures[name]; exists {
					return esclient.NewMockResponse(statusCode, req, `{"error":{"reason":"simulated failure"}}`)
				}
				return esclient.NewMockResponse(http.StatusOK, req, `{"acknowledged":true}`)
			})

			err := deleteObsoletePolicies(ctx, logTest, specs, capacity, es)
			assert.Equal(t, tt.wantCalls, calls)
			switch {
			case tt.canceled:
				require.ErrorIs(t, err, context.Canceled)
			case len(tt.wantFailed) == 0:
				require.NoError(t, err)
			default:
				require.Error(t, err)
				for _, policyName := range tt.wantFailed {
					assert.ErrorContains(t, err, fmt.Sprintf("delete obsolete autoscaling policy %q", policyName))
				}
			}
		})
	}
}
