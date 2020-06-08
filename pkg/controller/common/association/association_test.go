// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"crypto/sha256"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_writeAuthSecretToConfigHash(t *testing.T) {
	for _, tt := range []struct {
		name       string
		client     k8s.Client
		assoc      func() commonv1.Association
		wantHashed string
		wantErr    bool
	}{
		{
			name:  "no association",
			assoc: func() commonv1.Association { return &beatv1beta1.Beat{} },
		},
		{
			name:   "association secret missing",
			client: k8s.WrappedFakeClient(),
			assoc: func() commonv1.Association {
				withAssoc := &beatv1beta1.Beat{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
				withAssoc.SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "secret-name",
					AuthSecretKey:  "secret-key",
				})
				return withAssoc
			},
			wantHashed: "",
			wantErr:    true,
		},
		{
			name: "association secret data missing",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "test-ns",
				},
			}),
			assoc: func() commonv1.Association {
				withAssoc := &beatv1beta1.Beat{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
				withAssoc.SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "secret-name",
					AuthSecretKey:  "non-existing-key",
				})
				return withAssoc
			},
			wantHashed: "",
			wantErr:    true,
		},
		{
			name: "association secret data present",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					"secret-key": []byte("123"),
				},
			}),
			assoc: func() commonv1.Association {
				withAssoc := &beatv1beta1.Beat{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
				withAssoc.SetAssociationConf(&commonv1.AssociationConf{
					AuthSecretName: "secret-name",
					AuthSecretKey:  "secret-key",
				})
				return withAssoc
			},
			wantHashed: "123",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configHashPassed := sha256.New224()
			gotErr := writeAuthSecretToConfigHash(tt.client, tt.assoc(), configHashPassed)
			require.Equal(t, tt.wantErr, gotErr != nil)

			configHash := sha256.New224()
			_, _ = configHash.Write([]byte(tt.wantHashed))
			require.Equal(t, configHash.Sum(nil), configHashPassed.Sum(nil))
		})
	}
}

func Test_writeCASecretToConfigHash(t *testing.T) {
	for _, tt := range []struct {
		name       string
		client     k8s.Client
		assoc      func() commonv1.Association
		wantHashed string
		wantErr    bool
	}{
		{
			name:  "no association",
			assoc: func() commonv1.Association { return &beatv1beta1.Beat{} },
		},
		{
			name:   "association ca secret missing",
			client: k8s.WrappedFakeClient(),
			assoc: func() commonv1.Association {
				withAssoc := &beatv1beta1.Beat{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
				withAssoc.SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "ca-secret-name",
				})
				return withAssoc
			},
			wantHashed: "",
			wantErr:    true,
		},
		{
			name: "association ca secret data missing",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "test-ns",
				},
			}),
			assoc: func() commonv1.Association {
				withAssoc := &beatv1beta1.Beat{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
				withAssoc.SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "ca-secret-name",
				})
				return withAssoc
			},
			wantHashed: "",
			wantErr:    true,
		},
		{
			name: "association ca secret data present",
			client: k8s.WrappedFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ca-secret-name",
					Namespace: "test-ns",
				},
				Data: map[string][]byte{
					certificates.CertFileName: []byte("456"),
				},
			}),
			assoc: func() commonv1.Association {
				withAssoc := &beatv1beta1.Beat{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}}
				withAssoc.SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "ca-secret-name",
				})
				return withAssoc
			},
			wantHashed: "456",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configHashPassed := sha256.New224()
			gotErr := writeCASecretToConfigHash(tt.client, tt.assoc(), configHashPassed)
			require.Equal(t, tt.wantErr, gotErr != nil)

			configHash := sha256.New224()
			_, _ = configHash.Write([]byte(tt.wantHashed))
			require.Equal(t, configHash.Sum(nil), configHashPassed.Sum(nil))
		})
	}
}
