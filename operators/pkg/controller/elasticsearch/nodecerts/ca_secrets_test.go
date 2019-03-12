// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package nodecerts

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_CACertSecretName(t *testing.T) {
	require.Equal(t, "mycluster-ca", CACertSecretName(testName))
}

func Test_CAPrivateKeySecretName(t *testing.T) {
	require.Equal(t, "mycluster-ca-private-key", caPrivateKeySecretName(testName))
}

func Test_secretsForCA(t *testing.T) {
	cluster := types.NamespacedName{
		Namespace: testNamespace,
		Name:      testName,
	}
	testCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	require.NoError(t, err)

	privateKeySecret, certSecret := secretsForCA(*testCa, cluster)

	assert.Equal(t, testNamespace, privateKeySecret.Namespace)
	assert.Equal(t, testName+"-ca", certSecret.Name)
	assert.Len(t, certSecret.Data, 1)
	assert.NotEmpty(t, certSecret.Data[certificates.CAFileName])

	assert.Equal(t, cluster.Namespace, privateKeySecret.Namespace)
	assert.Equal(t, testName+"-ca-private-key", privateKeySecret.Name)
	assert.Len(t, privateKeySecret.Data, 1)
	assert.NotEmpty(t, privateKeySecret.Data[CAPrivateKeyFileName])

}
func Test_caFromSecrets(t *testing.T) {
	cluster := types.NamespacedName{
		Namespace: testNamespace,
		Name:      testName,
	}
	testCa, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	require.NoError(t, err)
	privateKeySecret, certSecret := secretsForCA(*testCa, cluster)

	tests := []struct {
		name             string
		certSecret       corev1.Secret
		privateKeySecret corev1.Secret
		wantCa           *certificates.CA
		wantOK           bool
	}{
		{
			name:             "valid secrets",
			certSecret:       certSecret,
			privateKeySecret: privateKeySecret,
			wantCa:           testCa,
			wantOK:           true,
		},
		{
			name:             "empty cert secret",
			certSecret:       corev1.Secret{},
			privateKeySecret: privateKeySecret,
			wantCa:           nil,
			wantOK:           false,
		},
		{
			name:             "empty private key secret",
			certSecret:       certSecret,
			privateKeySecret: corev1.Secret{},
			wantCa:           nil,
			wantOK:           false,
		},
		{
			name: "invalid cert secret",
			certSecret: corev1.Secret{
				Data: map[string][]byte{
					certificates.CAFileName: []byte("invalid"),
				},
			},
			privateKeySecret: privateKeySecret,
			wantCa:           nil,
			wantOK:           false,
		},
		{
			name:       "invalid private key secret",
			certSecret: certSecret,
			privateKeySecret: corev1.Secret{
				Data: map[string][]byte{
					CAPrivateKeyFileName: []byte("invalid"),
				},
			},
			wantCa: nil,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca, ok := caFromSecrets(tt.certSecret, tt.privateKeySecret)
			if !reflect.DeepEqual(ca, tt.wantCa) {
				t.Errorf("CaFromSecrets() got = %v, want %v", ca, tt.wantCa)
			}
			if ok != tt.wantOK {
				t.Errorf("CaFromSecrets() got = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}
