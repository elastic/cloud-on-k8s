// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"bytes"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestReconcileAdditionalCAs(t *testing.T) {
	const fieldPath = "spec.some.field"
	owner := types.NamespacedName{Namespace: "ns", Name: "owner"}
	definedRef := commonv1.ConfigMapRef{ConfigMapName: "my-trust"}
	caPEM := EncodePEMCert(testCA.Cert.Raw)
	keyPEM := []byte(testPemPrivateKey)
	invalidCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("garbage")})
	leafPEM := EncodePEMCert(cert)

	tests := []struct {
		name             string
		ref              commonv1.ConfigMapRef
		configMapMissing bool
		data             map[string]string
		caOnly           bool
		want             []byte
		wantErr          string
		wantInvalid      bool
	}{
		{name: "no reference"},
		{name: "missing config map", ref: definedRef, configMapMissing: true,
			wantErr: `could not retrieve config map ns/my-trust specified in spec.some.field: configmaps "my-trust" not found`},
		{name: "missing ca.crt", ref: definedRef, wantErr: "config map ns/my-trust specified in spec.some.field must contain ca.crt file", wantInvalid: true},
		{name: "valid certificate", ref: definedRef, data: map[string]string{CAFileName: string(caPEM)}, want: caPEM},
		{name: "missing trailing newline", ref: definedRef, data: map[string]string{CAFileName: string(bytes.TrimSuffix(caPEM, []byte("\n")))}, want: caPEM},
		{name: "private keys are filtered", ref: definedRef, data: map[string]string{CAFileName: string(keyPEM) + string(caPEM) + string(keyPEM)}, want: caPEM},
		{name: "private key only", ref: definedRef, data: map[string]string{CAFileName: string(keyPEM)}, wantErr: "no certificate found in ca.crt", wantInvalid: true},
		{name: "empty bundle", ref: definedRef, data: map[string]string{CAFileName: ""}, wantErr: "no certificate found in ca.crt", wantInvalid: true},
		{name: "invalid certificate", ref: definedRef, data: map[string]string{CAFileName: string(invalidCert)}, wantErr: "contains invalid certificates", wantInvalid: true},
		{name: "invalid certificate after valid one", ref: definedRef, data: map[string]string{CAFileName: string(caPEM) + string(invalidCert)}, wantErr: "contains invalid certificates", wantInvalid: true},
		{name: "non-CA certificate is accepted by default", ref: definedRef, data: map[string]string{CAFileName: string(caPEM) + string(leafPEM)}, want: EncodePEMCert(testCA.Cert.Raw, cert)},
		{name: "CA certificate with CA only", ref: definedRef, data: map[string]string{CAFileName: string(caPEM)}, caOnly: true, want: caPEM},
		{name: "non-CA certificate with CA only", ref: definedRef, data: map[string]string{CAFileName: string(caPEM) + string(leafPEM)}, caOnly: true,
			wantErr: "is not a CA certificate, it must have the BasicConstraints extension set to CA:TRUE", wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient()
			if !tt.configMapMissing {
				require.NoError(t, c.Create(t.Context(), &corev1.ConfigMap{Namespace: owner.Namespace, Name: "my-trust", Data: tt.data}))
			}
			dw := watches.NewDynamicWatches("testCtr")
			got, err := ReconcileAdditionalCAs(t.Context(), c, dw, AdditionalCAsOptions{
				Owner: owner, Ref: tt.ref, WatchKey: "my-watch", FieldPath: fieldPath, CAOnly: tt.caOnly,
			})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), fieldPath)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
			// only invalid content is reported as an InvalidCAsError, not a config map which could not be read
			_, invalid := errors.AsType[*InvalidCAsError](err)
			assert.Equal(t, tt.wantInvalid, invalid)

			// the watch is registered before the ConfigMap is loaded, whatever the outcome
			expectedRegistrations := 1
			if !tt.ref.IsDefined() {
				expectedRegistrations = 0
			}
			assert.Len(t, dw.ConfigMaps.Registrations(), expectedRegistrations)
		})
	}
}

func TestReconcileAdditionalCAs_ReleasesWatch(t *testing.T) {
	owner := types.NamespacedName{Namespace: "ns", Name: "owner"}
	c := k8s.NewFakeClient(&corev1.ConfigMap{
		Namespace: owner.Namespace, Name: "my-trust",
		Data: map[string]string{CAFileName: string(EncodePEMCert(testCA.Cert.Raw))},
	})
	dw := watches.NewDynamicWatches("testCtr")
	opts := AdditionalCAsOptions{Owner: owner, Ref: commonv1.ConfigMapRef{ConfigMapName: "my-trust"}, WatchKey: "my-watch", FieldPath: "spec.some.field"}

	_, err := ReconcileAdditionalCAs(t.Context(), c, dw, opts)
	require.NoError(t, err)
	require.Contains(t, dw.ConfigMaps.Registrations(), "my-watch")

	opts.Ref = commonv1.ConfigMapRef{}
	got, err := ReconcileAdditionalCAs(t.Context(), c, dw, opts)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.NotContains(t, dw.ConfigMaps.Registrations(), "my-watch")
}
