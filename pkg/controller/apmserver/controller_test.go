// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
