// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch/resources"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

var (
	fetchEvents = func(recorder *record.FakeRecorder) []string {
		close(recorder.Events)
		events := make([]string, 0)
		for event := range recorder.Events {
			events = append(events, event)
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

func TestReconcileLegacy(t *testing.T) {
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
				EsClient:       newFakeEsClient(t).withCapacity("annotation/frozen-tier"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "frozen-tier",
				isOnline:   true,
			},
			want:       defaultRequeue,
			wantErr:    false,
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
		},
		{
			name: "ML case where tier total memory was lower than node memory",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("annotation/ml"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "ml",
				isOnline:   true,
			},
			want:       defaultRequeue,
			wantErr:    false,
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
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
			want:       defaultRequeue, // we still expect the default requeue to be set even if there was an error
			wantErr:    true,           // Autoscaling API error should be returned.
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
		},
		{
			name: "Cluster is online, but answer from the API is empty, do not touch anything",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("annotation/empty-autoscaling-api-response"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "empty-autoscaling-api-response",
				isOnline:   true,
			},
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
			want:       defaultRequeue,
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
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
			want:       defaultRequeue,
		},
		{
			name: "Cluster is online, data tier has reached max. capacity",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("annotation/max-storage-reached"),
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
			wantEvents: []string{
				"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource.",
				"Warning HorizontalScalingLimitReached Can't provide total required storage 39059593954, max number of nodes is 8, requires 10 nodes",
			},
		},
		{
			name: "Cluster is online, data tier needs to be scaled up from 8 to 10 nodes",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("annotation/storage-scaled-horizontally"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "storage-scaled-horizontally",
				isOnline:   true,
			},
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
			want:       defaultRequeue,
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
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
		},
		{
			name: "CPU autoscaling",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("annotation/cpu-scaled-horizontally"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				esManifest: "cpu-scaled-horizontally",
				isOnline:   true,
			},
			want:       defaultRequeue,
			wantErr:    false,
			wantEvents: []string{"Warning Deprecated The use of the Elasticsearch autoscaling annotation is deprecated, please consider moving to the ElasticsearchAutoscaler custom resource."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient()
			if tt.args.esManifest != "" {
				// Load the current Elasticsearch resource from the sample files.
				es := esv1.Elasticsearch{}
				bytes, err := os.ReadFile(filepath.Join("testdata", "annotation", tt.args.esManifest, "elasticsearch.yml"))
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

			r := &ReconcileElasticsearchAutoscalingAnnotation{
				baseReconcileAutoscaling: baseReconcileAutoscaling{
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
				},
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
				t.Errorf("ReconcileElasticsearchAutoscaler.reconcileInternal() = %v, want %v", got, tt.want)
			}
			if tt.args.esManifest != "" {
				// Get back Elasticsearch from the API Server.
				updatedElasticsearch := esv1.Elasticsearch{}
				require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "testns", Name: "testes"}, &updatedElasticsearch))
				// Read expected the expected Elasticsearch resource.
				expectedElasticsearch := esv1.Elasticsearch{}
				bytes, err := os.ReadFile(filepath.Join("testdata", "annotation", tt.args.esManifest, "elasticsearch-expected.yml"))
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

func statusesEqual(t *testing.T, got, want v1alpha1.AutoscalingResource) {
	t.Helper()
	gotStatus, err := got.GetElasticsearchAutoscalerStatus()
	require.NoError(t, err)
	wantStatus, err := want.GetElasticsearchAutoscalerStatus()
	require.NoError(t, err)
	require.Equal(t, len(gotStatus.AutoscalingPolicyStatuses), len(wantStatus.AutoscalingPolicyStatuses))
	for _, wantPolicyStatus := range wantStatus.AutoscalingPolicyStatuses {
		gotPolicyStatus := getPolicyStatus(gotStatus.AutoscalingPolicyStatuses, wantPolicyStatus.Name)
		require.NotNilf(t, gotPolicyStatus, "Autoscaling policy '%s' not found", wantPolicyStatus.Name)
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

func getPolicyStatus(autoscalingPolicyStatuses []v1alpha1.AutoscalingPolicyStatus, name string) *v1alpha1.AutoscalingPolicyStatus {
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
	updatedPolicies                             map[string]v1alpha1.AutoscalingPolicy
}

func newFakeEsClient(t *testing.T) *fakeEsClient {
	t.Helper()
	return &fakeEsClient{
		t:                   t,
		autoscalingPolicies: esclient.AutoscalingCapacityResult{Policies: make(map[string]esclient.AutoscalingPolicyResult)},
		updatedPolicies:     make(map[string]v1alpha1.AutoscalingPolicy),
	}
}

func (f *fakeEsClient) withCapacity(testdata string) *fakeEsClient {
	policies := esclient.AutoscalingCapacityResult{}
	bytes, err := os.ReadFile("testdata/" + testdata + "/capacity.json")
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
func (f *fakeEsClient) CreateAutoscalingPolicy(_ context.Context, _ string, _ v1alpha1.AutoscalingPolicy) error {
	return nil
}
func (f *fakeEsClient) GetAutoscalingCapacity(_ context.Context) (esclient.AutoscalingCapacityResult, error) {
	return f.autoscalingPolicies, nil
}
func (f *fakeEsClient) UpdateMLNodesSettings(_ context.Context, _ int32, _ string) error {
	return nil
}
