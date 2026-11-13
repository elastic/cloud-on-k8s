// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

//nolint:thelper
func TestReconcileKibana_Reconcile(t *testing.T) {
	sampleElasticsearch := esv1.Elasticsearch{
		Name:      "test-es",
		Namespace: "test",
		Spec: esv1.ElasticsearchSpec{
			Version: "7.17.0",
		},
	}
	sampleKibana := kibanav1.Kibana{
		Name:       "test-kibana",
		Namespace:  "test",
		Generation: 2,
		Spec: kibanav1.KibanaSpec{
			Version: "7.17.0",
			Count:   1,
		},
		Status: kibanav1.KibanaStatus{
			ObservedGeneration: 1,
		},
	}
	defaultRequest := reconcile.Request{
		Name:      "test-kibana",
		Namespace: "test",
	}
	type fields struct {
		Client k8s.Client
		// licenseChecker defaults to an enabled Enterprise license when nil.
		licenseChecker license.Checker
	}
	tests := []struct {
		name     string
		fields   fields
		request  reconcile.Request
		want     reconcile.Result
		wantErr  bool
		errorMsg string
		// wantRequeueAfter, when non-zero, is asserted against Result.RequeueAfter.
		wantRequeueAfter time.Duration
		// wantEvent, when non-empty, must be contained in one of the emitted events.
		wantEvent string
		validate  func(*testing.T, fields)
	}{
		{
			name: "unmanaged kibana instance does not increment observedGeneration",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleElasticsearch,
					withAnnotations(&sampleKibana, map[string]string{common.ManagedAnnotation: "false"}),
				),
			},
			request: defaultRequest,
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var kibana kibanav1.Kibana
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test-kibana"}, &kibana)
				require.NoError(t, err)
				require.Equal(t, kibanav1.KibanaStatus{
					Selector:           "",
					Count:              0,
					AvailableNodes:     0,
					Version:            "",
					Health:             commonv1.DeploymentHealth(""),
					ObservedGeneration: 1,
				}, kibana.Status)
			},
		},
		{
			name: "kibana instance with legacy finalizer has finalizer removed and increments observedGeneration",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleElasticsearch,
					withFinalizers(&sampleKibana, []string{"finalizer.elasticsearch.k8s.elastic.co/secure-settings-secret"}),
				),
			},
			request: defaultRequest,
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var kibana kibanav1.Kibana
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test-kibana"}, &kibana)
				require.NoError(t, err)
				require.Len(t, kibana.ObjectMeta.Finalizers, 0)
				require.Equal(t, kibanav1.KibanaStatus{
					Selector:           "common.k8s.elastic.co/type=kibana,kibana.k8s.elastic.co/name=test-kibana,kibana.k8s.elastic.co/role=primary",
					Count:              0,
					AvailableNodes:     0,
					Version:            "",
					Health:             commonv1.RedHealth,
					ObservedGeneration: 2,
					Pools: kibanav1.KibanaPoolsStatuses{
						Primary: commonv1.DeploymentStatus{
							Selector: "common.k8s.elastic.co/type=kibana,kibana.k8s.elastic.co/name=test-kibana,kibana.k8s.elastic.co/role=primary",
							Health:   commonv1.RedHealth,
						},
					},
				}, kibana.Status)
			},
		},
		{
			name: "kibana instance with validation issues returns error and increments observedGeneration",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleElasticsearch,
					withName(&sampleKibana, "superlongkibananamecausesvalidationissues"),
				),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "superlongkibananamecausesvalidationissues",
				},
			},
			want:     reconcile.Result{},
			wantErr:  true,
			errorMsg: `Kibana.kibana.k8s.elastic.co "superlongkibananamecausesvalidationissues" is invalid: metadata.name: Too long: may not be more than 36 bytes`,
			validate: func(t *testing.T, f fields) {
				var kibana kibanav1.Kibana
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "superlongkibananamecausesvalidationissues"}, &kibana)
				require.NoError(t, err)
				require.Equal(t, kibanav1.KibanaStatus{
					Selector:           "",
					Count:              0,
					AvailableNodes:     0,
					Version:            "",
					Health:             commonv1.DeploymentHealth(""),
					ObservedGeneration: 2,
				}, kibana.Status)
			},
		},
		{
			name: "kibana instance with validation issues attempts update of status which fails returns status update error from deferred function and does not increment observedGeneration",
			fields: fields{
				Client: newK8sFailingStatusUpdateClient(
					&sampleElasticsearch,
					withName(&sampleKibana, "superlongkibananamecausesvalidationissues"),
				),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "superlongkibananamecausesvalidationissues",
				},
			},
			want:     reconcile.Result{},
			wantErr:  true,
			errorMsg: `while updating status: internal error: Kibana.kibana.k8s.elastic.co "superlongkibananamecausesvalidationissues" is invalid: metadata.name: Too long: may not be more than 36 bytes`,
			validate: func(t *testing.T, f fields) {
				var kibana kibanav1.Kibana
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "superlongkibananamecausesvalidationissues"}, &kibana)
				require.NoError(t, err)
				require.Equal(t, kibanav1.KibanaStatus{
					Selector:           "",
					Count:              0,
					AvailableNodes:     0,
					Version:            "",
					Health:             commonv1.DeploymentHealth(""),
					ObservedGeneration: 1,
				}, kibana.Status)
			},
		},
		{
			name: "sample kibana instance returns no error and increments observedGeneration",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleElasticsearch,
					&sampleKibana,
				),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "test-kibana",
				},
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var kibana kibanav1.Kibana
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test-kibana"}, &kibana)
				require.NoError(t, err)
				require.Equal(t, kibanav1.KibanaStatus{
					Selector:           "common.k8s.elastic.co/type=kibana,kibana.k8s.elastic.co/name=test-kibana,kibana.k8s.elastic.co/role=primary",
					Count:              0,
					AvailableNodes:     0,
					Version:            "",
					Health:             commonv1.RedHealth,
					ObservedGeneration: 2,
					Pools: kibanav1.KibanaPoolsStatuses{
						Primary: commonv1.DeploymentStatus{
							Selector: "common.k8s.elastic.co/type=kibana,kibana.k8s.elastic.co/name=test-kibana,kibana.k8s.elastic.co/role=primary",
							Health:   commonv1.RedHealth,
						},
					},
				}, kibana.Status)
			},
		},
		{
			name: "sample kibana instance with es association with version that is not allowed returns no error and increments observedGeneration",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleElasticsearch,
					withAssociationConf(withESReference(&sampleKibana, commonv1.ObjectSelector{Name: "test-es", Namespace: "test"}), commonv1.AssociationConf{
						AuthSecretName: "test-es-elastic-user",
						AuthSecretKey:  "elastic",
						CASecretName:   "ca-secret",
						CACertProvided: false,
						URL:            "https://test-es:9200",
						// This will be considered an invalid version, as it's considered 'not reported yet'.
						Version: "",
					}),
					&corev1.Secret{
						Name:      "test-es-elastic-user",
						Namespace: "test",
						Data: map[string][]byte{
							"elastic": []byte("password"),
						},
					},
				),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "test-kibana",
				},
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var kibana kibanav1.Kibana
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test-kibana"}, &kibana)
				require.NoError(t, err)
				require.Equal(t, kibanav1.KibanaStatus{
					Selector:           "",
					Count:              0,
					AvailableNodes:     0,
					Version:            "",
					Health:             commonv1.DeploymentHealth(""),
					ObservedGeneration: 2,
				}, kibana.Status)
			},
		},
		{
			name: "sample kibana instance with es association with valid version returns no error increments observedGeneration but does not update Elasticsearch association",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleElasticsearch,
					withTLSDisabled(withAssociationConf(withESReference(&sampleKibana, commonv1.ObjectSelector{Name: "test-es", Namespace: "test"}), commonv1.AssociationConf{
						AuthSecretName: "test-es-elastic-user",
						AuthSecretKey:  "elastic",
						CASecretName:   "ca-secret",
						CACertProvided: false,
						URL:            "https://test-es:9200",
						Version:        "7.17.0",
					})),
					&corev1.Secret{
						Name:      "test-es-elastic-user",
						Namespace: "test",
						Data: map[string][]byte{
							"elastic": []byte("password"),
						},
					},
					&corev1.Secret{
						Name:      "ca-secret",
						Namespace: "test",
						Data: map[string][]byte{
							"ca.crt": []byte("fake data"),
						},
					},
				),
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test",
					Name:      "test-kibana",
				},
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var kibana kibanav1.Kibana
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test-kibana"}, &kibana)
				require.NoError(t, err)
				require.Equal(t, kibanav1.KibanaStatus{
					Selector:           "common.k8s.elastic.co/type=kibana,kibana.k8s.elastic.co/name=test-kibana,kibana.k8s.elastic.co/role=primary",
					Count:              0,
					AvailableNodes:     0,
					Version:            "",
					Health:             commonv1.RedHealth,
					ObservedGeneration: 2,
					Pools: kibanav1.KibanaPoolsStatuses{
						Primary: commonv1.DeploymentStatus{
							Selector: "common.k8s.elastic.co/type=kibana,kibana.k8s.elastic.co/name=test-kibana,kibana.k8s.elastic.co/role=primary",
							Health:   commonv1.RedHealth,
						},
					},
				}, kibana.Status)
			},
		},
		{
			name: "background tasks without an enterprise license and no existing background deployment skips reconciliation and requeues",
			fields: fields{
				Client:         k8s.NewFakeClient(&sampleElasticsearch, withBackgroundTasks(&sampleKibana)),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false},
			},
			request:          defaultRequest,
			want:             reconcile.Result{},
			wantRequeueAfter: common.DefaultRequeueInterval,
			wantEvent:        "ReconciliationError",
			validate: func(t *testing.T, f fields) {
				var deployments appsv1.DeploymentList
				require.NoError(t, f.Client.List(context.Background(), &deployments, client.InNamespace("test")))
				require.Empty(t, deployments.Items)
				var services corev1.ServiceList
				require.NoError(t, f.Client.List(context.Background(), &services, client.InNamespace("test")))
				require.Empty(t, services.Items)
				var kibana kibanav1.Kibana
				require.NoError(t, f.Client.Get(context.Background(), defaultRequest.NamespacedName, &kibana))
				require.Equal(t, int64(1), kibana.Status.ObservedGeneration, "generation must not be reported as observed when nothing was applied")
			},
		},
		{
			name: "background tasks without an enterprise license and an existing background deployment reconciles with a warning",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleElasticsearch,
					withBackgroundTasks(&sampleKibana),
					&appsv1.Deployment{
						Namespace: sampleKibana.Namespace,
						Name:      kibanav1.BackgroundTasksDeployment(sampleKibana.Name),
					},
				),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false},
			},
			request:   defaultRequest,
			want:      reconcile.Result{},
			wantEvent: "InvalidLicense",
			validate: func(t *testing.T, f fields) {
				var deployments appsv1.DeploymentList
				require.NoError(t, f.Client.List(context.Background(), &deployments, client.InNamespace("test")))
				require.Len(t, deployments.Items, 2)
				var kibana kibanav1.Kibana
				require.NoError(t, f.Client.Get(context.Background(), defaultRequest.NamespacedName, &kibana))
				require.NotNil(t, kibana.Status.Pools.BackgroundTasks)
			},
		},
		{
			name: "background tasks license check error fails closed",
			fields: fields{
				Client:         k8s.NewFakeClient(&sampleElasticsearch, withBackgroundTasks(&sampleKibana)),
				licenseChecker: license.MockLicenseChecker{Err: errors.New("license check failed")},
			},
			request:  defaultRequest,
			want:     reconcile.Result{},
			wantErr:  true,
			errorMsg: "license check failed",
			validate: func(t *testing.T, f fields) {
				var deployments appsv1.DeploymentList
				require.NoError(t, f.Client.List(context.Background(), &deployments, client.InNamespace("test")))
				require.Empty(t, deployments.Items)
			},
		},
		{
			name: "background tasks with an enterprise license is reconciled",
			fields: fields{
				Client:         k8s.NewFakeClient(&sampleElasticsearch, withBackgroundTasks(&sampleKibana)),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
			},
			request: defaultRequest,
			want:    reconcile.Result{},
			validate: func(t *testing.T, f fields) {
				var deployments appsv1.DeploymentList
				require.NoError(t, f.Client.List(context.Background(), &deployments, client.InNamespace("test")))
				require.Len(t, deployments.Items, 2)
			},
		},
		{
			name: "no background tasks without an enterprise license is not gated",
			fields: fields{
				Client:         k8s.NewFakeClient(&sampleElasticsearch, &sampleKibana),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false},
			},
			request: defaultRequest,
			want:    reconcile.Result{},
			validate: func(t *testing.T, f fields) {
				var deployments appsv1.DeploymentList
				require.NoError(t, f.Client.List(context.Background(), &deployments, client.InNamespace("test")))
				require.Len(t, deployments.Items, 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := tt.fields.licenseChecker
			if checker == nil {
				checker = license.MockLicenseChecker{EnterpriseEnabled: true}
			}
			recorder := toolsevents.NewFakeRecorder(100)
			r := &ReconcileKibana{
				Client:         tt.fields.Client,
				recorder:       recorder,
				dynamicWatches: watches.NewDynamicWatches(),
				params:         operator.Parameters{},
				licenseChecker: checker,
			}
			got, err := r.Reconcile(context.Background(), tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileKibana.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				require.EqualError(t, err, tt.errorMsg)
			}
			// RequeueAfter is ignored here, as certificate reconciler sets this to expiration of the generated certificates.
			if !cmp.Equal(got, tt.want, cmpopts.IgnoreFields(reconcile.Result{}, "RequeueAfter")) {
				t.Errorf("ReconcileKibana.Reconcile() = %v, want %v", got, tt.want)
			}
			if tt.wantRequeueAfter != 0 {
				require.Equal(t, tt.wantRequeueAfter, got.RequeueAfter)
			}
			if tt.wantEvent != "" {
				close(recorder.Events)
				var events []string
				for e := range recorder.Events {
					events = append(events, e)
				}
				require.Condition(t, func() bool {
					for _, e := range events {
						if strings.Contains(e, "Warning") && strings.Contains(e, tt.wantEvent) {
							return true
						}
					}
					return false
				}, "expected a warning event containing %q, got %v", tt.wantEvent, events)
			}
			tt.validate(t, tt.fields)
		})
	}
}

func withBackgroundTasks(kibana *kibanav1.Kibana) *kibanav1.Kibana {
	obj := kibana.DeepCopy()
	obj.Spec.Version = "8.16.0"
	obj.Spec.BackgroundTasks = &kibanav1.KibanaBackgroundTasks{}
	return obj
}

func withAnnotations(kibana *kibanav1.Kibana, annotations map[string]string) *kibanav1.Kibana {
	obj := kibana.DeepCopy()
	obj.ObjectMeta.Annotations = annotations
	return obj
}

func withFinalizers(kibana *kibanav1.Kibana, finalizers []string) *kibanav1.Kibana {
	obj := kibana.DeepCopy()
	obj.ObjectMeta.Finalizers = finalizers
	return obj
}

func withName(kibana *kibanav1.Kibana, name string) *kibanav1.Kibana {
	obj := kibana.DeepCopy()
	obj.ObjectMeta.Name = name
	return obj
}

func withESReference(kibana *kibanav1.Kibana, selector commonv1.ObjectSelector) *kibanav1.Kibana {
	obj := kibana.DeepCopy()
	obj.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{ObjectSelector: selector}
	return obj
}

func withAssociationConf(kibana *kibanav1.Kibana, conf commonv1.AssociationConf) *kibanav1.Kibana {
	obj := kibana.DeepCopy()
	association := kibanav1.KibanaEsAssociation{
		Kibana: obj,
	}
	association.SetAssociationConf(
		&conf,
	)
	b, _ := json.Marshal(&conf)
	association.SetAnnotations(map[string]string{
		association.AssociationConfAnnotationName(): string(b),
	})
	associated := association.Associated()
	return associated.(*kibanav1.Kibana) //nolint:forcetypeassert
}

func withTLSDisabled(kibana *kibanav1.Kibana) *kibanav1.Kibana {
	obj := kibana.DeepCopy()
	obj.Spec.HTTP.TLS = commonv1.TLSOptions{
		SelfSignedCertificate: &commonv1.SelfSignedCertificate{
			Disabled: true,
		},
	}
	return obj
}

type k8sFailingStatusUpdateClient struct {
	k8s.Client

	client       k8s.Client
	statusWriter client.StatusWriter
}

type k8sFailingStatusWriter struct {
	client.StatusWriter
}

func newK8sFailingStatusUpdateClient(initObjs ...client.Object) *k8sFailingStatusUpdateClient {
	return &k8sFailingStatusUpdateClient{
		client:       k8s.NewFakeClient(initObjs...),
		statusWriter: &k8sFailingStatusWriter{},
	}
}

func (k *k8sFailingStatusUpdateClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	return k.client.Get(ctx, key, obj)
}

func (k *k8sFailingStatusUpdateClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return errors.New("internal error")
}

func (k *k8sFailingStatusUpdateClient) Status() client.StatusWriter {
	return k.statusWriter
}

func (sw *k8sFailingStatusWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return errors.New("internal error")
}
