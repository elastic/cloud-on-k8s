// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/go-test/deep"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var expectedLicenseSpec = v1alpha1.EnterpriseLicenseSpec{
	LicenseMeta: v1alpha1.LicenseMeta{
		UID:                "840F0DB6-1906-452E-98C7-6F94E6012CD7",
		IssueDateInMillis:  1548115200000,
		ExpiryDateInMillis: 1561247999999,
		IssuedTo:           "test org",
		Issuer:             "test issuer",
		StartDateInMillis:  1548115200000,
	},
	Type:         "enterprise",
	MaxInstances: 40,
	SignatureRef: corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: "test-org-6F94E6012CD7-license-sigs",
		},
		Key: "enterprise-license-sig",
	},
	ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
		{
			LicenseMeta: v1alpha1.LicenseMeta{
				UID:                "73117B2A-FEEA-4FEC-B8F6-49D764E9F1DA",
				IssueDateInMillis:  1548115200000,
				ExpiryDateInMillis: 1561247999999,
				IssuedTo:           "test org",
				Issuer:             "test issuer",
				StartDateInMillis:  1548115200000,
			},
			MaxNodes: 100,
			Type:     "gold",
			SignatureRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "test-org-6F94E6012CD7-license-sigs",
				},
				Key: "49D764E9F1DA",
			},
		},
		{
			LicenseMeta: v1alpha1.LicenseMeta{
				UID:                "57E312E2-6EA0-49D0-8E65-AA5017742ACF",
				IssueDateInMillis:  1548115200000,
				ExpiryDateInMillis: 1561247999999,
				IssuedTo:           "test org",
				Issuer:             "test issuer",
				StartDateInMillis:  1548115200000,
			},
			MaxNodes: 100,
			Type:     "platinum",
			SignatureRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "test-org-6F94E6012CD7-license-sigs",
				},
				Key: "AA5017742ACF",
			},
		},
	},
	Eula: v1alpha1.EulaState{
		Accepted: false,
	},
}

var expectedSecret = corev1.Secret{
	ObjectMeta: v1.ObjectMeta{
		Name:      "test-org-6F94E6012CD7-license-sigs",
		Namespace: "default",
	},
	Data: map[string][]byte{
		"enterprise-license-sig": []byte("test signature"),
		"49D764E9F1DA":           []byte("test signature gold"),
		"AA5017742ACF":           []byte("test signature platinum"),
	},
}

func Test_extractTransformLoadLicense(t *testing.T) {
	require.NoError(t, apis.AddToScheme(scheme.Scheme))
	type args struct {
		p Params
	}
	tests := []struct {
		name      string
		args      args
		wantErr   bool
		assertion func(client.Client)
	}{
		{
			name: "invalid input: FAIL",
			args: args{
				p: Params{
					LicenseFile: "testdata/test-error.json",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid file: FAIL",
			args: args{
				p: Params{
					Client: nil,
				},
			},
			wantErr: true,
		},
		{
			name: "valid input: OK",
			args: args{
				p: Params{
					OperatorNs:  "default",
					LicenseFile: "testdata/test-license.json",
					Eula:        false,
					Client:      fake.NewFakeClient(),
				},
			},
			wantErr: false,
			assertion: func(c client.Client) {

				var actual v1alpha1.EnterpriseLicense
				require.NoError(t, c.Get(
					context.Background(),
					types.NamespacedName{Namespace: "default", Name: "test-org-6F94E6012CD7"},
					&actual,
				))
				if diff := deep.Equal(actual.Spec, expectedLicenseSpec); diff != nil {
					t.Error(diff)
				}
				var actualSec corev1.Secret
				require.NoError(t, c.Get(
					context.Background(),
					types.NamespacedName{Namespace: "default", Name: "test-org-6F94E6012CD7-license-sigs"},
					&actualSec,
				))

				if diff := deep.Equal(actualSec, expectedSecret); diff != nil {
					t.Error(diff)
				}
			},
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			if err := extractTransformLoadLicense(tt.args.p); (err != nil) != tt.wantErr {
				t.Errorf("extractTransformLoadLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.assertion != nil {
				tt.assertion(tt.args.p.Client)
			}
		})
	}
}
