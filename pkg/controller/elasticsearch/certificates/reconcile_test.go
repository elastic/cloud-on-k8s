// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestReconcileOperatorClientCertAndTrustBundle_AdditionalTrustedCAs(t *testing.T) {
	esNSN := types.NamespacedName{Namespace: "ns", Name: "es"}
	watchKey := certificates.AdditionalTrustedClientCAsWatchKey(esv1.ESNamer, esNSN)
	trustBundleNSN := types.NamespacedName{Namespace: esNSN.Namespace, Name: certificates.ClientCertTrustBundleSecretName(esv1.ESNamer, esNSN.Name)}

	ca, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	require.NoError(t, err)
	caPEM := certificates.EncodePEMCert(ca.Cert.Raw)
	caSecret := &corev1.Secret{
		Namespace: esNSN.Namespace, Name: "additional-ca",
		Data: map[string][]byte{certificates.CAFileName: caPEM},
	}
	newES := func(additionalTrustedCA bool) *esv1.Elasticsearch {
		es := &esv1.Elasticsearch{Namespace: esNSN.Namespace, Name: esNSN.Name}
		es.Spec.HTTP.TLS.Client.Authentication = true
		if additionalTrustedCA {
			es.Spec.HTTP.TLS.Client.AdditionalTrustedCAs = []commonv1.SecretRef{{SecretName: caSecret.Name}}
		}
		return es
	}
	certRotation := certificates.RotationParams{Validity: certificates.DefaultCertValidity, RotateBefore: certificates.DefaultRotateBefore}
	trustBundle := func(t *testing.T, c k8s.Client) string {
		t.Helper()
		var secret corev1.Secret
		require.NoError(t, c.Get(t.Context(), trustBundleNSN, &secret))
		return string(secret.Data[certificates.ClientCertificatesTrustBundleFileName])
	}

	deleteCASecret := func(t *testing.T, _ *esv1.Elasticsearch, c k8s.Client) {
		t.Helper()
		require.NoError(t, c.Delete(t.Context(), caSecret.DeepCopy()))
	}

	// caSecretUnreadable makes reading the secret of the additional trusted CA fail, for another reason than not existing
	var caSecretUnreadable bool

	for _, tt := range []struct {
		name string
		es   *esv1.Elasticsearch
		// previouslyReconciled reconciles once with client authentication required, before change is applied
		previouslyReconciled bool
		change               func(t *testing.T, es *esv1.Elasticsearch, c k8s.Client)
		// clientAuthenticationRequired is passed to the reconciliation under test
		clientAuthenticationRequired bool
		// wantErr is an error in the results, wantDeferredErr an error returned apart for the caller to report once
		// the nodes are reconciled
		wantErr         bool
		wantDeferredErr bool
		// wantCert is whether the operator client certificate is returned: the trust bundle is checked if so
		wantCert    bool
		wantTrusted bool
		wantWatch   bool
		wantWarning bool
	}{
		{
			name:                         "additional trusted CA is part of the trust bundle and its secret is watched",
			es:                           newES(true),
			clientAuthenticationRequired: true,
			wantCert:                     true,
			wantTrusted:                  true,
			wantWatch:                    true,
		},
		{
			name:                 "the reference is removed: the CA is not trusted anymore and the watch is removed",
			es:                   newES(true),
			previouslyReconciled: true,
			change: func(_ *testing.T, es *esv1.Elasticsearch, _ k8s.Client) {
				es.Spec.HTTP.TLS.Client.AdditionalTrustedCAs = nil
			},
			clientAuthenticationRequired: true,
			wantCert:                     true,
		},
		{
			name:                         "additional trusted CA secret is deleted: deferred error, the CA is not trusted anymore and the operator certificate returned",
			es:                           newES(true),
			previouslyReconciled:         true,
			change:                       deleteCASecret,
			clientAuthenticationRequired: true,
			wantDeferredErr:              true,
			wantCert:                     true,
			wantWatch:                    true,
		},
		{
			// the trust bundle is mounted in the Pods, which cannot start without it
			name:                         "additional trusted CA secret is missing from the start: deferred error, trust bundle is created without it",
			es:                           newES(true),
			change:                       deleteCASecret,
			clientAuthenticationRequired: true,
			wantDeferredErr:              true,
			wantCert:                     true,
			wantWatch:                    true,
		},
		{
			// what the secret holds is not known: the CA is not removed from the trust bundle
			name:                         "additional trusted CA secret cannot be read: error in the results, the trust bundle is left as it is",
			es:                           newES(true),
			previouslyReconciled:         true,
			change:                       func(_ *testing.T, _ *esv1.Elasticsearch, _ k8s.Client) { caSecretUnreadable = true },
			clientAuthenticationRequired: true,
			wantErr:                      true,
			wantCert:                     true,
			wantTrusted:                  true,
			wantWatch:                    true,
		},
		{
			// client authentication is not required anymore, but the annotation is still there until the nodes are rolled
			name:                 "client authentication is being disabled and the additional trusted CA secret is deleted: no error, the CA is not trusted anymore",
			es:                   newES(true),
			previouslyReconciled: true,
			change:               deleteCASecret,
			wantCert:             true,
			wantWatch:            true,
		},
		{
			// what the secret holds is not known: retried like any other API error, even while disabling
			name:                 "client authentication is being disabled and the additional trusted CA secret cannot be read: error in the results, the trust bundle is left as it is",
			es:                   newES(true),
			previouslyReconciled: true,
			change:               func(_ *testing.T, _ *esv1.Elasticsearch, _ k8s.Client) { caSecretUnreadable = true },
			wantErr:              true,
			wantCert:             true,
			wantTrusted:          true,
			wantWatch:            true,
		},
		{
			name:                 "client authentication is not in use: the watch is removed and a warning is emitted",
			es:                   newES(true),
			previouslyReconciled: true,
			// client authentication is disabled and the resources cleaned up, which removes the annotation
			change: func(_ *testing.T, es *esv1.Elasticsearch, _ k8s.Client) {
				delete(es.Annotations, annotation.ClientAuthenticationRequiredAnnotation)
			},
			wantWarning: true,
		},
		{
			name: "client authentication is not in use, no additional trusted CA: no warning",
			es:   newES(false),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caSecretUnreadable = false
			d := driver.TestDriver{
				Client: k8s.NewFakeClientBuilder(tt.es, caSecret).WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if caSecretUnreadable && key.Name == caSecret.Name {
							return apierrors.NewInternalError(errors.New("cannot read the secret"))
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).Build(),
				Watches:      watches.NewDynamicWatches(),
				FakeRecorder: toolsevents.NewFakeRecorder(10),
			}
			if tt.previouslyReconciled {
				_, results, deferredErr := ReconcileOperatorClientCertAndTrustBundle(t.Context(), d, tt.es, true, certRotation, metadata.Metadata{})
				require.False(t, results.HasError())
				require.NoError(t, deferredErr)
				require.Empty(t, d.FakeRecorder.Events)
				require.Contains(t, trustBundle(t, d.Client), string(caPEM))
			}
			if tt.change != nil {
				tt.change(t, tt.es, d.Client)
			}

			cert, results, deferredErr := ReconcileOperatorClientCertAndTrustBundle(t.Context(), d, tt.es, tt.clientAuthenticationRequired, certRotation, metadata.Metadata{})
			require.Equal(t, tt.wantErr, results.HasError())
			require.Equal(t, tt.wantDeferredErr, deferredErr != nil)
			require.Equal(t, tt.wantCert, cert != nil)
			if tt.wantCert {
				bundle := trustBundle(t, d.Client)
				require.Equal(t, tt.wantTrusted, strings.Contains(bundle, string(caPEM)))
				// the operator client certificate is trusted in any case
				require.Contains(t, bundle, string(certificates.EncodePEMCert(cert.Certificate[0])))
			}
			if tt.wantWatch {
				require.Contains(t, d.Watches.Secrets.Registrations(), watchKey)
			} else {
				require.NotContains(t, d.Watches.Secrets.Registrations(), watchKey)
			}
			if tt.wantWarning {
				require.Len(t, d.FakeRecorder.Events, 1)
				require.Contains(t, <-d.FakeRecorder.Events, "spec.http.tls.client.additionalTrustedCAs is ineffective")
			} else {
				require.Empty(t, d.FakeRecorder.Events)
			}
		})
	}
}
