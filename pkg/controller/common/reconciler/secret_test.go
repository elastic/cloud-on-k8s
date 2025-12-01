// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package reconciler

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
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

func ownedSecretMultiRefs(namespace, name, ownerRefs, ownerKind string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{
			SoftOwnerKindLabel: ownerKind,
		}, Annotations: map[string]string{
			SoftOwnerRefsAnnotation: ownerRefs,
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
		{
			name: "secret with multiple soft-owners that all exist",
			runtimeObjs: []client.Object{
				&policyv1alpha1.StackConfigPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy-1", Namespace: "namespace-1"}},
				&policyv1alpha1.StackConfigPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy-2", Namespace: "namespace-2"}},
				&policyv1alpha1.StackConfigPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy-3", Namespace: "namespace-3"}},
				ownedSecretMultiRefs("ns", "secret-1", `["namespace-1/policy-1","namespace-2/policy-2","namespace-3/policy-3"]`, "StackConfigPolicy"),
			},
			wantObjs: []client.Object{
				ownedSecretMultiRefs("ns", "secret-1", `["namespace-1/policy-1","namespace-2/policy-2","namespace-3/policy-3"]`, "StackConfigPolicy"),
			},
		},
		{
			name: "secret with multiple soft-owners that all exist but some in different namespace",
			runtimeObjs: []client.Object{
				&policyv1alpha1.StackConfigPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy-1", Namespace: "namespace-1"}},
				&policyv1alpha1.StackConfigPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy-2", Namespace: "namespace-other"}},
				&policyv1alpha1.StackConfigPolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy-3", Namespace: "namespace-3"}},
				ownedSecretMultiRefs("ns", "secret-1", `["namespace-1/policy-1","namespace-2/policy-2","namespace-3/policy-3"]`, "StackConfigPolicy"),
			},
			wantObjs: []client.Object{
				ownedSecretMultiRefs("ns", "secret-1", `["namespace-1/policy-1","namespace-2/policy-2","namespace-3/policy-3"]`, "StackConfigPolicy"),
			},
		},
		{
			name: "secret with multiple soft-owners that none exists",
			runtimeObjs: []client.Object{
				ownedSecretMultiRefs("ns", "secret-1", `["namespace-1/policy-1","namespace-2/policy-2","namespace-3/policy-3"]`, "StackConfigPolicy"),
			},
			wantObjs: []client.Object{},
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

//nolint:thelper
func TestSoftOwnerRefs(t *testing.T) {
	tests := []struct {
		name     string
		secret   *corev1.Secret
		validate func(t *testing.T, owners []SoftOwnerRef, err error)
	}{
		{
			name: "returns multi-owner policies from annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `["namespace-1/policy-1","namespace-2/policy-2"]`,
					},
				},
			},
			validate: func(t *testing.T, owners []SoftOwnerRef, err error) {
				require.NoError(t, err)
				require.Len(t, owners, 2)
				assert.Contains(t, owners, SoftOwnerRef{Name: "policy-1", Namespace: "namespace-1", Kind: policyv1alpha1.Kind})
				assert.Contains(t, owners, SoftOwnerRef{Name: "policy-2", Namespace: "namespace-2", Kind: policyv1alpha1.Kind})
			},
		},
		{
			name: "returns single-owner policy from labels",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						SoftOwnerKindLabel:      policyv1alpha1.Kind,
						SoftOwnerNameLabel:      "single-policy",
						SoftOwnerNamespaceLabel: "single-namespace",
					},
				},
			},
			validate: func(t *testing.T, owners []SoftOwnerRef, err error) {
				require.NoError(t, err)
				require.Len(t, owners, 1)
				assert.Equal(t, SoftOwnerRef{Name: "single-policy", Namespace: "single-namespace", Kind: policyv1alpha1.Kind}, owners[0])
			},
		},
		{
			name: "returns nil when secret has kind label but no owner labels",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						SoftOwnerKindLabel: policyv1alpha1.Kind,
						"other-label":      "other-value",
					},
				},
			},
			validate: func(t *testing.T, owners []SoftOwnerRef, err error) {
				require.NoError(t, err)
				assert.Nil(t, owners)
			},
		},
		{
			name: "returns nil for non-policy-owned secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"some-other-label": "some-value",
					},
				},
			},
			validate: func(t *testing.T, owners []SoftOwnerRef, err error) {
				require.NoError(t, err)
				assert.Nil(t, owners)
			},
		},
		{
			name: "returns error for invalid JSON in annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `invalid-json`,
					},
				},
			},
			validate: func(t *testing.T, owners []SoftOwnerRef, err error) {
				require.Error(t, err)
				assert.Nil(t, owners)
			},
		},
		{
			name: "skips malformed namespaced names in annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
					Labels: map[string]string{
						SoftOwnerKindLabel: policyv1alpha1.Kind,
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `["namespace-1/policy-1","malformed","too/many/slashes"]`,
					},
				},
			},
			validate: func(t *testing.T, owners []SoftOwnerRef, err error) {
				require.NoError(t, err)
				require.Len(t, owners, 1)
				assert.Equal(t, SoftOwnerRef{Name: "policy-1", Namespace: "namespace-1", Kind: policyv1alpha1.Kind}, owners[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owners, err := SoftOwnerRefs(tt.secret)
			tt.validate(t, owners, err)
		})
	}
}

//nolint:thelper
func TestSetSingleSoftOwner(t *testing.T) {
	tests := []struct {
		name     string
		obj      *corev1.Secret
		owner    SoftOwnerRef
		validate func(t *testing.T, obj *corev1.Secret)
	}{
		{
			name: "sets soft owner labels on empty object",
			obj:  &corev1.Secret{},
			owner: SoftOwnerRef{
				Namespace: "test-namespace",
				Name:      "test-owner",
				Kind:      "TestKind",
			},
			validate: func(t *testing.T, obj *corev1.Secret) {
				assert.Equal(t, "TestKind", obj.Labels[SoftOwnerKindLabel])
				assert.Equal(t, "test-owner", obj.Labels[SoftOwnerNameLabel])
				assert.Equal(t, "test-namespace", obj.Labels[SoftOwnerNamespaceLabel])
			},
		},
		{
			name: "overwrites existing soft owner labels",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel:      "OldKind",
						SoftOwnerNameLabel:      "old-owner",
						SoftOwnerNamespaceLabel: "old-namespace",
						"existing-label":        "existing-value",
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `["old/owner"]`,
						"existing-annotation":   "existing-value",
					},
				},
			},
			owner: SoftOwnerRef{
				Namespace: "new-namespace",
				Name:      "new-owner",
				Kind:      "NewKind",
			},
			validate: func(t *testing.T, obj *corev1.Secret) {
				assert.Equal(t, "NewKind", obj.Labels[SoftOwnerKindLabel])
				assert.Equal(t, "new-owner", obj.Labels[SoftOwnerNameLabel])
				assert.Equal(t, "new-namespace", obj.Labels[SoftOwnerNamespaceLabel])
				assert.Equal(t, "existing-value", obj.Labels["existing-label"])
				assert.NotContains(t, obj.Annotations, SoftOwnerRefsAnnotation)
				assert.Equal(t, "existing-value", obj.Annotations["existing-annotation"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetSingleSoftOwner(tt.obj, tt.owner)
			tt.validate(t, tt.obj)
		})
	}
}

//nolint:thelper
func TestSetMultipleSoftOwners(t *testing.T) {
	tests := []struct {
		name      string
		obj       *corev1.Secret
		ownerKind string
		owners    []types.NamespacedName
		validate  func(t *testing.T, obj *corev1.Secret, err error)
	}{
		{
			name:      "sets multiple owners on empty object",
			obj:       &corev1.Secret{},
			ownerKind: "TestKind",
			owners: []types.NamespacedName{
				{Namespace: "ns1", Name: "owner1"},
				{Namespace: "ns2", Name: "owner2"},
			},
			validate: func(t *testing.T, obj *corev1.Secret, err error) {
				require.NoError(t, err)
				assert.Equal(t, "TestKind", obj.Labels[SoftOwnerKindLabel])
				assert.NotContains(t, obj.Labels, SoftOwnerNameLabel)
				assert.NotContains(t, obj.Labels, SoftOwnerNamespaceLabel)

				var ownerRefs []string
				err = json.Unmarshal([]byte(obj.Annotations[SoftOwnerRefsAnnotation]), &ownerRefs)
				require.NoError(t, err)
				assert.ElementsMatch(t, []string{"ns1/owner1", "ns2/owner2"}, ownerRefs)
			},
		},
		{
			name: "removes single-owner labels when setting multiple owners",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel:      "TestKind",
						SoftOwnerNameLabel:      "old-owner",
						SoftOwnerNamespaceLabel: "old-namespace",
					},
				},
			},
			ownerKind: "TestKind",
			owners: []types.NamespacedName{
				{Namespace: "ns1", Name: "owner1"},
			},
			validate: func(t *testing.T, obj *corev1.Secret, err error) {
				require.NoError(t, err)
				assert.NotContains(t, obj.Labels, SoftOwnerNameLabel)
				assert.NotContains(t, obj.Labels, SoftOwnerNamespaceLabel)
			},
		},
		{
			name:      "deduplicates owners",
			obj:       &corev1.Secret{},
			ownerKind: "TestKind",
			owners: []types.NamespacedName{
				{Namespace: "ns1", Name: "owner1"},
				{Namespace: "ns1", Name: "owner1"}, // duplicate
				{Namespace: "ns2", Name: "owner2"},
			},
			validate: func(t *testing.T, obj *corev1.Secret, err error) {
				require.NoError(t, err)
				var ownerRefs []string
				err = json.Unmarshal([]byte(obj.Annotations[SoftOwnerRefsAnnotation]), &ownerRefs)
				require.NoError(t, err)
				assert.Len(t, ownerRefs, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetMultipleSoftOwners(tt.obj, tt.ownerKind, tt.owners)
			tt.validate(t, tt.obj, err)
		})
	}
}

//nolint:thelper
func TestRemoveSoftOwner(t *testing.T) {
	tests := []struct {
		name      string
		obj       *corev1.Secret
		owner     types.NamespacedName
		wantCount int
		wantErr   bool
		validate  func(t *testing.T, obj *corev1.Secret)
	}{
		{
			name: "removes owner from multi-owner with remaining owners",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel: "TestKind",
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `["ns1/owner1","ns2/owner2","ns3/owner3"]`,
					},
				},
			},
			owner:     types.NamespacedName{Namespace: "ns2", Name: "owner2"},
			wantCount: 2,
			validate: func(t *testing.T, obj *corev1.Secret) {
				var ownerRefs []string
				err := json.Unmarshal([]byte(obj.Annotations[SoftOwnerRefsAnnotation]), &ownerRefs)
				require.NoError(t, err)
				assert.ElementsMatch(t, []string{"ns1/owner1", "ns3/owner3"}, ownerRefs)
				assert.Equal(t, "TestKind", obj.Labels[SoftOwnerKindLabel])
			},
		},
		{
			name: "removes last owner and cleans up annotation",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel: "TestKind",
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `["ns1/owner1"]`,
						"other-annotation":      "preserved",
					},
				},
			},
			owner:     types.NamespacedName{Namespace: "ns1", Name: "owner1"},
			wantCount: 0,
			validate: func(t *testing.T, obj *corev1.Secret) {
				assert.NotContains(t, obj.Annotations, SoftOwnerRefsAnnotation)
				assert.Equal(t, "preserved", obj.Annotations["other-annotation"])
			},
		},
		{
			name: "removes matching single-owner",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel:      "TestKind",
						SoftOwnerNameLabel:      "single-owner",
						SoftOwnerNamespaceLabel: "single-namespace",
						"other-label":           "preserved",
					},
				},
			},
			owner:     types.NamespacedName{Namespace: "single-namespace", Name: "single-owner"},
			wantCount: 0,
			validate: func(t *testing.T, obj *corev1.Secret) {
				assert.NotContains(t, obj.Labels, SoftOwnerNameLabel)
				assert.NotContains(t, obj.Labels, SoftOwnerNamespaceLabel)
				assert.Equal(t, "preserved", obj.Labels["other-label"])
			},
		},
		{
			name: "returns 1 when owner doesn't match single-owner",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel:      "TestKind",
						SoftOwnerNameLabel:      "existing-owner",
						SoftOwnerNamespaceLabel: "existing-namespace",
					},
				},
			},
			owner:     types.NamespacedName{Namespace: "different-namespace", Name: "different-owner"},
			wantCount: 1,
			validate: func(t *testing.T, obj *corev1.Secret) {
				assert.Equal(t, "existing-owner", obj.Labels[SoftOwnerNameLabel])
				assert.Equal(t, "existing-namespace", obj.Labels[SoftOwnerNamespaceLabel])
				assert.Equal(t, "TestKind", obj.Labels[SoftOwnerKindLabel])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := RemoveSoftOwner(tt.obj, tt.owner)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCount, count)
			if tt.validate != nil {
				tt.validate(t, tt.obj)
			}
		})
	}
}

//nolint:thelper
func TestIsSoftOwnedBy(t *testing.T) {
	tests := []struct {
		name      string
		obj       *corev1.Secret
		ownerKind string
		owner     types.NamespacedName
		want      bool
		wantErr   bool
	}{
		{
			name: "returns true for multi-owner match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel: "TestKind",
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `["ns1/owner1","ns2/owner2"]`,
					},
				},
			},
			ownerKind: "TestKind",
			owner:     types.NamespacedName{Namespace: "ns1", Name: "owner1"},
			want:      true,
		},
		{
			name: "returns false for multi-owner non-match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel: "TestKind",
					},
					Annotations: map[string]string{
						SoftOwnerRefsAnnotation: `["ns1/owner1","ns2/owner2"]`,
					},
				},
			},
			ownerKind: "TestKind",
			owner:     types.NamespacedName{Namespace: "ns3", Name: "owner3"},
			want:      false,
		},
		{
			name: "returns true for single-owner match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel:      "TestKind",
						SoftOwnerNameLabel:      "single-owner",
						SoftOwnerNamespaceLabel: "single-namespace",
					},
				},
			},
			ownerKind: "TestKind",
			owner:     types.NamespacedName{Namespace: "single-namespace", Name: "single-owner"},
			want:      true,
		},
		{
			name: "returns false for single-owner non-match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel:      "TestKind",
						SoftOwnerNameLabel:      "single-owner",
						SoftOwnerNamespaceLabel: "single-namespace",
					},
				},
			},
			ownerKind: "TestKind",
			owner:     types.NamespacedName{Namespace: "different-namespace", Name: "different-owner"},
			want:      false,
		},
		{
			name: "returns false for wrong kind",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						SoftOwnerKindLabel:      "DifferentKind",
						SoftOwnerNameLabel:      "owner",
						SoftOwnerNamespaceLabel: "namespace",
					},
				},
			},
			ownerKind: "TestKind",
			owner:     types.NamespacedName{Namespace: "namespace", Name: "owner"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsSoftOwnedBy(tt.obj, tt.ownerKind, tt.owner)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
