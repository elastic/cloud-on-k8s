// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	testNamespace = "ns"
)

var (
	sampleData        = map[string][]byte{"key1": []byte("data1"), "key2": []byte("data2")}
	sampleDataUpdated = map[string][]byte{"key1updated": []byte("data1updated"), "key2": []byte("data2")}
	sampleLabels      = map[string]string{"label1": "value1", "label2": "value2"}

	sampleAnnotations = map[string]string{"annotation1": "value1", "annotation2": "value2"}

	owner = &esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
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
	t.Helper()
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
			c:        k8s.NewFakeClient(),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations)),
		},
		{
			name:     "actual matches expected: do nothing",
			c:        k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations))),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations)),
		},
		{
			name:     "data should be updated",
			c:        k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations))),
			expected: createSecret("s", sampleDataUpdated, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleDataUpdated, sampleLabels, sampleAnnotations)),
		},
		{
			name:     "label and annotations should be updated",
			c:        k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData, nil, nil))),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want:     withOwnerRef(t, createSecret("s", sampleData, sampleLabels, sampleAnnotations)),
		},
		{
			name: "preserve existing labels and annotations",
			c: k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData,
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
		{
			name: "reset secure settings and hash config annotations",
			c: k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing": "existing",
				},
				map[string]string{
					"policy.k8s.elastic.co/secure-settings-secrets": "[{..}]",
					"policy.k8s.elastic.co/settings-hash":           "hash-1",
					"existing":                                      "existing",
				}),
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
		{
			name: "reset soft owner labels",
			c: k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing":                           "existing",
					"eck.k8s.elastic.co/owner-namespace": "test",
					"eck.k8s.elastic.co/owner-name":      "test",
					"eck.k8s.elastic.co/owner-kind":      "StackConfigPolicy",
				},
				sampleAnnotations,
			))),
			expected: createSecret("s", sampleData, sampleLabels, sampleAnnotations),
			want: withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing": "existing",                   // keep existing
					"label1":   "value1", "label2": "value2", // add expected
				}, sampleAnnotations,
			)),
		},
		{
			name: "override secure settings and hash config annotations",
			c: k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{"existing": "existing"},
				map[string]string{
					"policy.k8s.elastic.co/secure-settings-secrets": `[{"secretName":"secret-1"}]`,
					"policy.k8s.elastic.co/settings-hash":           "hash-1",
					"existing":                                      "existing",
				}),
			)),
			expected: createSecret("s", sampleData, sampleLabels, map[string]string{
				"policy.k8s.elastic.co/secure-settings-secrets": `[{"secretName":"secret-2"}]`,
				"policy.k8s.elastic.co/settings-hash":           "hash-2",
			}),
			want: withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing": "existing",                   // keep existing
					"label1":   "value1", "label2": "value2", // add expected
				}, map[string]string{
					"policy.k8s.elastic.co/secure-settings-secrets": `[{"secretName":"secret-2"}]`,
					"policy.k8s.elastic.co/settings-hash":           "hash-2",
					"existing":                                      "existing",
				}),
			),
		},
		{
			name: "remove secure settings from expected",
			c: k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{"existing": "existing"},
				map[string]string{
					"policy.k8s.elastic.co/secure-settings-secrets": `[{"secretName":"secret-1"}]`,
					"policy.k8s.elastic.co/settings-hash":           "hash-1",
				}),
			)),
			expected: createSecret("s", sampleData, map[string]string{"existing": "existing"}, map[string]string{
				"policy.k8s.elastic.co/settings-hash": "hash-1",
			}),
			want: withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing": "existing", // keep existing
				}, map[string]string{
					"policy.k8s.elastic.co/settings-hash": "hash-1",
				}),
			),
		},
		{
			name: "override soft owner labels",
			c: k8s.NewFakeClient(withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing":                           "existing",
					"eck.k8s.elastic.co/owner-namespace": "x",
					"eck.k8s.elastic.co/owner-name":      "x",
					"eck.k8s.elastic.co/owner-kind":      "x",
				},
				sampleAnnotations,
			))),
			expected: createSecret("s", sampleData, map[string]string{
				"eck.k8s.elastic.co/owner-namespace": "test",
				"eck.k8s.elastic.co/owner-name":      "test",
				"eck.k8s.elastic.co/owner-kind":      "StackConfigPolicy",
			}, sampleAnnotations),
			want: withOwnerRef(t, createSecret("s", sampleData,
				map[string]string{
					"existing":                           "existing", // keep existing
					"eck.k8s.elastic.co/owner-namespace": "test",
					"eck.k8s.elastic.co/owner-name":      "test",
					"eck.k8s.elastic.co/owner-kind":      "StackConfigPolicy",
				}, sampleAnnotations,
			)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ReconcileSecret(context.Background(), tt.c, *tt.expected, owner)
			require.NoError(t, err)

			var secret corev1.Secret
			err = tt.c.Get(context.Background(), k8s.ExtractNamespacedName(tt.expected), &secret)
			require.NoError(t, err)

			// owner ref should be set
			require.Len(t, secret.OwnerReferences, 1)
			require.Equal(t, owner.Name, secret.OwnerReferences[0].Name)
			// data, labels and annotations should be expected
			require.Equal(t, tt.want.Data, secret.Data)
			require.Equal(t, tt.want.Annotations, secret.Annotations)
			require.Equal(t, tt.want.Labels, secret.Labels)
		})
	}
}

func Test_ReconcileEmptyFileSettingsSecret(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: "esNs",
		Name:      "esName",
	}}

	fakeClient := k8s.NewFakeClient()

	err := ReconcileEmptyFileSettingsSecret(context.Background(), fakeClient, es, true)
	assert.NoError(t, err)

	var secret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret)
	assert.NoError(t, err)
	var settings Settings
	err = json.Unmarshal(secret.Data[SettingsSecretKey], &settings)
	assert.NoError(t, err)
	// check that the Secret is empty
	assert.Empty(t, settings.State.ClusterSettings.Data)
	assert.Empty(t, settings.State.SnapshotRepositories.Data)
	assert.Empty(t, settings.State.SLM.Data)

	// reconcile again with create only: secret is not reconciled
	err = ReconcileEmptyFileSettingsSecret(context.Background(), fakeClient, es, true)
	assert.NoError(t, err)

	var secret2 corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret2)
	assert.NoError(t, err)
	// check that the Secret was not updated
	assert.Equal(t, "1", secret2.ResourceVersion)

	// reconcile again without create only: secret is reconciled but its content hasn't changed
	err = ReconcileEmptyFileSettingsSecret(context.Background(), fakeClient, es, false)
	assert.NoError(t, err)

	var secret3 corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret3)
	assert.NoError(t, err)
	// check that the Secret was not updated
	assert.NotEqual(t, "1", secret3.ResourceVersion)
}
