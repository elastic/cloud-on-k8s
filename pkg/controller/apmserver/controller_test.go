// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestReconcileApmServer_doReconcile(t *testing.T) {
	type fields struct {
		resources      []runtime.Object
		recorder       record.EventRecorder
		dynamicWatches watches.DynamicWatches
		Parameters     operator.Parameters
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name        string
		as          apmv1.ApmServer
		fields      fields
		args        args
		wantRequeue bool
		wantErr     bool
	}{
		{
			name: "If no error ensure a requeue is scheduled for CA",
			as: apmv1.ApmServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmserver",
					Namespace: "default",
				},
				Spec: apmv1.ApmServerSpec{
					Version: "7.6.1",
				},
			},
			fields: fields{
				resources:      []runtime.Object{},
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters: operator.Parameters{
					CACertRotation: certificates.RotationParams{
						Validity:     certificates.DefaultCertValidity,
						RotateBefore: certificates.DefaultRotateBefore,
					},
				},
			},
			args: args{
				request: reconcile.Request{},
			},
			wantRequeue: false,
		},
		{
			name: "Validation failure",
			as: apmv1.ApmServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "apmserver",
					Namespace: "default",
				},
				Spec: apmv1.ApmServerSpec{
					Version: "7.x.1",
				},
			},
			fields: fields{
				resources:      []runtime.Object{},
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			},
			args: args{
				request: reconcile.Request{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileApmServer{
				Client:         k8s.NewFakeClient(&tt.as),
				recorder:       tt.fields.recorder,
				dynamicWatches: tt.fields.dynamicWatches,
				Parameters:     tt.fields.Parameters,
			}
			got, err := r.doReconcile(context.Background(), tt.args.request, tt.as.DeepCopy())
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileApmServer.doReconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, got.Requeue, tt.wantRequeue)
			if tt.wantRequeue {
				require.True(t, got.RequeueAfter > 0)
			}
		})
	}
}

func Test_reconcileApmServerToken(t *testing.T) {
	apm := &apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "apm",
		},
	}
	tests := []struct {
		name       string
		c          k8s.Client
		reuseToken []byte
	}{
		{
			name: "no secret exists: create one",
			c:    k8s.NewFakeClient(),
		},
		{
			name: "reuse token if it already exists",
			c: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      SecretToken(apm.Name),
				},
				Data: map[string][]byte{
					SecretTokenKey: []byte("existing"),
				},
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reconcileApmServerToken(tt.c, apm)
			require.NoError(t, err)
			require.NotEmpty(t, got.Data[SecretTokenKey])
			if tt.reuseToken != nil {
				require.Equal(t, tt.reuseToken, got.Data[SecretTokenKey])
			}
		})
	}
}

func TestNewService(t *testing.T) {
	testCases := []struct {
		name     string
		httpConf commonv1.HTTPConfig
		wantSvc  func() corev1.Service
	}{
		{
			name: "no TLS",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						Disabled: true,
					},
				},
			},
			wantSvc: mkService,
		},
		{
			name: "self-signed certificate",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
							{
								DNS: "apm-test.local",
							},
						},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
		{
			name: "user-provided certificate",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					Certificate: commonv1.SecretRef{
						SecretName: "my-cert",
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			apm := mkAPMServer(tc.httpConf)
			haveSvc := NewService(apm)
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func mkService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-test-apm-http",
			Namespace: "test",
			Labels: map[string]string{
				ApmServerNameLabelName: "apm-test",
				common.TypeLabelName:   Type,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     HTTPPort,
				},
			},
			Selector: map[string]string{
				ApmServerNameLabelName: "apm-test",
				common.TypeLabelName:   Type,
			},
		},
	}
}

func mkAPMServer(httpConf commonv1.HTTPConfig) apmv1.ApmServer {
	return apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-test",
			Namespace: "test",
		},
		Spec: apmv1.ApmServerSpec{
			HTTP: httpConf,
		},
	}
}

func TestReconcileApmServer_Reconcile(t *testing.T) {
	sampleAPMObject := apmv1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "test",
			Name:       "test",
			Generation: 2,
		},
		Spec: apmv1.ApmServerSpec{
			Version: "7.0.1",
			Count:   1,
		},
		Status: apmv1.ApmServerStatus{
			ObservedGeneration: 1,
		},
	}
	defaultRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "test",
		},
	}
	type fields struct {
		Client         k8s.Client
		recorder       record.EventRecorder
		dynamicWatches watches.DynamicWatches
		Parameters     operator.Parameters
	}
	type args struct {
		ctx     context.Context
		request reconcile.Request
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		want     reconcile.Result
		wantErr  bool
		validate func(*testing.T, fields)
	}{
		{
			name: "unmanaged apm server does not increment observedGeneration",
			fields: fields{
				Client: k8s.NewFakeClient(
					withAnnotations(sampleAPMObject, map[string]string{common.ManagedAnnotation: "false"}),
				),
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			},
			args: args{
				ctx:     context.Background(),
				request: defaultRequest,
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var apm apmv1.ApmServer
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test"}, &apm)
				require.NoError(t, err)
				require.Equal(t, int64(1), apm.Status.ObservedGeneration)
			},
		},
		{
			name: "Legacy finalizer on apm server gets removed, and updates observedGeneration",
			fields: fields{
				Client: k8s.NewFakeClient(
					withFinalizers(sampleAPMObject, []string{"finalizer.elasticsearch.k8s.elastic.co/secure-settings-secret"}),
				),
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			},
			args: args{
				ctx:     context.Background(),
				request: defaultRequest,
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var apm apmv1.ApmServer
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test"}, &apm)
				require.NoError(t, err)
				require.Len(t, apm.ObjectMeta.Finalizers, 0)
				require.Equal(t, int64(2), apm.Status.ObservedGeneration)
			},
		},
		{
			// This could be a potential existing issue.  Do we want observedGeneration to be updated here?
			name: "With Elasticsearch association not ready, observedGeneration is not updated",
			fields: fields{
				Client: k8s.NewFakeClient(
					withESReference(sampleAPMObject, commonv1.ObjectSelector{Name: "testes"}),
				),
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			},
			args: args{
				ctx:     context.Background(),
				request: defaultRequest,
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var apm apmv1.ApmServer
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test"}, &apm)
				require.NoError(t, err)
				require.Equal(t, int64(1), apm.Status.ObservedGeneration)
			},
		},
		{
			name: "With Elasticsearch association ready, but apm version not allowed with es version, observedGeneration is updated",
			fields: fields{
				Client: k8s.NewFakeClient(
					&esv1.Elasticsearch{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "testes",
							Namespace: "test",
						},
						Spec: esv1.ElasticsearchSpec{
							Version: "7.16.2",
						},
					},
					withAssociationConf(*(withESReference(sampleAPMObject, commonv1.ObjectSelector{Name: "testes", Namespace: "test"})), commonv1.AssociationConf{
						AuthSecretName: "testes-es-elastic-user",
						AuthSecretKey:  "elastic",
						CASecretName:   "ca-secret",
						CACertProvided: false,
						URL:            "https://es:9200",
						// This will be considered an invalid version, as it's considered 'not reported yet'.
						Version: "",
					}),
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "testes-es-elastic-user",
							Namespace: "test",
						},
						Data: map[string][]byte{
							"elastic": []byte("password"),
						},
					},
				),
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			},
			args: args{
				ctx:     context.Background(),
				request: defaultRequest,
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var apm apmv1.ApmServer
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test"}, &apm)
				require.NoError(t, err)
				require.Equal(t, int64(2), apm.Status.ObservedGeneration)
			},
		},
		{
			name: "With validation issues, observedGeneration is updated",
			fields: fields{
				Client: k8s.NewFakeClient(
					withName(sampleAPMObject, "superlongapmservernamecausesvalidationissues"),
				),
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			},
			args: args{
				ctx: context.Background(),
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "superlongapmservernamecausesvalidationissues",
						Namespace: "test",
					},
				},
			},
			want:    reconcile.Result{},
			wantErr: true,
			validate: func(t *testing.T, f fields) {
				var apm apmv1.ApmServer
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "superlongapmservernamecausesvalidationissues"}, &apm)
				require.NoError(t, err)
				require.Equal(t, int64(2), apm.Status.ObservedGeneration)
			},
		},
		{
			name: "Reconcile of standard apm object updates observedGeneration, and creates deployment",
			fields: fields{
				Client: k8s.NewFakeClient(
					&sampleAPMObject,
				),
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			},
			args: args{
				ctx:     context.Background(),
				request: defaultRequest,
			},
			want:    reconcile.Result{},
			wantErr: false,
			validate: func(t *testing.T, f fields) {
				var apm apmv1.ApmServer
				err := f.Client.Get(context.Background(), types.NamespacedName{Namespace: "test", Name: "test"}, &apm)
				require.NoError(t, err)
				require.Len(t, apm.ObjectMeta.Finalizers, 0)
				require.Equal(t, int64(2), apm.Status.ObservedGeneration)
				var deploymentList appsv1.DeploymentList
				err = f.Client.List(context.Background(), &deploymentList)
				require.NoError(t, err)
				require.Len(t, deploymentList.Items, 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileApmServer{
				Client:         tt.fields.Client,
				recorder:       tt.fields.recorder,
				dynamicWatches: tt.fields.dynamicWatches,
				Parameters:     tt.fields.Parameters,
			}
			var apm apmv1.ApmServer
			getErr := association.FetchWithAssociations(context.Background(), r.Client, tt.args.request, &apm)
			require.NoError(t, getErr)
			got, err := r.Reconcile(tt.args.ctx, tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileApmServer.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// RequeueAfter is ignored here, as certificate reconciler sets this to expiration of the generated certificates.
			if !cmp.Equal(got, tt.want, cmpopts.IgnoreFields(reconcile.Result{}, "RequeueAfter")) {
				t.Errorf("ReconcileApmServer.Reconcile() = %v, want %v", got, tt.want)
			}
			tt.validate(t, tt.fields)
		})
	}
}

func withAnnotations(apm apmv1.ApmServer, annotations map[string]string) *apmv1.ApmServer {
	obj := apm.DeepCopy()
	obj.ObjectMeta.Annotations = annotations
	return obj
}

func withFinalizers(apm apmv1.ApmServer, finalizers []string) *apmv1.ApmServer {
	obj := apm.DeepCopy()
	obj.ObjectMeta.Finalizers = finalizers
	return obj
}

func withESReference(apm apmv1.ApmServer, selector commonv1.ObjectSelector) *apmv1.ApmServer {
	obj := apm.DeepCopy()
	obj.Spec.ElasticsearchRef = selector
	return obj
}

func withAssociationConf(apm apmv1.ApmServer, conf commonv1.AssociationConf) *apmv1.ApmServer {
	obj := apm.DeepCopy()
	association := apmv1.NewApmEsAssociation(obj)
	association.SetAssociationConf(
		&commonv1.AssociationConf{
			AuthSecretName: "auth-secret",
			AuthSecretKey:  "elastic",
			CASecretName:   "ca-secret",
			CACertProvided: true,
			URL:            "https://es.svc:9200",
		},
	)
	association.SetAnnotations(map[string]string{
		association.AssociationConfAnnotationName(): `{"authSecretName":"auth-secret", "authSecretKey":"elastic", "caSecretName": "ca-secret", "url":"https://es.svc:9200"}`,
	})
	associated := association.Associated()
	return associated.(*apmv1.ApmServer)
}

func withName(apm apmv1.ApmServer, name string) *apmv1.ApmServer {
	obj := apm.DeepCopy()
	obj.ObjectMeta.Name = name
	return obj
}

var _ client.Client = &fakeK8sClient{}

type fakeK8sClient struct {
	client.Client

	internalClient client.Client

	errors []error
}

func newFakeK8sClient(errors []error, objs ...runtime.Object) client.Client {
	return &fakeK8sClient{
		internalClient: k8s.NewFakeClient(objs...),
		errors:         errors,
	}
}

func (f *fakeK8sClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	if len(f.errors) > 0 {
		defer func() { f.errors = f.errors[1:] }()
		return f.errors[0]
	}
	return f.internalClient.Get(ctx, key, obj)
}
