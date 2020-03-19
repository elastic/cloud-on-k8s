// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"testing"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_reuseOrGenerateSecrets(t *testing.T) {
	type args struct {
		c    k8s.Client
		ents entsv1beta1.EnterpriseSearch
	}
	tests := []struct {
		name      string
		args      args
		assertion func(*testing.T, *settings.CanonicalConfig, error)
		wantErr   bool
	}{
		{
			name: "Do not override existing keys",
			args: args{
				c: k8s.WrappedFakeClient(
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "entsearch-sample-ents-config"},
						Data: map[string][]byte{
							ConfigFilename: []byte(existingConfigWithSecureSettings),
						},
					},
				),
				ents: entsv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "entsearch-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				expectedSettings := settings.MustCanonicalConfig(map[string]interface{}{
					SecretSessionKey:  "alreadysetsessionkey",
					EncryptionKeysKey: []string{"alreadysetencryptionkey1", "alreadysetencryptionkey2"},
				})
				assert.Equal(t, expectedSettings, got)
			},
		},
		{
			name: "Do not override existing encryption keys, create missing session key",
			args: args{
				c: k8s.WrappedFakeClient(
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "entsearch-sample-ents-config"},
						Data: map[string][]byte{
							ConfigFilename: []byte(existingConfig),
						},
					},
				),
				ents: entsv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "entsearch-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				// Unpack the configuration to check that some default secure settings have been generated
				var e enterpriseSearchSecrets
				assert.NoError(t, got.Unpack(&e))
				assert.Equal(t, len(e.EncryptionKeysKey), 1)     // We set 1 encryption key by default
				assert.Equal(t, len(e.EncryptionKeysKey[0]), 32) // encryption key length should be 32
				assert.Equal(t, len(e.SecretSessionKey), 32)     // session key length should be 24
			},
		},
		{
			name: "Create missing keys",
			args: args{
				c: k8s.WrappedFakeClient(
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "entsearch-sample-ents-config"},
						Data: map[string][]byte{
							ConfigFilename: []byte(existingConfig),
						},
					},
				),
				ents: entsv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "entsearch-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				// Unpack the configuration to check that some default secure settings have been generated
				var e enterpriseSearchSecrets
				assert.NoError(t, got.Unpack(&e))
				assert.Equal(t, len(e.EncryptionKeysKey), 1)     // We set 1 encryption key by default
				assert.Equal(t, len(e.EncryptionKeysKey[0]), 32) // encryption key length should be 32
				assert.Equal(t, len(e.SecretSessionKey), 32)     // session key length should be 32
			},
		},
		{
			name: "No configuration to reuse",
			args: args{
				c: k8s.WrappedFakeClient(),
				ents: entsv1beta1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "entsearch-sample"},
				},
			},
			assertion: func(t *testing.T, got *settings.CanonicalConfig, err error) {
				// Unpack the configuration to check that some default secure settings have been generated
				var e enterpriseSearchSecrets
				assert.NoError(t, got.Unpack(&e))
				assert.Equal(t, len(e.EncryptionKeysKey), 1)     // We set 1 encryption key by default
				assert.Equal(t, len(e.EncryptionKeysKey[0]), 32) // encryption key length should be 32
				assert.Equal(t, len(e.SecretSessionKey), 32)     // session key length should be 32
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reuseOrGenerateSecretKeys(tt.args.c, tt.args.ents)
			if (err != nil) != tt.wantErr {
				t.Errorf("reuseOrGenerateSecretKeys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.assertion(t, got, err)
		})
	}
}
