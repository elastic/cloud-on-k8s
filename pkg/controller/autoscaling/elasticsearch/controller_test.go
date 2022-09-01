// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v2/pkg/about"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
		manifestsDir string
		isOnline     bool
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
				EsClient:       newFakeEsClient(t).withCapacity("custom_resource/frozen-tier"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				manifestsDir: "frozen-tier",
				isOnline:     true,
			},
			want:       defaultRequeue,
			wantErr:    false,
			wantEvents: []string{},
		},
		{
			name: "ML case where tier total memory was lower than node memory",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("custom_resource/ml"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				manifestsDir: "ml",
				isOnline:     true,
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
				manifestsDir: "min-nodes-increased-by-user",
				isOnline:     true, // Online, but an error will be raised when updating the autoscaling policies.
			},
			want:       defaultRequeue, // we still expect the default requeue to be set even if there was an error
			wantErr:    true,           // Autoscaling API error should be returned.
			wantEvents: []string{},
		},
		{
			name: "Cluster is online, but answer from the API is empty, do not touch anything",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("custom_resource/empty-autoscaling-api-response"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				manifestsDir: "empty-autoscaling-api-response",
				isOnline:     true,
			},
			wantEvents: []string{},
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
				manifestsDir: "cluster-creation",
				isOnline:     false,
			},
			wantEvents: []string{},
			want:       defaultRequeue,
		},
		{
			name: "Cluster is online, data tier has reached max. capacity",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("custom_resource/max-storage-reached"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				manifestsDir: "max-storage-reached",
				isOnline:     true,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 42 * time.Second,
			},
			wantEvents: []string{
				"Warning HorizontalScalingLimitReached Can't provide total required storage 39059593954, max number of nodes is 8, requires 10 nodes",
			},
		},
		{
			name: "Cluster is online, data tier needs to be scaled up from 8 to 10 nodes",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("custom_resource/storage-scaled-horizontally"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				manifestsDir: "storage-scaled-horizontally",
				isOnline:     true,
			},
			wantEvents: []string{},
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
				manifestsDir: "",
			},
			want: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			wantErr:    false,
			wantEvents: []string{},
		},
		{
			name: "CPU autoscaling",
			fields: fields{
				EsClient:       newFakeEsClient(t).withCapacity("custom_resource/cpu-scaled-horizontally"),
				recorder:       record.NewFakeRecorder(1000),
				licenseChecker: &license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			args: args{
				manifestsDir: "cpu-scaled-horizontally",
				isOnline:     true,
			},
			want:       defaultRequeue,
			wantErr:    false,
			wantEvents: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient()
			if tt.args.manifestsDir != "" {
				// Load the current Elasticsearch resource from the sample files.
				es := esv1.Elasticsearch{}
				bytes, err := os.ReadFile(filepath.Join("testdata", "custom_resource", tt.args.manifestsDir, "elasticsearch.yml"))
				require.NoError(t, err)
				if err := yaml.Unmarshal(bytes, &es); err != nil {
					t.Fatalf("yaml.Unmarshal error = %v, wantErr %v", err, tt.wantErr)
				}
				esa := v1alpha1.ElasticsearchAutoscaler{}
				bytes, err = os.ReadFile(filepath.Join("testdata", "custom_resource", tt.args.manifestsDir, "autoscaler.yml"))
				require.NoError(t, err)
				if err := yaml.Unmarshal(bytes, &esa); err != nil {
					t.Fatalf("yaml.Unmarshal error = %v, wantErr %v", err, tt.wantErr)
				}
				if tt.args.isOnline {
					k8sClient = k8s.NewFakeClient(es.DeepCopy(), esa.DeepCopy(), fakeService, fakeEndpoints)
				} else {
					k8sClient = k8s.NewFakeClient(es.DeepCopy(), esa.DeepCopy())
				}
			}

			r := &ReconcileElasticsearchAutoscaler{
				Watches: watches.NewDynamicWatches(),
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
					Name:      "test-autoscaler",
				}})
			if (err != nil) != tt.wantErr {
				t.Errorf("autoscaling.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileElasticsearchAutoscaler.reconcileInternal() = %v, want %v", got, tt.want)
			}
			if tt.args.manifestsDir != "" {
				// Get back Elasticsearch from the API Server.
				updatedElasticsearch := esv1.Elasticsearch{}
				require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "testns", Name: "testes"}, &updatedElasticsearch))

				// Read expected the expected Elasticsearch resource.
				expectedElasticsearch := esv1.Elasticsearch{}
				bytes, err := os.ReadFile(filepath.Join("testdata", "custom_resource", tt.args.manifestsDir, "elasticsearch-expected.yml"))
				require.NoError(t, err)
				require.NoError(t, yaml.Unmarshal(bytes, &expectedElasticsearch))

				// Get back ElasticsearchAutoscaler from the API Server.
				updatedElasticsearchAutoscaler := &v1alpha1.ElasticsearchAutoscaler{}
				require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "testns", Name: "test-autoscaler"}, updatedElasticsearchAutoscaler))

				// Read expected the expected ElasticsearchAutoscaler resource.
				expectedElasticsearchAutoscaler := &v1alpha1.ElasticsearchAutoscaler{}
				bytes, err = os.ReadFile(filepath.Join("testdata", "custom_resource", tt.args.manifestsDir, "autoscaler-expected.yml"))
				require.NoError(t, err)
				require.NoError(t, yaml.Unmarshal(bytes, expectedElasticsearchAutoscaler))
				assert.Equal(t, updatedElasticsearch.Spec, expectedElasticsearch.Spec, "Updated Elasticsearch spec. is not the expected one")

				// Compare the statuses.
				statusesEqual(t, updatedElasticsearchAutoscaler, expectedElasticsearchAutoscaler)

				// Check event raised
				gotEvents := fetchEvents(tt.fields.recorder)
				require.ElementsMatch(t, tt.wantEvents, gotEvents)
			}
		})
	}
}
