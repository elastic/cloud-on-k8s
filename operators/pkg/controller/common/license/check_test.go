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
	}
	tests := []struct {
		name   string
		around func(*testing.T, func())
		fields fields
		want   bool
	}{
		{
			name:   "valid license: OK",
			around: withPublicKeyFixture,
			fields: fields{
				initialObjects:    withSignature(licenseFixture, signatureFixture),
				operatorNamespace: "test-system",
			},
			want: true,
		},
		{
			name:   "no secret: FAIL",
			around: withPublicKeyFixture,
			fields: fields{
				initialObjects:    withSignature(licenseFixture, signatureFixture)[:0],
				operatorNamespace: "test-system",
			},
			want: false,
		},
		{
			name:   "wrong namespace: FAIL",
			around: withPublicKeyFixture,
			fields: fields{
				initialObjects:    withSignature(licenseFixture, signatureFixture),
				operatorNamespace: "another-ns",
			},
			want: false,
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
				client:            k8s.WrapClient(fake.NewFakeClientWithScheme(scheme.Scheme, tt.fields.initialObjects...)),
				operatorNamespace: tt.fields.operatorNamespace,
			}
			test := func() {
				if got := lc.CommercialFeaturesEnabled(); got != tt.want {
					t.Errorf("Checker.CommercialFeaturesEnabled() = %v, want %v", got, tt.want)
				}
			}
			if tt.around != nil {
				tt.around(t, test)
			} else {
				test()
			}
		})
	}
}
