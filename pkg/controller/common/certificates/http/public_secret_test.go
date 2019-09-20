// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"encoding/json"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestReconcileHTTPCertsPublicSecret(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")

	owner := &v1alpha1.Elasticsearch{
		ObjectMeta: v1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
	}

	certificate := &CertificatesSecret{
		Data: map[string][]byte{
			certificates.CAFileName:   ca,
			certificates.CertFileName: tls,
			certificates.KeyFileName:  key,
		},
	}

	namespacedSecretName := PublicCertsSecretRef(name.ESNamer, k8s.ExtractNamespacedName(owner))

	mkClient := func(t *testing.T, objs ...runtime.Object) k8s.Client {
		t.Helper()
		return k8s.WrapClient(fake.NewFakeClient(objs...))
	}

	mkWantedSecret := func(t *testing.T) *corev1.Secret {
		t.Helper()
		wantSecret := &corev1.Secret{
			ObjectMeta: k8s.ToObjectMeta(namespacedSecretName),
			Data: map[string][]byte{
				certificates.CertFileName: tls,
				certificates.CAFileName:   ca,
			},
		}

		if err := controllerutil.SetControllerReference(owner, wantSecret, scheme.Scheme); err != nil {
			t.Fatal(err)
		}

		return wantSecret
	}

	tests := []struct {
		name       string
		client     func(*testing.T, ...runtime.Object) k8s.Client
		wantSecret func(*testing.T) *corev1.Secret
		wantErr    bool
	}{
		{
			name:       "is created if missing",
			client:     mkClient,
			wantSecret: mkWantedSecret,
		},
		{
			name: "is updated on mismatch",
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
				s := mkWantedSecret(t)
				s.Data[certificates.CertFileName] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "removes extraneous keys",
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
				s := mkWantedSecret(t)
				s.Data["extra"] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "preserves labels and annotations",
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
				s := mkWantedSecret(t)
				if s.Labels == nil {
					s.Labels = make(map[string]string)
				}
				s.Labels["label1"] = "labelValue1"
				s.Labels["label2"] = "labelValue2"
				if s.Annotations == nil {
					s.Annotations = make(map[string]string)
				}
				s.Annotations["annotation1"] = "annotationValue1"
				s.Annotations["annotation2"] = "annotationValue2"
				return mkClient(t, s)
			},
			wantSecret: func(t *testing.T) *corev1.Secret {
				s := mkWantedSecret(t)
				if s.Labels == nil {
					s.Labels = make(map[string]string)
				}
				s.Labels["label1"] = "labelValue1"
				s.Labels["label2"] = "labelValue2"
				if s.Annotations == nil {
					s.Annotations = make(map[string]string)
				}
				s.Annotations["annotation1"] = "annotationValue1"
				s.Annotations["annotation2"] = "annotationValue2"
				return s
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client(t)
			err := ReconcileHTTPCertsPublicSecret(client, scheme.Scheme, owner, name.ESNamer, certificate)
			if tt.wantErr {
				require.Error(t, err, "Failed to reconcile")
				return
			}

			var gotSecret corev1.Secret
			err = client.Get(namespacedSecretName, &gotSecret)
			require.NoError(t, err, "Failed to get secret")

			wantSecret := tt.wantSecret(t)
			jsonEqual(t, wantSecret, gotSecret)
		})
	}
}

func jsonEqual(t *testing.T, a, b interface{}) {
	t.Helper()
	obj1, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}

	obj2, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, string(obj1), string(obj2))
}
