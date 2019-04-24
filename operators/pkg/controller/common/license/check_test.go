// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestChecker_CommercialFeaturesEnabled(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
	type fields struct {
		initialObjects    []runtime.Object
		operatorNamespace string
		publicKey         []byte
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		{
			name: "valid license: OK",
			fields: fields{
				initialObjects:    withSignature(licenseFixture, signatureFixture),
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			want: true,
		},
		{
			name: "no secret: FAIL",
			fields: fields{
				initialObjects:    withSignature(licenseFixture, signatureFixture)[:1],
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "wrong namespace: FAIL",
			fields: fields{
				initialObjects:    withSignature(licenseFixture, signatureFixture),
				operatorNamespace: "another-ns",
				publicKey:         publicKeyBytesFixture(t),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "no public key: FAIL",
			fields: fields{
				initialObjects:    withSignature(licenseFixture, signatureFixture),
				operatorNamespace: "test-system",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := &Checker{
				k8sClient:         k8s.WrapClient(fake.NewFakeClientWithScheme(scheme.Scheme, tt.fields.initialObjects...)),
				operatorNamespace: tt.fields.operatorNamespace,
				publicKey:         tt.fields.publicKey,
			}
			got, err := lc.CommercialFeaturesEnabled()
			if (err != nil) != tt.wantErr {
				t.Errorf("Checker.CommercialFeaturesEnabled() err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Checker.CommercialFeaturesEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
