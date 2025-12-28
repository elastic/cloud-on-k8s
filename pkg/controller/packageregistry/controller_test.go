// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

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

	"github.com/elastic/cloud-on-k8s/v3/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var nsnFixture = types.NamespacedName{
	Namespace: "ns",
	Name:      "test-resource",
}
var eprFixture = v1alpha1.PackageRegistry{
	ObjectMeta: metav1.ObjectMeta{
		Namespace:  nsnFixture.Namespace,
		Name:       nsnFixture.Name,
		Generation: 2,
	},
	Spec: v1alpha1.PackageRegistrySpec{
		Version: "7.17.8",
		Count:   1,
	},
	Status: v1alpha1.PackageRegistryStatus{
		ObservedGeneration: 1,
	},
}

func TestReconcilePackageRegistry_Reconcile(t *testing.T) {
	timeFixture := metav1.Now()

	assertObservedGeneration := func(r k8s.Client, expected int) {
		var epr v1alpha1.PackageRegistry
		require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: nsnFixture.Name, Namespace: nsnFixture.Namespace}, &epr))
		require.Equal(t, int64(expected), epr.Status.ObservedGeneration)
	}
	tests := []struct {
		name             string
		reconciler       ReconcilePackageRegistry
		pre              func(r ReconcilePackageRegistry)
		post             func(r ReconcilePackageRegistry)
		wantRequeueAfter bool
		wantErr          bool
	}{
		{
			name: "Resource not found",
			reconciler: ReconcilePackageRegistry{
				Client:         k8s.NewFakeClient(),
				dynamicWatches: watches.NewDynamicWatches(),
			},
			pre: func(r ReconcilePackageRegistry) {
				// simulate a watch for a configRef that has been added during a previous reconciliation
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), common.ConfigRefWatchName(nsnFixture), []string{"user-config-secret"}))
				// simulate a watch for custom TLS certificates
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), certificates.CertificateWatchKey(v1alpha1.Namer, nsnFixture.Name), []string{"user-tls-secret"}))
				require.NotEmpty(t, r.DynamicWatches().Secrets.Registrations())
			},
			post: func(r ReconcilePackageRegistry) {
				// watches should have been cleared
				require.Empty(t, r.DynamicWatches().Secrets.Registrations())
			},
			wantErr: false,
		},
		{
			name: "Resource marked for deletion",
			reconciler: ReconcilePackageRegistry{
				Client: k8s.NewFakeClient(&v1alpha1.PackageRegistry{
					ObjectMeta: metav1.ObjectMeta{
						Name:              nsnFixture.Name,
						Namespace:         nsnFixture.Namespace,
						DeletionTimestamp: &timeFixture, Generation: 2,
						Finalizers: []string{"something"},
					},
					Status: v1alpha1.PackageRegistryStatus{
						ObservedGeneration: 1,
					},
				}),
				dynamicWatches: watches.NewDynamicWatches(),
			},
			pre: func(r ReconcilePackageRegistry) {
				// simulate a watch for a configRef that has been added during a previous reconciliation
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), common.ConfigRefWatchName(nsnFixture), []string{"user-config-secret"}))
				// simulate a watch for custom TLS certificates
				require.NoError(t, watches.WatchUserProvidedSecrets(nsnFixture, r.DynamicWatches(), certificates.CertificateWatchKey(v1alpha1.Namer, nsnFixture.Name), []string{"user-tls-secret"}))
				require.NotEmpty(t, r.DynamicWatches().Secrets.Registrations())
			},
			post: func(r ReconcilePackageRegistry) {
				// watches should have been cleared
				require.Empty(t, r.DynamicWatches().Secrets.Registrations())

				// observedGeneration should not have been updated
				assertObservedGeneration(r, 1)
			},
			wantErr: false,
		},
		{
			name: "Resource is unmanaged",
			reconciler: ReconcilePackageRegistry{
				Client: k8s.NewFakeClient(&v1alpha1.PackageRegistry{
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
			name: "validates on reconcile",
			reconciler: ReconcilePackageRegistry{
				Client: k8s.NewFakeClient(&v1alpha1.PackageRegistry{
					ObjectMeta: metav1.ObjectMeta{
						Name:       nsnFixture.Name,
						Namespace:  nsnFixture.Namespace,
						Generation: 2,
					},
					Spec: v1alpha1.PackageRegistrySpec{
						Version: "7.14.0", // unsupported version - below minimum 7.17.8
					},
					Status: v1alpha1.PackageRegistryStatus{
						ObservedGeneration: 1,
					},
				}),
				recorder: record.NewFakeRecorder(10),
			},
			post: func(r ReconcilePackageRegistry) {
				// observedGeneration should have been updated
				assertObservedGeneration(r, 2)
			},
			wantErr: true,
		},
		{
			name: "Happy path: first reconciliation",
			reconciler: ReconcilePackageRegistry{
				Client:         k8s.NewFakeClient(&eprFixture),
				recorder:       record.NewFakeRecorder(10),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{OperatorInfo: about.OperatorInfo{BuildInfo: about.BuildInfo{Version: "1.6.0"}}},
			},
			post: func(r ReconcilePackageRegistry) {
				// service
				var svc corev1.Service
				err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: HTTPServiceName(nsnFixture.Name)}, &svc)
				require.NoError(t, err)
				require.Equal(t, int32(8080), svc.Spec.Ports[0].Port)

				// should create internal ca, internal http certs secret, public http certs secret
				var caSecret corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-epr-http-ca-internal"}, &caSecret)
				require.NoError(t, err)
				require.NotEmpty(t, caSecret.Data)

				var httpInternalSecret corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-epr-http-certs-internal"}, &httpInternalSecret)
				require.NoError(t, err)
				require.NotEmpty(t, httpInternalSecret.Data)

				var httpPublicSecret corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-epr-http-certs-public"}, &httpPublicSecret)
				require.NoError(t, err)
				require.NotEmpty(t, httpPublicSecret.Data)

				// should create a secret for the configuration
				var config corev1.Secret
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-epr-config"}, &config)
				require.NoError(t, err)
				require.Contains(t, string(config.Data["config.yml"]), "package_paths")

				// should create a 1-replica deployment
				var dep appsv1.Deployment
				err = r.Client.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "test-resource-epr"}, &dep)
				require.NoError(t, err)
				require.Equal(t, int32(1), *dep.Spec.Replicas)
				// with the config hash annotation set
				require.NotEmpty(t, dep.Spec.Template.Annotations[configHashAnnotationName])

				// observedGeneration should have been updated
				assertObservedGeneration(r, 2)
			},
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
	epr := *eprFixture.DeepCopy()
	eprNoTLS := *epr.DeepCopy()
	eprNoTLS.Spec.HTTP.TLS = commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}

	cfgFixture := corev1.Secret{
		Data: map[string][]byte{
			ConfigFilename: []byte("host: 0.0.0.0"),
		},
	}
	tlsCertsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: nsnFixture.Namespace, Name: certificates.InternalCertsSecretName(v1alpha1.Namer, nsnFixture.Name)},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("cert-data"),
		},
	}
	type args struct {
		epr             v1alpha1.PackageRegistry
		configSecret    corev1.Secret
		httpCertificate *certificates.CertificatesSecret
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "full configuration",
			args: args{
				epr:             epr,
				configSecret:    cfgFixture,
				httpCertificate: &certificates.CertificatesSecret{Secret: tlsCertsSecret},
			},
			want: "3032871734",
		},
		{
			name: "no TLS",
			args: args{
				epr:             eprNoTLS,
				configSecret:    cfgFixture,
				httpCertificate: nil,
			},
			want: "2560904737",
		},
		{
			name: "TLS but nil http certificate",
			args: args{
				epr:             eprFixture,
				configSecret:    cfgFixture,
				httpCertificate: nil,
			},
			want: "2560904737",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildConfigHash(tt.args.epr, tt.args.configSecret, tt.args.httpCertificate)
			if got != tt.want {
				t.Errorf("buildConfigHash() got = %v, want %v", got, tt.want)
			}
		})
	}
}
