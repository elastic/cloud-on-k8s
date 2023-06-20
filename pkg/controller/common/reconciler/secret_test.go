// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconciler

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileSecret(context.Background(), tt.c, *tt.expected, owner)
			require.NoError(t, err)

			var retrieved corev1.Secret
			err = tt.c.Get(context.Background(), k8s.ExtractNamespacedName(tt.expected), &retrieved)
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

func concatMaps(m1 map[string]string, m2 map[string]string) map[string]string {
	newMap := map[string]string{}
	maps.Merge(newMap, m1)
	maps.Merge(newMap, m2)
	return newMap
}

func TestReconcileSecretNoOwnerRef(t *testing.T) {
	softOwner := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es-name", UID: types.UID("es-uid")},
		TypeMeta:   metav1.TypeMeta{Kind: esv1.Kind},
	}
	expectedSoftOwnerLabels := map[string]string{
		SoftOwnerNamespaceLabel: "ns",
		SoftOwnerNameLabel:      "es-name",
		SoftOwnerKindLabel:      esv1.Kind,
	}
	sampleSecret := createSecret("s", sampleData, sampleLabels, sampleAnnotations)
	sampleLabelsWithSoftOwnerRef := concatMaps(sampleLabels, expectedSoftOwnerLabels)
	sampleSecretWithSoftOwnerRef := createSecret("s", sampleData, sampleLabelsWithSoftOwnerRef, sampleAnnotations)
	tests := []struct {
		name      string
		c         k8s.Client
		expected  *corev1.Secret
		softOwner runtime.Object
		want      *corev1.Secret
	}{
		{
			name:      "actual object does not exist: create the expected one",
			c:         k8s.NewFakeClient(),
			expected:  sampleSecret,
			softOwner: softOwner,
			want:      sampleSecretWithSoftOwnerRef,
		},
		{
			name:      "actual matches expected: do nothing",
			c:         k8s.NewFakeClient(sampleSecretWithSoftOwnerRef),
			expected:  sampleSecret,
			softOwner: softOwner,
			want:      sampleSecretWithSoftOwnerRef,
		},
		{
			name:      "data should be updated",
			c:         k8s.NewFakeClient(sampleSecretWithSoftOwnerRef),
			expected:  createSecret("s", sampleDataUpdated, sampleLabels, sampleAnnotations),
			softOwner: softOwner,
			want:      createSecret("s", sampleDataUpdated, sampleLabelsWithSoftOwnerRef, sampleAnnotations),
		},
		{
			name:      "label and annotations should be updated",
			c:         k8s.NewFakeClient(createSecret("s", sampleData, nil, nil)),
			expected:  sampleSecret,
			softOwner: softOwner,
			want:      sampleSecretWithSoftOwnerRef,
		},
		{
			name: "preserve existing labels and annotations",
			c: k8s.NewFakeClient(createSecret("s", sampleData,
				map[string]string{"existing": "existing"}, map[string]string{"existing": "existing"}),
			),
			expected:  createSecret("s", sampleData, sampleLabelsWithSoftOwnerRef, sampleAnnotations),
			softOwner: softOwner,
			want: createSecret("s", sampleData,
				concatMaps(expectedSoftOwnerLabels, map[string]string{
					"existing": "existing",                   // keep existing
					"label1":   "value1", "label2": "value2", // add expected
				}),
				map[string]string{
					"existing":    "existing",                        // keep existing
					"annotation1": "value1", "annotation2": "value2", // add expected
				},
			),
		},
		{
			name:      "remove existing ownerRef, replace with soft owner labels, don't touch other owner regs",
			c:         k8s.NewFakeClient(addOwner(addOwner(sampleSecret, softOwner.Name, softOwner.UID), "unrelated-owner", "unrelated-owner-id")),
			expected:  sampleSecret,
			softOwner: softOwner,
			want:      addOwner(sampleSecretWithSoftOwnerRef, "unrelated-owner", "unrelated-owner-id"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileSecretNoOwnerRef(context.Background(), tt.c, *tt.expected, tt.softOwner)
			require.NoError(t, err)

			var retrieved corev1.Secret
			err = tt.c.Get(context.Background(), k8s.ExtractNamespacedName(tt.expected), &retrieved)
			require.NoError(t, err)

			for _, secret := range []corev1.Secret{got, retrieved} {
				// owner refs should be expected
				if len(tt.want.OwnerReferences) == 0 {
					require.Len(t, secret.OwnerReferences, 0)
				} else {
					require.Equal(t, tt.want.OwnerReferences, secret.OwnerReferences)
				}
				// data, labels and annotations should be expected
				require.Equal(t, tt.want.Data, secret.Data)
				require.Equal(t, tt.want.Annotations, secret.Annotations)
				require.Equal(t, tt.want.Labels, secret.Labels)
			}
		})
	}
}

func sampleOwner() *corev1.Secret {
	// we use a secret here but it could be any Elasticsearch | Kibana | ApmServer | etc.
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "owner-name", UID: "owner-id"},
		TypeMeta:   metav1.TypeMeta{Kind: "Secret"},
	}
}

func addOwner(secret *corev1.Secret, name string, uid types.UID) *corev1.Secret {
	secret = secret.DeepCopy()
	secret.OwnerReferences = append(secret.OwnerReferences, metav1.OwnerReference{Name: name, UID: uid})
	return secret
}

func ownedSecret(namespace, name, ownerNs, ownerName, ownerKind string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{
			SoftOwnerNameLabel:      ownerName,
			SoftOwnerNamespaceLabel: ownerNs,
			SoftOwnerKindLabel:      ownerKind,
		}}}
}

func TestGarbageCollectSoftOwnedSecrets(t *testing.T) {
	tests := []struct {
		name            string
		existingSecrets []client.Object
		deletedOwner    types.NamespacedName
		ownerKind       string
		wantObjs        []client.Object
	}{
		{
			name:            "no soft-owned secret to gc",
			existingSecrets: nil,
			deletedOwner:    k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:       "Secret",
			wantObjs:        nil,
		},
		{
			name: "gc soft-owned secret",
			existingSecrets: []client.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind)},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs:     nil,
		},
		{
			name: "don't gc secret with no owner label",
			existingSecrets: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: sampleOwner().Namespace, Name: sampleOwner().Name}}},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: sampleOwner().Namespace, Name: sampleOwner().Name}}},
		},
		{
			name: "don't gc secret pointing to a soft owner with a different name",
			existingSecrets: []client.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, "another-name", sampleOwner().Kind)},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []client.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, "another-name", sampleOwner().Kind)},
		},
		{
			name: "don't gc secret pointing to a soft owner with a different namespace",
			existingSecrets: []client.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind)},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []client.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind)},
		},
		{
			name: "don't gc secret pointing to a soft owner with a different kind",
			existingSecrets: []client.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, "another-kind")},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []client.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, "another-kind")},
		},
		{
			name: "2 secrets to gc out of 5 secrets",
			existingSecrets: []client.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-3", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-4", sampleOwner().Namespace, "another-owner", sampleOwner().Kind),
				ownedSecret("ns", "secret-5", sampleOwner().Namespace, sampleOwner().Name, "another-kind"),
			},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []client.Object{
				ownedSecret("ns", "secret-4", sampleOwner().Namespace, "another-owner", sampleOwner().Kind),
				ownedSecret("ns", "secret-5", sampleOwner().Namespace, sampleOwner().Name, "another-kind"),
			},
		},
		{
			name: "gc secrets pointing to a stackconfigpolicy soft owner with a different namespace",
			existingSecrets: []client.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", "another-namespace", sampleOwner().Name, "StackConfigPolicy"),
				ownedSecret("ns-2", "secret-1", "another-namespace", sampleOwner().Name, "StackConfigPolicy"),
			},
			deletedOwner: types.NamespacedName{Name: sampleOwner().Name, Namespace: "another-namespace"},
			ownerKind:    "StackConfigPolicy",
			wantObjs: []client.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.existingSecrets...)
			err := GarbageCollectSoftOwnedSecrets(context.Background(), c, tt.deletedOwner, tt.ownerKind)
			require.NoError(t, err)
			var retrievedSecrets corev1.SecretList
			err = c.List(context.Background(), &retrievedSecrets)
			require.NoError(t, err)
			require.Equal(t, len(tt.wantObjs), len(retrievedSecrets.Items))
			for i := range tt.wantObjs {
				require.Equal(t, tt.wantObjs[i].(*corev1.Secret).Name, retrievedSecrets.Items[i].Name) //nolint:forcetypeassert
			}
		})
	}
}

func TestGarbageCollectAllSoftOwnedOrphanSecrets(t *testing.T) {
	ownerKinds := map[string]client.Object{
		"Secret":            &corev1.Secret{},
		"StackConfigPolicy": &policyv1alpha1.StackConfigPolicy{},
	}
	tests := []struct {
		name        string
		runtimeObjs []client.Object
		wantObjs    []client.Object
		assert      func(t *testing.T, c k8s.Client)
	}{
		{
			name: "nothing to gc",
			runtimeObjs: []client.Object{
				// owner exists, 2 owned secrets
				sampleOwner(),
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
			wantObjs: []client.Object{
				sampleOwner(),
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
		},
		{
			name: "gc 2 secrets",
			runtimeObjs: []client.Object{
				// owner doesn't exist: gc these 2 secrets
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
			wantObjs: []client.Object{},
		},
		{
			name: "don't gc secret targeting an owner in a different namespace",
			runtimeObjs: []client.Object{
				// secret likely copied manually into another namespace
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind),
			},
			wantObjs: []client.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind),
			},
		},
		{
			name: "don't gc resources of a non-managed Kind",
			runtimeObjs: []client.Object{
				// configmap whose owner doesn't exist, should not be gc
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "configmap-name", Labels: map[string]string{
					SoftOwnerNameLabel:      "owner-name",
					SoftOwnerNamespaceLabel: "ns",
					SoftOwnerKindLabel:      "ConfigMap",
				}}},
			},
			assert: func(t *testing.T, c k8s.Client) {
				t.Helper()
				// configmap should still be there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "configmap-name"}, &corev1.ConfigMap{}))
			},
		},
		{
			name: "gc secrets pointing to a stackconfigpolicy soft owner with a different namespace",
			runtimeObjs: []client.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", "another-namespace", sampleOwner().Name, "StackConfigPolicy"),
				ownedSecret("ns-2", "secret-2", "another-namespace", sampleOwner().Name, "StackConfigPolicy"),
				ownedSecret("ns", "secret-3", "another-another-namespace", sampleOwner().Name, "StackConfigPolicy"),
			},
			wantObjs: []client.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.runtimeObjs...)
			err := GarbageCollectAllSoftOwnedOrphanSecrets(context.Background(), c, ownerKinds)
			require.NoError(t, err)
			var retrievedSecrets corev1.SecretList
			err = c.List(context.Background(), &retrievedSecrets)
			require.NoError(t, err)
			require.Equal(t, len(tt.wantObjs), len(retrievedSecrets.Items))
			for i := range tt.wantObjs {
				require.Equal(t, tt.wantObjs[i].(*corev1.Secret).Name, retrievedSecrets.Items[i].Name) //nolint:forcetypeassert
			}
			if tt.assert != nil {
				tt.assert(t, c)
			}
		})
	}
}

func TestSoftOwnerRefFromLabels(t *testing.T) {
	tests := []struct {
		name           string
		labels         map[string]string
		wantSoftOwner  SoftOwnerRef
		wantReferenced bool
	}{
		{
			name: "return soft owner reference",
			labels: map[string]string{
				SoftOwnerNamespaceLabel: "ns",
				SoftOwnerNameLabel:      "name",
				SoftOwnerKindLabel:      "kind",
			},
			wantSoftOwner:  SoftOwnerRef{Namespace: "ns", Name: "name", Kind: "kind"},
			wantReferenced: true,
		},
		{
			name:           "no soft owner labels",
			labels:         nil,
			wantSoftOwner:  SoftOwnerRef{},
			wantReferenced: false,
		},
		{
			name: "namespace empty: no soft owner",
			labels: map[string]string{
				SoftOwnerNamespaceLabel: "",
				SoftOwnerNameLabel:      "name",
				SoftOwnerKindLabel:      "kind",
			},
			wantSoftOwner:  SoftOwnerRef{},
			wantReferenced: false,
		},
		{
			name: "namespace missing: no soft owner",
			labels: map[string]string{
				SoftOwnerNameLabel: "name",
				SoftOwnerKindLabel: "kind",
			},
			wantSoftOwner:  SoftOwnerRef{},
			wantReferenced: false,
		},
		{
			name: "name empty: no soft owner",
			labels: map[string]string{
				SoftOwnerNamespaceLabel: "ns",
				SoftOwnerNameLabel:      "",
				SoftOwnerKindLabel:      "kind",
			},
			wantSoftOwner:  SoftOwnerRef{},
			wantReferenced: false,
		},
		{
			name: "name missing: no soft owner",
			labels: map[string]string{
				SoftOwnerNamespaceLabel: "namespace",
				SoftOwnerKindLabel:      "kind",
			},
			wantSoftOwner:  SoftOwnerRef{},
			wantReferenced: false,
		},
		{
			name: "kind empty: no soft owner",
			labels: map[string]string{
				SoftOwnerNamespaceLabel: "ns",
				SoftOwnerNameLabel:      "name",
				SoftOwnerKindLabel:      "",
			},
			wantSoftOwner:  SoftOwnerRef{},
			wantReferenced: false,
		},
		{
			name: "kind missing: no soft owner",
			labels: map[string]string{
				SoftOwnerNamespaceLabel: "ns",
				SoftOwnerNameLabel:      "name",
			},
			wantSoftOwner:  SoftOwnerRef{},
			wantReferenced: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := SoftOwnerRefFromLabels(tt.labels)
			if !reflect.DeepEqual(got, tt.wantSoftOwner) {
				t.Errorf("SoftOwnerRefFromLabels() got = %v, want %v", got, tt.wantSoftOwner)
			}
			if got1 != tt.wantReferenced {
				t.Errorf("SoftOwnerRefFromLabels() got1 = %v, want %v", got1, tt.wantReferenced)
			}
		})
	}
}
