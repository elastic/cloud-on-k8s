// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch/status"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

var (
	fetchEvents = func(recorder *record.FakeRecorder) []string {
		events := make([]string, 0)
		select {
		case event := <-recorder.Events:
			events = append(events, event)
		default:
			break
		}
		return events
	}

	fakeService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testns",
			Name:      services.InternalServiceName("testes"),
		},
	}
	fakeEndpoints = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "testns",
			Name:      services.InternalServiceName("testes"),
		},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{
				IP: "10.0.0.2",
			}},
			Ports: []corev1.EndpointPort{},
		}},
	}
)

func TestReconcile(t *testing.T) {
	defaultRequeue := reconcile.Result{
		Requeue:      true,
		RequeueAfter: 60 * time.Second,
	}
	type fields struct {
		EsClient       *fakeEsClient
		recorder       *record.FakeRecorder
		licenseChecker license.Checker
	}
	type args struct {
		esManifest string
		isOnline   bool
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		want       reconcile.Result
		wantEvents []string
		wantErr    bool
	}{
		{
			name: "Frozen decider only returns capacity at the tier level",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("frozen-tier"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "frozen-tier",
				isOnline:   true,
			},
			want:       defaultRequeue,
			wantErr:    false,
			wantEvents: []string{},
		},
		{
			name: "ML case where tier total memory was lower than node memory",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("ml"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "ml",
				isOnline:   true,
			},
			want:       defaultRequeue,
			wantErr:    false,
			wantEvents: []string{},
		},
		{
			name: "Simulate an error while updating the autoscaling policies, we still want to respect min nodes count set by user",
			fields: fields{
				EsClient:       newFakeEsClient(t).withErrorOnDeleteAutoscalingAutoscalingPolicies(),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "min-nodes-increased-by-user",
				isOnline:   true, // Online, but an error will be raised when updating the autoscaling policies.
			},
			want:       reconcile.Result{},
			wantErr:    true, // Autoscaling API error should be returned.
			wantEvents: []string{},
		},
		{
			name: "Cluster is online, but answer from the API is empty, do not touch anything",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("empty-autoscaling-api-response"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "empty-autoscaling-api-response",
				isOnline:   true,
			},
			want: defaultRequeue,
		},
		{
			name: "Cluster has just been created, initialize resources",
			fields: fields{
				EsClient:       newFakeEsClient(t),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "cluster-creation",
				isOnline:   false,
			},
			want: defaultRequeue,
		},
		{
			name: "Cluster is online, data tier has reached max. capacity",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("max-storage-reached"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "max-storage-reached",
				isOnline:   true,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 42 * time.Second,
			},
			wantEvents: []string{"Warning HorizontalScalingLimitReached Can't provide total required storage 39059593954, max number of nodes is 8, requires 10 nodes"},
		},
		{
			name: "Cluster is online, data tier needs to be scaled up from 8 to 10 nodes",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("storage-scaled-horizontally"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "storage-scaled-horizontally",
				isOnline:   true,
			},
			want: defaultRequeue,
		},
		{
			name: "Cluster does not exit",
			fields: fields{
				EsClient:       newFakeEsClient(t),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "",
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			wantErr:    false,
			wantEvents: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient()
			if tt.args.esManifest != "" {
				// Load the current Elasticsearch resource from the sample files.
				es := esv1.Elasticsearch{}
				bytes, err := ioutil.ReadFile(filepath.Join("testdata", tt.args.esManifest, "elasticsearch.yml"))
				require.NoError(t, err)
				if err := yaml.Unmarshal(bytes, &es); err != nil {
					t.Fatalf("yaml.Unmarshal error = %v, wantErr %v", err, tt.wantErr)
				}
				if tt.args.isOnline {
					k8sClient = k8s.NewFakeClient(es.DeepCopy(), fakeService, fakeEndpoints)
				} else {
					k8sClient = k8s.NewFakeClient(es.DeepCopy())
				}
			}

			r := &ReconcileElasticsearch{
				Client:           k8sClient,
				esClientProvider: tt.fields.EsClient.newFakeElasticsearchClient,
				Parameters: operator.Parameters{
					OperatorInfo: about.OperatorInfo{
						BuildInfo: about.BuildInfo{
							Version: "1.5.0",
						},
					},
				},
				recorder:       tt.fields.recorder,
				licenseChecker: tt.fields.licenseChecker,
			}
			got, err := r.Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: "testns",
					Name:      "testes", // All the samples must have this name
				}})
			if (err != nil) != tt.wantErr {
				t.Errorf("autoscaling.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileElasticsearch.reconcileInternal() = %v, want %v", got, tt.want)
			}
			if tt.args.esManifest != "" {
				// Get back Elasticsearch from the API Server.
				updatedElasticsearch := esv1.Elasticsearch{}
				require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "testns", Name: "testes"}, &updatedElasticsearch))
				// Read expected the expected Elasticsearch resource.
				expectedElasticsearch := esv1.Elasticsearch{}
				bytes, err := ioutil.ReadFile(filepath.Join("testdata", tt.args.esManifest, "elasticsearch-expected.yml"))
				require.NoError(t, err)
				require.NoError(t, yaml.Unmarshal(bytes, &expectedElasticsearch))
				assert.Equal(t, updatedElasticsearch.Spec, expectedElasticsearch.Spec, "Updated Elasticsearch spec. is not the expected one")
				// Check that the autoscaling spec is still the expected one.
				assert.Equal(
					t,
					updatedElasticsearch.Annotations[esv1.ElasticsearchAutoscalingSpecAnnotationName],
					expectedElasticsearch.Annotations[esv1.ElasticsearchAutoscalingSpecAnnotationName],
					"Autoscaling specification is not the expected one",
				)
				// Compare the statuses.
				statusesEqual(t, updatedElasticsearch, expectedElasticsearch)
				// Check event raised
				gotEvents := fetchEvents(tt.fields.recorder)
				require.ElementsMatch(t, tt.wantEvents, gotEvents)
			}
		})
	}
}

func statusesEqual(t *testing.T, got, want esv1.Elasticsearch) {
	t.Helper()
	gotStatus, err := status.From(got)
	require.NoError(t, err)
	wantStatus, err := status.From(want)
	require.NoError(t, err)
	require.Equal(t, len(gotStatus.AutoscalingPolicyStatuses), len(wantStatus.AutoscalingPolicyStatuses))
	for _, wantPolicyStatus := range wantStatus.AutoscalingPolicyStatuses {
		gotPolicyStatus := getPolicyStatus(gotStatus.AutoscalingPolicyStatuses, wantPolicyStatus.Name)
		require.NotNil(t, gotPolicyStatus, "Autoscaling policy not found")
		require.ElementsMatch(t, gotPolicyStatus.NodeSetNodeCount, wantPolicyStatus.NodeSetNodeCount)
		require.ElementsMatch(t, gotPolicyStatus.PolicyStates, wantPolicyStatus.PolicyStates)
		for resource := range wantPolicyStatus.ResourcesSpecification.Requests {
			require.True(
				t,
				resources.ResourceEqual(resource, wantPolicyStatus.ResourcesSpecification.Requests, gotPolicyStatus.ResourcesSpecification.Requests),
				"unexpected resource requests for policy %s, expected %v, got %v", gotPolicyStatus.Name, wantPolicyStatus.ResourcesSpecification.Requests, gotPolicyStatus.ResourcesSpecification.Requests)
		}
		for resource := range wantPolicyStatus.ResourcesSpecification.Limits {
			require.True(
				t,
				resources.ResourceEqual(resource, wantPolicyStatus.ResourcesSpecification.Limits, gotPolicyStatus.ResourcesSpecification.Limits),
				"unexpected resource limits for policy %s, expected %v, got %v", gotPolicyStatus.Name, wantPolicyStatus.ResourcesSpecification.Limits, gotPolicyStatus.ResourcesSpecification.Limits)
		}
	}
}

func getPolicyStatus(autoscalingPolicyStatuses []status.AutoscalingPolicyStatus, name string) *status.AutoscalingPolicyStatus {
	for _, policyStatus := range autoscalingPolicyStatuses {
		if policyStatus.Name == name {
			return &policyStatus
		}
	}
	return nil
}

// - Fake Elasticsearch Autoscaling Client

type fakeEsClient struct {
	t *testing.T
	esclient.Client

	autoscalingPolicies                         esclient.AutoscalingCapacityResult
	policiesCleaned                             bool
	errorOnDeleteAutoscalingAutoscalingPolicies bool
	updatedPolicies                             map[string]esv1.AutoscalingPolicy
}

func newFakeEsClient(t *testing.T) *fakeEsClient {
	t.Helper()
	return &fakeEsClient{
		t:                   t,
		autoscalingPolicies: esclient.AutoscalingCapacityResult{Policies: make(map[string]esclient.AutoscalingPolicyResult)},
		updatedPolicies:     make(map[string]esv1.AutoscalingPolicy),
	}
}

func (f *fakeEsClient) withCapacity(testdata string) *fakeEsClient {
	policies := esclient.AutoscalingCapacityResult{}
	bytes, err := ioutil.ReadFile("testdata/" + testdata + "/capacity.json")
	if err != nil {
		f.t.Fatalf("Error while reading autoscaling capacity content: %v", err)
	}
	if err := json.Unmarshal(bytes, &policies); err != nil {
		f.t.Fatalf("Error while parsing autoscaling capacity content: %v", err)
	}
	f.autoscalingPolicies = policies
	return f
}

func (f *fakeEsClient) withErrorOnDeleteAutoscalingAutoscalingPolicies() *fakeEsClient {
	f.errorOnDeleteAutoscalingAutoscalingPolicies = true
	return f
}

func (f *fakeEsClient) newFakeElasticsearchClient(_ context.Context, _ k8s.Client, _ net.Dialer, _ esv1.Elasticsearch) (esclient.Client, error) {
	return f, nil
}

func (f *fakeEsClient) DeleteAutoscalingPolicies(_ context.Context) error {
	f.policiesCleaned = true
	if f.errorOnDeleteAutoscalingAutoscalingPolicies {
		return fmt.Errorf("simulated error while calling DeleteAutoscalingAutoscalingPolicies")
	}
	return nil
}
func (f *fakeEsClient) CreateAutoscalingPolicy(_ context.Context, policyName string, autoscalingPolicy esv1.AutoscalingPolicy) error {
	return nil
}
func (f *fakeEsClient) GetAutoscalingCapacity(_ context.Context) (esclient.AutoscalingCapacityResult, error) {
	return f.autoscalingPolicies, nil
}
func (f *fakeEsClient) UpdateMLNodesSettings(_ context.Context, maxLazyMLNodes int32, maxMemory string) error {
	return nil
}
