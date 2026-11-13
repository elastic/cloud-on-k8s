// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"bytes"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	v1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func elasticsearchWithAdditionalCAs() v1.Elasticsearch {
	return v1.Elasticsearch{
		Namespace: "ns", Name: "es",
		Spec: v1.ElasticsearchSpec{Transport: v1.TransportConfig{TLS: v1.TransportTLSOptions{
			CertificateAuthorities: commonv1.ConfigMapRef{ConfigMapName: "my-trust"},
		}}},
	}
}

func TestReconcileAdditionalCAs(t *testing.T) {
	caPEM := certificates.EncodePEMCert(testRSACA.Cert.Raw)
	otherCA := genCA(t)
	otherPEM := certificates.EncodePEMCert(otherCA.Cert.Raw)
	keyPEM, err := certificates.EncodePEMPrivateKey(testRSACA.PrivateKey)
	require.NoError(t, err)
	invalidCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("garbage")})
	tests := []struct {
		name             string
		data             map[string]string
		noRef            bool
		configMapMissing bool
		want             []byte
		wantErr          string
	}{
		{name: "no reference", noRef: true},
		{name: "missing config map", configMapMissing: true,
			wantErr: `could not retrieve config map ns/my-trust specified in spec.transport.tls.certificateAuthorities: configmaps "my-trust" not found`},
		{name: "missing ca.crt", wantErr: "config map ns/my-trust specified in spec.transport.tls.certificateAuthorities must contain ca.crt file"},
		{name: "valid certificate", data: map[string]string{certificates.CAFileName: string(caPEM)}, want: caPEM},
		{name: "multiple certificates", data: map[string]string{certificates.CAFileName: string(caPEM) + string(otherPEM)}, want: certificates.EncodePEMCert(testRSACA.Cert.Raw, otherCA.Cert.Raw)},
		{name: "missing trailing newline", data: map[string]string{certificates.CAFileName: string(bytes.TrimSuffix(caPEM, []byte("\n")))}, want: caPEM},
		{name: "private keys are filtered", data: map[string]string{certificates.CAFileName: string(keyPEM) + string(caPEM) + string(keyPEM) + string(otherPEM)}, want: certificates.EncodePEMCert(testRSACA.Cert.Raw, otherCA.Cert.Raw)},
		{name: "empty bundle", data: map[string]string{certificates.CAFileName: ""}, wantErr: "no certificate found in ca.crt"},
		{name: "non PEM data", data: map[string]string{certificates.CAFileName: "not a certificate"}, wantErr: "no certificate found in ca.crt"},
		{name: "private key only", data: map[string]string{certificates.CAFileName: string(keyPEM)}, wantErr: "no certificate found in ca.crt"},
		{name: "invalid certificate", data: map[string]string{certificates.CAFileName: string(invalidCert)}, wantErr: "contains invalid certificates"},
		{name: "invalid certificate after valid one", data: map[string]string{certificates.CAFileName: string(caPEM) + string(invalidCert)}, wantErr: "contains invalid certificates"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := elasticsearchWithAdditionalCAs()
			if tt.noRef {
				es.Spec.Transport.TLS.CertificateAuthorities = commonv1.ConfigMapRef{}
			}
			c := k8s.NewFakeClient()
			if !tt.configMapMissing {
				require.NoError(t, c.Create(t.Context(), &corev1.ConfigMap{Namespace: es.Namespace, Name: "my-trust", Data: tt.data}))
			}
			dw := watches.NewDynamicWatches("testCtr")
			got, err := ReconcileAdditionalCAs(t.Context(), c, es, dw)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), "spec.transport.tls.certificateAuthorities")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
			expectedRegistrations := 1
			if tt.noRef {
				expectedRegistrations = 0
			}
			assert.Len(t, dw.ConfigMaps.Registrations(), expectedRegistrations)

			if len(got) > 0 {
				// Exercise the consumers: only certificates reach the public and node transport secrets.
				require.NoError(t, ReconcileTransportCertsPublicSecret(t.Context(), c, es, testRSACA, got, metadata.Metadata{}))
				var public corev1.Secret
				require.NoError(t, c.Get(t.Context(), PublicCertsSecretRef(k8s.ExtractNamespacedName(&es)), &public))
				node := corev1.Secret{Data: map[string][]byte{}}
				mayBeUpdateCAFile(&node, testRSACA, got)
				expected := append(bytes.Clone(caPEM), tt.want...)
				for _, data := range []map[string][]byte{public.Data, node.Data} {
					assert.Equal(t, expected, data[certificates.CAFileName])
					certs, err := certificates.ParsePEMCerts(data[certificates.CAFileName])
					require.NoError(t, err)
					require.NotEmpty(t, certs)
					assert.NotContains(t, string(data[certificates.CAFileName]), "PRIVATE KEY")
				}
			}
		})
	}
}

func TestReconcileAdditionalCAsWatch(t *testing.T) {
	es := elasticsearchWithAdditionalCAs()
	owner := k8s.ExtractNamespacedName(&es)
	watchKey := AdditionalCAWatchKey(owner)
	c := k8s.NewFakeClient()
	dw := watches.NewDynamicWatches("testCtr")
	cm := &corev1.ConfigMap{Namespace: es.Namespace, Name: "my-trust", Data: map[string]string{
		certificates.CAFileName: string(certificates.EncodePEMCert(testRSACA.Cert.Raw)),
	}}
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	assertEnqueued := func(cm *corev1.ConfigMap, wantReconcile bool) {
		t.Helper()
		dw.ConfigMaps.Create(t.Context(), event.TypedCreateEvent[*corev1.ConfigMap]{Object: cm}, q)
		if !wantReconcile {
			require.Zero(t, q.Len())
			return
		}
		require.Equal(t, 1, q.Len())
		req, _ := q.Get()
		assert.Equal(t, reconcile.Request{NamespacedName: owner}, req)
		q.Done(req)
	}

	// A missing ConfigMap must already be watched so creating it can recover reconciliation.
	_, err := ReconcileAdditionalCAs(t.Context(), c, es, dw)
	require.Error(t, err)
	require.Contains(t, dw.ConfigMaps.Registrations(), watchKey)
	assertEnqueued(&corev1.ConfigMap{Namespace: "other", Name: cm.Name}, false)
	assertEnqueued(cm, true)
	require.NoError(t, c.Create(t.Context(), cm))
	_, err = ReconcileAdditionalCAs(t.Context(), c, es, dw)
	require.NoError(t, err)

	// Changing the reference replaces the watched object even when the new ConfigMap is missing.
	es.Spec.Transport.TLS.CertificateAuthorities.ConfigMapName = "new-trust"
	_, err = ReconcileAdditionalCAs(t.Context(), c, es, dw)
	require.Error(t, err)
	assertEnqueued(cm, false)
	assertEnqueued(&corev1.ConfigMap{Namespace: es.Namespace, Name: "new-trust"}, true)

	es.Spec.Transport.TLS.CertificateAuthorities = commonv1.ConfigMapRef{}
	got, err := ReconcileAdditionalCAs(t.Context(), c, es, dw)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.NotContains(t, dw.ConfigMaps.Registrations(), watchKey)
	assertEnqueued(&corev1.ConfigMap{Namespace: es.Namespace, Name: "new-trust"}, false)
}
