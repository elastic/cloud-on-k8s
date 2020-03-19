// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	testNamespace = "ns"
)

var (
	sampleData        = map[string][]byte{"key1": []byte("data1"), "key2": []byte("data2")}
	sampleDataUpdated = map[string][]byte{"key1updated": []byte("data1updated"), "key2": []byte("data2")}
	sampleLabels      = map[string]string{"label1": "value1", "label2": "value2"}

	sampleAnnotations = map[string]string{"annotation1": "value1", "annotation2": "value2"}

	// the owner could be any type, we randomly pick another Secret resource here
	owner = createSecret("owner", nil, nil, nil)
)

func createSecret(name string, data map[string][]byte, labels map[string]string, annotations map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   testNamespace,
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Data: data,
	}
}

func withOwnerRef(t *testing.T, s *corev1.Secret) *corev1.Secret {
	err := controllerutil.SetControllerReference(owner, s, scheme.Scheme)
	require.NoError(t, err)
	return s
}

func TestReconcileSecret(t *testing.T) {
	tests := []struct {
		name     string
		c        k8s.Client
		expected *corev1.Secret
		want     *corev1.Secret
	}{
		{
			name:     "actual object does not exist: create the expected one",
			c:        k8s.WrappedFakeClient(),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations)),
		},
		{
			name:     "actual matches expected: do nothing",
			c:        k8s.WrappedFakeClient(withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations))),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations)),
		},
		{
			name:     "data should be updated",
			c:        k8s.WrappedFakeClient(withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations))),
			expected: createSecret("s", sampleDataUpdated, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleDataUpdated, sampleLabels, sampleAnnotations)),
		},
		{
			name:     "label and annotations should be updated",
			c:        k8s.WrappedFakeClient(withOwnerRef(t, createSecret("s", sampleData, nil, nil))),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations)),
		},
		{
			name: "preserve existing labels and annotations",
			c: k8s.WrappedFakeClient(withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{"existing": "existing"}, map[string]string{"existing": "existing"}),
			)),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want: withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing": "existing",                   // keep existing
					"label1":   "value1", "label2": "value2", // add expected
				}, map[string]string{
					"existing":    "existing",                        // keep existing
					"annotation1": "value1", "annotation2": "value2", // add expected
				}),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileSecret(tt.c, *tt.expected, owner)
			require.NoError(t, err)

			var retrieved corev1.Secret
			err = tt.c.Get(k8s.ExtractNamespacedName(tt.expected), &retrieved)
			require.NoError(t, err)

			for _, secret := range []corev1.Secret{got, retrieved} {
				// owner ref should be set
				require.Len(t, secret.OwnerReferences, 1)
				require.Equal(t, owner.Name, secret.OwnerReferences[0].Name)
				// data, labels and annotations should be expected
				require.Equal(t, tt.want.Data, secret.Data)
				require.Equal(t, tt.want.Annotations, secret.Annotations)
				require.Equal(t, tt.want.Labels, secret.Labels)
			}
		})
	}
}
