// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/events"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecureSettingsSecret(t *testing.T) {
	require.Equal(t, "es-cluster-secure-settings", name.SecureSettingsSecret("es-cluster"))
}

func TestReconcileSecureSettings(t *testing.T) {
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	clusterObjMeta := metav1.ObjectMeta{
		Namespace: "ns",
		Name:      "es-cluster",
	}
	secureSettingsSecretMeta := metav1.ObjectMeta{
		Namespace: clusterObjMeta.Namespace,
		Name:      name.SecureSettingsSecret(clusterObjMeta.Name),
	}

	tests := []struct {
		name     string
		c        k8s.Client
		watches  watches.DynamicWatches
		es       v1alpha1.Elasticsearch
		expected corev1.Secret
	}{
		{
			name:    "no user secret",
			c:       k8s.WrapClient(fake.NewFakeClient()),
			watches: watches.NewDynamicWatches(),
			es: v1alpha1.Elasticsearch{
				ObjectMeta: clusterObjMeta,
				// no secure settings specified
				Spec: v1alpha1.ElasticsearchSpec{},
			},
			expected: corev1.Secret{
				ObjectMeta: secureSettingsSecretMeta,
				Data:       nil,
			},
		},
		{
			name: "user secret not found",
			// user secret does not exist in the apiserver
			c:       k8s.WrapClient(fake.NewFakeClient()),
			watches: watches.NewDynamicWatches(),
			es: v1alpha1.Elasticsearch{
				ObjectMeta: clusterObjMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					SecureSettings: &corev1.SecretReference{
						Namespace: "ns",
						Name:      "non-existing",
					},
				},
			},
			expected: corev1.Secret{
				ObjectMeta: secureSettingsSecretMeta,
				Data:       nil,
			},
		},
		{
			name: "empty user secret",
			// user secret exists, but has no data
			c: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "user-secret",
				},
			})),
			watches: watches.NewDynamicWatches(),
			es: v1alpha1.Elasticsearch{
				ObjectMeta: clusterObjMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					SecureSettings: &corev1.SecretReference{
						Namespace: "ns",
						Name:      "user-secret",
					},
				},
			},
			expected: corev1.Secret{
				ObjectMeta: secureSettingsSecretMeta,
				Data:       nil,
			},
		},
		{
			name: "new user secret",
			// new user secret was just added in the apiserver
			c: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "user-secret",
				},
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			})),
			watches: watches.NewDynamicWatches(),
			es: v1alpha1.Elasticsearch{
				ObjectMeta: clusterObjMeta,
				// it is referenced in the spec
				Spec: v1alpha1.ElasticsearchSpec{
					SecureSettings: &corev1.SecretReference{
						Namespace: "ns",
						Name:      "user-secret",
					},
				},
			},
			expected: corev1.Secret{
				ObjectMeta: secureSettingsSecretMeta,
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			},
		},
		{
			name: "same user secret content",
			// user secret and managed secret exist with the same content (no-op)
			c: k8s.WrapClient(
				fake.NewFakeClient(
					// user secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns",
							Name:      "user-secret",
						},
						Data: map[string][]byte{
							"key1": []byte("value1"),
							"key2": []byte("value2"),
						},
					},
					// managed secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns",
							Name:      name.SecureSettingsSecret(clusterObjMeta.Name),
						},
						Data: map[string][]byte{
							"key1": []byte("value1"),
							"key2": []byte("value2"),
						},
					},
				),
			),
			watches: watches.NewDynamicWatches(),
			es: v1alpha1.Elasticsearch{
				ObjectMeta: clusterObjMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					SecureSettings: &corev1.SecretReference{
						Namespace: "ns",
						Name:      "user-secret",
					},
				},
			},
			expected: corev1.Secret{
				ObjectMeta: secureSettingsSecretMeta,
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			},
		},
		{
			name: "user secret updated",
			// user secret and managed secret exist with a different content
			c: k8s.WrapClient(
				fake.NewFakeClient(
					// user secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns",
							Name:      "user-secret",
						},
						Data: map[string][]byte{
							"key1": []byte("value1"),
							"key2": []byte("value2"),
						},
					},
					// managed secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns",
							Name:      name.SecureSettingsSecret(clusterObjMeta.Name),
						},
						Data: map[string][]byte{
							"key1": []byte("value1-old"),
						},
					},
				),
			),
			watches: watches.NewDynamicWatches(),
			es: v1alpha1.Elasticsearch{
				ObjectMeta: clusterObjMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					SecureSettings: &corev1.SecretReference{
						Namespace: "ns",
						Name:      "user-secret",
					},
				},
			},
			expected: corev1.Secret{
				ObjectMeta: secureSettingsSecretMeta,
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			},
		},
		{
			name: "secure settings ref removed",
			// user secret and managed secret exist, but the ref was removed from the spec
			c: k8s.WrapClient(
				fake.NewFakeClient(
					// user secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns",
							Name:      "user-secret",
						},
						Data: map[string][]byte{
							"key1": []byte("value1"),
							"key2": []byte("value2"),
						},
					},
					// managed secret
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns",
							Name:      name.SecureSettingsSecret(clusterObjMeta.Name),
						},
						Data: map[string][]byte{
							"key1": []byte("value1"),
							"key2": []byte("value2"),
						},
					},
				),
			),
			watches: watches.NewDynamicWatches(),
			es: v1alpha1.Elasticsearch{
				ObjectMeta: clusterObjMeta,
				Spec:       v1alpha1.ElasticsearchSpec{
					// no secure settings referenced
				},
			},
			expected: corev1.Secret{
				ObjectMeta: secureSettingsSecretMeta,
				Data:       nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.watches.InjectScheme(scheme.Scheme))
			eventsRecorder := events.NewRecorder()
			err := ReconcileSecureSettings(tt.c, eventsRecorder, scheme.Scheme, tt.watches, tt.es)
			require.NoError(t, err)
			// managed secret should have been updated to match user secret
			actual := corev1.Secret{}
			err = tt.c.Get(k8s.ExtractNamespacedName(&secureSettingsSecretMeta), &actual)
			require.NoError(t, err)
			require.Equal(t, tt.expected.Data, actual.Data)
		})
	}
}

func Test_retrieveUserSecret(t *testing.T) {
	secretNs := "ns"
	secretName := "user-secret-name"
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNs,
			Name:      secretName,
		},
		Data: map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
		},
	}
	ref := corev1.SecretReference{
		Namespace: secretNs,
		Name:      secretName,
	}
	tests := []struct {
		name             string
		c                k8s.Client
		ref              corev1.SecretReference
		defaultNamespace string
		want             *corev1.Secret
		wantEvents       []events.Event
	}{
		{
			name:             "secret exists",
			c:                k8s.WrapClient(fake.NewFakeClient(&secret)),
			ref:              ref,
			defaultNamespace: "default-ns",
			want:             &secret,
			wantEvents:       []events.Event{},
		},
		{
			name:             "secret does not exist",
			c:                k8s.WrapClient(fake.NewFakeClient()),
			ref:              ref,
			defaultNamespace: "default-ns",
			want:             &corev1.Secret{},
			wantEvents: []events.Event{
				{
					EventType: corev1.EventTypeWarning,
					Reason:    events.EventReasonUnexpected,
					Message:   "Secure settings secret not found: user-secret-name",
				},
			},
		},
		{
			name:             "no namespace provided, use default one",
			c:                k8s.WrapClient(fake.NewFakeClient(&secret)),
			ref:              corev1.SecretReference{Name: secretName},
			defaultNamespace: secretNs,
			want:             &secret,
			wantEvents:       []events.Event{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventsRecorder := events.NewRecorder()
			got, err := retrieveUserSecret(tt.c, eventsRecorder, tt.ref, tt.defaultNamespace)
			require.NoError(t, err)
			require.Equal(t, tt.want.Data, got.Data)
			require.EqualValues(t, tt.wantEvents, eventsRecorder.Events())
		})
	}
}
