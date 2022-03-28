// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var nsnFixture = types.NamespacedName{
	Namespace: "ns",
	Name:      "test-resource",
}
var emsFixture = v1alpha1.ElasticMapsServer{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: nsnFixture.Namespace,
		Name:      nsnFixture.Name,
	},
	Spec: v1alpha1.MapsSpec{
		Version: "7.12.0",
		Count:   1,
	},
}

func TestReconcileMapsServer_Reconcile(t *testing.T) {
	timeFixture := metav1.Now()
	tests := []struct {
		name             string
		reconciler       ReconcileMapsServer
		pre              func(r ReconcileMapsServer)
		post             func(r ReconcileMapsServer)
		wantRequeue      bool
		wantRequeueAfter bool
		wantErr          bool
	}{
		{
			name: "Resource not found",
			reconciler: ReconcileMapsServer{
				Client:         k8s.NewFakeClient(),
				dynamicWatches: watches.NewDynamicWatches(),
			},
			pre: func(r ReconcileMapsServer) {
				// simulate a watch for a configRef that has been added during a previous reconciliation
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), common.ConfigRefWatchName(nsnFixture), []string{"user-config-secret"}))
				// simulate a watch for custom TLS certificates
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), certificates.CertificateWatchKey(EMSNamer, nsnFixture.Name), []string{"user-tls-secret"}))
				require.NotEmpty(t, r.DynamicWatches().Secrets.Registrations())
			},
			post: func(r ReconcileMapsServer) {
				// watches should have been cleared
				require.Empty(t, r.DynamicWatches().Secrets.Registrations())
			},
			wantErr: false,
		},
		{
			name: "Resource marked for deletion",
			reconciler: ReconcileMapsServer{
				Client: k8s.NewFakeClient(&v1alpha1.ElasticMapsServer{
					ObjectMeta: metav1.ObjectMeta{Name: nsnFixture.Name, Namespace: nsnFixture.Namespace, DeletionTimestamp: &timeFixture},
				}),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
				dynamicWatches: watches.NewDynamicWatches(),
			},
			pre: func(r ReconcileMapsServer) {
				// simulate a watch for a configRef that has been added during a previous reconciliation
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), common.ConfigRefWatchName(nsnFixture), []string{"user-config-secret"}))
				// simulate a watch for custom TLS certificates
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), certificates.CertificateWatchKey(EMSNamer, nsnFixture.Name), []string{"user-tls-secret"}))
				require.NotEmpty(t, r.DynamicWatches().Secrets.Registrations())
			},
			post: func(r ReconcileMapsServer) {
				// watches should have been cleared
				require.Empty(t, r.DynamicWatches().Secrets.Registrations())
			},
			wantErr: false,
		},
		{
			name: "Resource is unmanaged",
			reconciler: ReconcileMapsServer{
				Client: k8s.NewFakeClient(&v1alpha1.ElasticMapsServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nsnFixture.Name,
						Namespace: nsnFixture.Namespace,
						Annotations: map[string]string{
							common.ManagedAnnotation: "false",
						},
					},
				}),
			},
			wantErr: false,
		},
		{
			name: "License missing or invalid ",
			reconciler: ReconcileMapsServer{
				Client:         k8s.NewFakeClient(&emsFixture),
				recorder:       record.NewFakeRecorder(10),
				dynamicWatches: watches.NewDynamicWatches(),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false},
				Parameters:     operator.Parameters{OperatorInfo: about.OperatorInfo{BuildInfo: about.BuildInfo{Version: "1.6.0"}}},
			},
			post: func(r ReconcileMapsServer) {
				e := <-r.recorder.(*record.FakeRecorder).Events
				require.Equal(t, "Warning ReconciliationError Elastic Maps Server is an enterprise feature. Enterprise features are disabled", e)
			},
			wantRequeue:      true,
			wantRequeueAfter: true, // license recheck
			wantErr:          false,
		},
		{
			name: "validates on reconcile",
			reconciler: ReconcileMapsServer{
				Client: k8s.NewFakeClient(&v1alpha1.ElasticMapsServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nsnFixture.Name,
						Namespace: nsnFixture.Namespace,
					},
					Spec: v1alpha1.MapsSpec{
						Version: "7.10.0", // unsupported version
					},
				}),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
				recorder:       record.NewFakeRecorder(10),
			},
			wantErr: true,
		},
		{
			name: "Association specified but not configured (yet)",
			reconciler: ReconcileMapsServer{
				Client: k8s.NewFakeClient(&v1alpha1.ElasticMapsServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nsnFixture.Name,
						Namespace: nsnFixture.Namespace,
					},
					Spec: v1alpha1.MapsSpec{
						Version:          "7.12.0",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es", Namespace: "ns"},
					},
				}),
				dynamicWatches: watches.NewDynamicWatches(),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
				recorder:       record.NewFakeRecorder(10),
			},
			post: func(r ReconcileMapsServer) {
				e := <-r.recorder.(*record.FakeRecorder).Events
				require.Equal(t, "Warning AssociationError Association backend for elasticsearch is not configured", e)
			},
			wantErr: false,
		},
		{
			name: "Association specified but ES version too old",
			reconciler: ReconcileMapsServer{
				Client: k8s.NewFakeClient(&v1alpha1.ElasticMapsServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nsnFixture.Name,
						Namespace: nsnFixture.Namespace,
						Annotations: map[string]string{
							"association.k8s.elastic.co/es-conf": `{"authSecretName":"test-resource-maps-user","authSecretKey":"ns-test-resource-maps-user","caCertProvided":true,"caSecretName": "test-resource-es-ca","url":"https://es-es-http.ns.svc:9200","version":"7.10.0"}`,
						},
					},
					Spec: v1alpha1.MapsSpec{
						Version:          "7.12.0",
						ElasticsearchRef: commonv1.ObjectSelector{Name: "es", Namespace: "ns"},
					},
				}),
				dynamicWatches: watches.NewDynamicWatches(),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
				recorder:       record.NewFakeRecorder(10),
			},
			post: func(r ReconcileMapsServer) {
				e := <-r.recorder.(*record.FakeRecorder).Events
				require.Equal(t, "Warning Delayed Delaying deployment of version 7.12.0 since the referenced elasticsearch is not upgraded yet", e)
			},
			wantErr: false,
		},
		{
			name: "Happy path: first reconciliation",
			reconciler: ReconcileMapsServer{
				Client:         k8s.NewFakeClient(&emsFixture),
				recorder:       record.NewFakeRecorder(10),
				dynamicWatches: watches.NewDynamicWatches(),
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: true},
				Parameters:     operator.Parameters{OperatorInfo: about.OperatorInfo{BuildInfo: about.BuildInfo{Version: "1.6.0"}}},
			},
			post: func(r ReconcileMapsServer) {
				// service
				var svc corev1.Service
				err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: HTTPService(nsnFixture.Name)}, &svc)
				require.NoError(t, err)
				require.Equal(t, int32(8080), svc.Spec.Ports[0].Port)

				// should create internal ca, internal http certs secret, public http certs secret
				var caSecret corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-ems-http-ca-internal"}, &caSecret)
				require.NoError(t, err)
				require.NotEmpty(t, caSecret.Data)

				var httpInternalSecret corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-ems-http-certs-internal"}, &httpInternalSecret)
				require.NoError(t, err)
				require.NotEmpty(t, httpInternalSecret.Data)

				var httpPublicSecret corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-ems-http-certs-public"}, &httpPublicSecret)
				require.NoError(t, err)
				require.NotEmpty(t, httpPublicSecret.Data)

				// should create a secret for the configuration
				var config corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-ems-config"}, &config)
				require.NoError(t, err)
				require.Contains(t, string(config.Data["elastic-maps-server.yml"]), "host:")

				// should create a 1-replica deployment
				var dep appsv1.Deployment
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-ems"}, &dep)
				require.NoError(t, err)
				require.Equal(t, int32(1), *dep.Spec.Replicas)
				// with the config hash annotation set
				require.NotEmpty(t, dep.Spec.Template.Annotations[configHashAnnotationName])
			},
			wantRequeue:      false,
			wantRequeueAfter: true, // certificate refresh
			wantErr:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pre != nil {
				tt.pre(tt.reconciler)
			}
			got, err := tt.reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: nsnFixture})
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Requeue != tt.wantRequeue {
				t.Errorf("Reconcile() got = %v, wantRequeue %v", got, tt.wantRequeue)
			}
			if got.RequeueAfter > 0 != tt.wantRequeueAfter {
				t.Errorf("Reconcile() got = %v, wantRequeueAfter %v", got, tt.wantRequeueAfter)
			}
			if tt.post != nil {
				tt.post(tt.reconciler)
			}
		})
	}
}

func Test_buildConfigHash(t *testing.T) {
	emsWithAssoc := *emsFixture.DeepCopy()
	esTLSCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: nsnFixture.Namespace, Name: "es-tls-certs"},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("es-cert-data"),
		},
	}
	emsWithAssoc.SetAssociationConf(&commonv1.AssociationConf{CACertProvided: true, CASecretName: esTLSCertsSecret.Name})

	emsNoTLS := *emsWithAssoc.DeepCopy()
	emsNoTLS.Spec.HTTP.TLS = commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}

	cfgFixture := corev1.Secret{
		Data: map[string][]byte{
			ConfigFilename: []byte("host: 0.0.0.0"),
		},
	}
	tlsCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: nsnFixture.Namespace, Name: certificates.InternalCertsSecretName(EMSNamer, nsnFixture.Name)},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("cert-data"),
		},
	}
	type args struct {
		c            k8s.Client
		ems          v1alpha1.ElasticMapsServer
		configSecret corev1.Secret
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "full configuration",
			args: args{
				c:            k8s.NewFakeClient(&tlsCertsSecret, &esTLSCertsSecret),
				ems:          emsWithAssoc,
				configSecret: cfgFixture,
			},
			want:    "3032871734",
			wantErr: false,
		},
		{
			name: "no TLS",
			args: args{
				c:            k8s.NewFakeClient(&esTLSCertsSecret),
				ems:          emsNoTLS,
				configSecret: cfgFixture,
			},
			want:    "2560904737",
			wantErr: false,
		},
		{
			name: "No association",
			args: args{
				c:            k8s.NewFakeClient(&tlsCertsSecret),
				ems:          emsFixture,
				configSecret: cfgFixture,
			},
			want:    "3032871734",
			wantErr: false,
		},
		{
			name: "TLS cert not found",
			args: args{
				c:            k8s.NewFakeClient(),
				ems:          emsFixture,
				configSecret: cfgFixture,
			},
			wantErr: true,
		},
		{
			name: "ES TLS cert not found",
			args: args{
				c:            k8s.NewFakeClient(&tlsCertsSecret),
				ems:          emsWithAssoc,
				configSecret: cfgFixture,
			},
			want:    "3032871734",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildConfigHash(tt.args.c, tt.args.ems, tt.args.configSecret)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildConfigHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("buildConfigHash() got = %v, want %v", got, tt.want)
			}
		})
	}
}
