// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/test"
)

var (
	emptyList                   = `{"items":[]}`
	enrollmentKeySample         = `{"item":{"id":"some-token-id","active":true,"api_key_id":"2y2otIEB7e2EvmgdFYCC","api_key":"some-token","name":"fe2c49b4-a94a-41ba-b6ed-a64c536874e9","policy_id":"a-policy-id","created_at":"2022-06-30T12:48:44.378Z"}}`
	enrollmentKeyListSample     = `{"items":[{"id":"some-token-id","active":true,"api_key_id":"2y2otIEB7e2EvmgdFYCC","api_key":"some-token","name":"fe2c49b4-a94a-41ba-b6ed-a64c536874e9","policy_id":"a-policy-id","created_at":"2022-06-30T12:48:44.378Z"}], "total":1,"page":1,"perPage":20}`
	fleetServerKeySample        = `{"item":{"id":"some-token-id","active":true,"api_key_id":"2y2otIEB7e2EvmgdFYCC","api_key":"fleet-token","name":"fe2c49b4-a94a-41ba-b6ed-a64c536874e9","policy_id":"fleet-policy-id","created_at":"2022-06-30T12:48:44.378Z"}}`
	inactiveEnrollmentKeySample = `{"item":{"id":"some-token-id","active":false,"api_key_id":"2y2otIEB7e2EvmgdFYCC","api_key":"some-token","name":"fe2c49b4-a94a-41ba-b6ed-a64c536874e9","policy_id":"a-policy-id","created_at":"2022-06-30T12:48:44.378Z"}}`
	agentPoliciesSample         = `{"items":[{"id":"fleet-policy-id","namespace":"default","monitoring_enabled":["logs","metrics"],"name":"Default Fleet Server policy","description":"Default Fleet Server agent policy created by Kibana","is_default":false,"is_default_fleet_server":true,"is_preconfigured":true,"status":"active","is_managed":false,"revision":1,"updated_at":"2022-06-30T12:48:35.349Z","updated_by":"system","package_policies":["dcc6e5b3-ea49-4b96-ae39-a1a3b74d849b"],"agents":1},{"id":"f217f7e0-f872-11ec-8bc1-17034ca5bd9f","namespace":"default","monitoring_enabled":["logs","metrics"],"name":"Default policy","description":"Default agent policy created by Kibana","is_default":true,"is_preconfigured":true,"status":"active","is_managed":false,"revision":1,"updated_at":"2022-06-30T12:48:33.323Z","updated_by":"system","package_policies":["8a7a3e75-47fb-4205-8c1c-db69a2c70458"],"agents":3}],"total":2,"page":1,"perPage":20}`
)

func Test_reconcileEnrollmentToken(t *testing.T) {
	asObject := func(raw string) EnrollmentAPIKey {
		var r EnrollmentAPIKeyResult
		require.NoError(t, json.Unmarshal([]byte(raw), &r))
		return r.Item
	}

	type args struct {
		agent  v1alpha1.Agent
		client *k8s.Client
		api    *mockFleetAPI
	}
	tests := []struct {
		name       string
		args       args
		want       EnrollmentAPIKey
		wantErr    bool
		wantEvents []string
	}{
		{
			name: "Agent annotated and fixed policy",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
						FleetTokenAnnotation: "some-token-id",
					}},
					Spec: v1alpha1.AgentSpec{
						PolicyID: "a-policy-id",
					},
				},
				api: mockFleetResponses(map[request]response{
					{"GET", "/api/fleet/enrollment_api_keys/some-token-id"}: {code: 200, body: enrollmentKeySample},
				}),
			},
			want:       asObject(enrollmentKeySample),
			wantEvents: nil, // PolicyID is provided.
			wantErr:    false,
		},
		{
			name: "Agent annotated but default policy",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "ns",
						Annotations: map[string]string{
							FleetTokenAnnotation: "some-token-id",
						}},
				},
				api: mockFleetResponses(map[request]response{
					// get all policies
					{"GET", "/api/fleet/agent_policies"}: {code: 200, body: agentPoliciesSample},
					// check annotated api key
					{"GET", "/api/fleet/enrollment_api_keys/some-token-id"}: {code: 200, body: enrollmentKeySample},
					// try to find existing key but there is none
					{"GET", "/api/fleet/enrollment_api_keys"}: {code: 200, body: emptyList},
					// new token because existing key not valid for policy
					{"POST", "/api/fleet/enrollment_api_keys"}: {code: 200, body: enrollmentKeySample},
				}),
			},
			want:       asObject(enrollmentKeySample),
			wantEvents: []string{"Warning Validation spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"},
			wantErr:    false,
		},
		{
			name: "Agent annotated but token does not exist",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "ns",
						Annotations: map[string]string{
							FleetTokenAnnotation: "invalid-token-id",
						}},
					Spec: v1alpha1.AgentSpec{
						PolicyID: "a-policy-id",
					},
				},
				api: mockFleetResponses(map[request]response{
					{"GET", "/api/fleet/enrollment_api_keys/invalid-token-id"}: {code: 404},
					{"GET", "/api/fleet/enrollment_api_keys"}:                  {code: 200, body: enrollmentKeyListSample},
				}),
			},
			want:       asObject(enrollmentKeySample),
			wantEvents: nil, // PolicyID is provided.
			wantErr:    false,
		},
		{
			name: "Agent annotated but token is invalid",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "ns",
						Annotations: map[string]string{
							FleetTokenAnnotation: "invalid-token-id",
						}},
					Spec: v1alpha1.AgentSpec{},
				},
				api: mockFleetResponses(map[request]response{
					{"GET", "/api/fleet/agent_policies"}:                       {code: 200, body: agentPoliciesSample},
					{"GET", "/api/fleet/enrollment_api_keys/invalid-token-id"}: {code: 200, body: inactiveEnrollmentKeySample},
					{"GET", "/api/fleet/enrollment_api_keys"}:                  {code: 200, body: emptyList},
					{"POST", "/api/fleet/enrollment_api_keys"}:                 {code: 200, body: enrollmentKeySample},
				}),
			},
			want:       asObject(enrollmentKeySample),
			wantEvents: []string{"Warning Validation spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"},
			wantErr:    false,
		},
		{
			name: "Agent not annotated yet",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "ns",
					},
				},
				api: mockFleetResponses(map[request]response{
					{"POST", "/api/fleet/setup"}:               {code: 200},
					{"GET", "/api/fleet/agent_policies"}:       {code: 200, body: agentPoliciesSample},
					{"GET", "/api/fleet/enrollment_api_keys"}:  {code: 200, body: emptyList},
					{"POST", "/api/fleet/enrollment_api_keys"}: {code: 200, body: enrollmentKeySample},
				}),
			},
			want:       asObject(enrollmentKeySample),
			wantEvents: []string{"Warning Validation spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"},
			wantErr:    false,
		},
		{
			name: "Error in Fleet API",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "ns",
					},
				},
				api: mockFleetResponses(map[request]response{
					{"POST", "/api/fleet/setup"}:              {code: 200},
					{"GET", "/api/fleet/agent_policies"}:      {code: 200, body: agentPoliciesSample},
					{"GET", "/api/fleet/enrollment_api_keys"}: {code: 500}, // could also be a timeout etc
				}),
			},
			want:       EnrollmentAPIKey{},
			wantEvents: []string{"Warning Validation spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"},
			wantErr:    true,
		},
		{
			name: "Fleet Server policy and key",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "ns",
						Annotations: map[string]string{
							FleetTokenAnnotation: "some-token-id",
						}},
					Spec: v1alpha1.AgentSpec{
						FleetServerEnabled: true,
					}},
				api: mockFleetResponses(map[request]response{
					// get all policies
					{"GET", "/api/fleet/agent_policies"}: {code: 200, body: agentPoliciesSample},
					// check annotated api key, should be valid
					{"GET", "/api/fleet/enrollment_api_keys/some-token-id"}: {code: 200, body: fleetServerKeySample},
				}),
			},
			want:       asObject(fleetServerKeySample),
			wantEvents: []string{"Warning Validation spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"},
			wantErr:    false,
		},
		{
			name: "Error in Kubernetes API",
			args: args{
				agent: v1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "ns",
					},
					Spec: v1alpha1.AgentSpec{
						FleetServerEnabled: true,
					}},
				client: func() *k8s.Client {
					client := k8s.NewFailingClient(errors.New("boom"))
					return &client
				}(),
				api: mockFleetResponses(map[request]response{
					{"POST", "/api/fleet/setup"}: {code: 200},
					// get all policies
					{"GET", "/api/fleet/agent_policies"}: {code: 200, body: agentPoliciesSample},
					// no token to reuse create a new one
					{"GET", "/api/fleet/enrollment_api_keys"}:  {code: 200, body: emptyList},
					{"POST", "/api/fleet/enrollment_api_keys"}: {code: 200, body: fleetServerKeySample},
				}),
			},
			want:       EnrollmentAPIKey{},
			wantEvents: []string{"Warning Validation spec.PolicyID is empty, spec.PolicyID will become mandatory in a future release"},
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client = k8s.NewFakeClient(&tt.args.agent)
			if tt.args.client != nil {
				client = *tt.args.client
			}
			fakeRecorder := record.NewFakeRecorder(10)
			params := Params{
				Context:       context.Background(),
				Client:        client,
				EventRecorder: fakeRecorder,
				Agent:         tt.args.agent,
			}
			got, err := reconcileEnrollmentToken(params, tt.args.api.fleetAPI)
			require.Empty(t, tt.args.api.missingRequests())
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileEnrollmentToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("reconcileEnrollmentToken() got = %v, want %v", got, tt.want)
			}
			gotEvents := test.ReadAtMostEvents(t, len(tt.wantEvents), fakeRecorder)
			assert.Equal(t, tt.wantEvents, gotEvents)
		})
	}
}

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

type request struct {
	method string
	path   string
}

type response struct {
	code int
	body string
}

type mockFleetAPI struct {
	fleetAPI
	requests map[request]response
	callLog  map[request]int
}

func (m *mockFleetAPI) missingRequests() []request {
	var missing []request
	for req := range m.requests {
		if _, ok := m.callLog[req]; !ok {
			missing = append(missing, req)
		}
	}
	return missing
}

func mockFleetResponses(rs map[request]response) *mockFleetAPI {
	callLog := map[request]int{}
	fn := func(req *http.Request) *http.Response {
		r := request{method: req.Method, path: req.URL.Path}
		response, exists := rs[r]
		if exists {
			callLog[r]++
			return &http.Response{
				StatusCode: response.code,
				Body:       io.NopCloser(strings.NewReader(response.body)),
				Request:    req,
			}
		}
		panic(fmt.Sprintf("unexpected request %+v", r))
	}
	return &mockFleetAPI{
		fleetAPI: fleetAPI{
			client: &http.Client{
				Transport: RoundTripFunc(fn),
			},
			log: ulog.Log,
		},
		callLog:  callLog,
		requests: rs,
	}
}
