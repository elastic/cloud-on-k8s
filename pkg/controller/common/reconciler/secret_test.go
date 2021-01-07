// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"context"
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
			got, err := ReconcileSecret(tt.c, *tt.expected, owner)
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
			got, err := ReconcileSecretNoOwnerRef(tt.c, *tt.expected, tt.softOwner)
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

func Test_hasOwner(t *testing.T) {
	owner := sampleOwner()
	type args struct {
		resource metav1.Object
		owner    metav1.Object
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "owner is referenced (same name and uid)",
			args: args{
				resource: addOwner(&corev1.Secret{}, owner.Name, owner.UID),
				owner:    owner,
			},
			want: true,
		},
		{
			name: "owner referenced among other owner references",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, "another-name", types.UID("another-id")), owner.Name, owner.UID),
				owner:    owner,
			},
			want: true,
		},
		{
			name: "owner not referenced",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, "another-name", types.UID("another-id")), "yet-another-name", "yet-another-uid"),
				owner:    owner,
			},
			want: false,
		},
		{
			name: "no owner ref",
			args: args{
				resource: &corev1.Secret{},
				owner:    owner,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasOwner(tt.args.resource, tt.args.owner); got != tt.want {
				t.Errorf("hasOwner() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_removeOwner(t *testing.T) {
	type args struct {
		resource metav1.Object
		owner    metav1.Object
	}
	tests := []struct {
		name         string
		args         args
		wantResource *corev1.Secret
	}{
		{
			name: "no owner: no-op",
			args: args{
				resource: &corev1.Secret{},
				owner:    sampleOwner(),
			},
			wantResource: &corev1.Secret{},
		},
		{
			name: "different owner: no-op",
			args: args{
				resource: addOwner(&corev1.Secret{}, "another-owner-name", "another-owner-id"),
				owner:    sampleOwner(),
			},
			wantResource: addOwner(&corev1.Secret{}, "another-owner-name", "another-owner-id"),
		},
		{
			name: "remove the single owner",
			args: args{
				resource: addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantResource: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{}}},
		},
		{
			name: "remove the owner from a list of owners",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID), "another-owner", "another-uid"),
				owner:    sampleOwner(),
			},
			wantResource: addOwner(&corev1.Secret{}, "another-owner", "another-uid"),
		},
		{
			name: "owner listed twice in the list (shouldn't happen): remove the first occurrence",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID), sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantResource: addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removeOwner(tt.args.resource, tt.args.owner)
			require.Equal(t, tt.wantResource, tt.args.resource)
		})
	}
}

func Test_findOwner(t *testing.T) {
	type args struct {
		resource metav1.Object
		owner    metav1.Object
	}
	tests := []struct {
		name      string
		args      args
		wantFound bool
		wantIndex int
	}{
		{
			name: "no owner: not found",
			args: args{
				resource: &corev1.Secret{},
				owner:    sampleOwner(),
			},
			wantFound: false,
			wantIndex: 0,
		},
		{
			name: "different owner: not found",
			args: args{
				resource: addOwner(&corev1.Secret{}, "another-owner-name", "another-owner-id"),
				owner:    sampleOwner(),
			},
			wantFound: false,
			wantIndex: 0,
		},
		{
			name: "owner at index 0",
			args: args{
				resource: addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantFound: true,
			wantIndex: 0,
		},
		{
			name: "owner at index 1",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, "another-owner", "another-uid"), sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantFound: true,
			wantIndex: 1,
		},
		{
			name: "owner listed twice in the list (shouldn't happen): return the first occurrence (index 0)",
			args: args{
				resource: addOwner(addOwner(&corev1.Secret{}, sampleOwner().Name, sampleOwner().UID), sampleOwner().Name, sampleOwner().UID),
				owner:    sampleOwner(),
			},
			wantFound: true,
			wantIndex: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFound, gotIndex := findOwner(tt.args.resource, tt.args.owner)
			if gotFound != tt.wantFound {
				t.Errorf("findOwner() gotFound = %v, want %v", gotFound, tt.wantFound)
			}
			if gotIndex != tt.wantIndex {
				t.Errorf("findOwner() gotIndex = %v, want %v", gotIndex, tt.wantIndex)
			}
		})
	}
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
	kind := "Secret"
	tests := []struct {
		name            string
		existingSecrets []runtime.Object
		deletedOwner    types.NamespacedName
		ownerKind       string
		wantObjs        []runtime.Object
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
			existingSecrets: []runtime.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind)},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs:     nil,
		},
		{
			name: "don't gc secret with no owner label",
			existingSecrets: []runtime.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: sampleOwner().Namespace, Name: sampleOwner().Name}}},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []runtime.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: sampleOwner().Namespace, Name: sampleOwner().Name}}},
		},
		{
			name: "don't gc secret pointing to a soft owner with a different name",
			existingSecrets: []runtime.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, "another-name", sampleOwner().Kind)},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []runtime.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, "another-name", sampleOwner().Kind)},
		},
		{
			name: "don't gc secret pointing to a soft owner with a different namespace",
			existingSecrets: []runtime.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind)},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []runtime.Object{
				ownedSecret("ns", "secret-1", "another-namespace", sampleOwner().Name, sampleOwner().Kind)},
		},
		{
			name: "don't gc secret pointing to a soft owner with a different kind",
			existingSecrets: []runtime.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, "another-kind")},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []runtime.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, "another-kind")},
		},
		{
			name: "2 secrets to gc out of 5 secrets",
			existingSecrets: []runtime.Object{
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-3", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-4", sampleOwner().Namespace, "another-owner", sampleOwner().Kind),
				ownedSecret("ns", "secret-5", sampleOwner().Namespace, sampleOwner().Name, "another-kind"),
			},
			deletedOwner: k8s.ExtractNamespacedName(sampleOwner()),
			ownerKind:    "Secret",
			wantObjs: []runtime.Object{
				ownedSecret("ns", "secret-4", sampleOwner().Namespace, "another-owner", sampleOwner().Kind),
				ownedSecret("ns", "secret-5", sampleOwner().Namespace, sampleOwner().Name, "another-kind"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.existingSecrets...)
			err := GarbageCollectSoftOwnedSecrets(c, tt.deletedOwner, kind)
			require.NoError(t, err)
			var retrievedSecrets corev1.SecretList
			err = c.List(context.Background(), &retrievedSecrets)
			require.NoError(t, err)
			require.Equal(t, len(tt.wantObjs), len(retrievedSecrets.Items))
			for i := range tt.wantObjs {
				require.Equal(t, tt.wantObjs[i].(*corev1.Secret).Name, retrievedSecrets.Items[i].Name)
			}
		})
	}
}

func TestGarbageCollectAllSoftOwnedOrphanSecrets(t *testing.T) {
	ownerKinds := map[string]client.Object{
		"Secret": &corev1.Secret{},
	}
	tests := []struct {
		name        string
		runtimeObjs []runtime.Object
		wantObjs    []runtime.Object
		assert      func(c k8s.Client, t *testing.T)
	}{
		{
			name: "nothing to gc",
			runtimeObjs: []runtime.Object{
				// owner exists, 2 owned secrets
				sampleOwner(),
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
			wantObjs: []runtime.Object{
				sampleOwner(),
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
		},
		{
			name: "gc 2 secrets",
			runtimeObjs: []runtime.Object{
				// owner doesn't exist: gc these 2 secrets
				ownedSecret("ns", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
				ownedSecret("ns", "secret-2", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
			wantObjs: []runtime.Object{},
		},
		{
			name: "don't gc secret targeting an owner in a different namespace",
			runtimeObjs: []runtime.Object{
				// secret likely copied manually into another namespace
				ownedSecret("another-namespace", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
			wantObjs: []runtime.Object{
				ownedSecret("another-namespace", "secret-1", sampleOwner().Namespace, sampleOwner().Name, sampleOwner().Kind),
			},
		},
		{
			name: "don't gc resources of a non-managed Kind",
			runtimeObjs: []runtime.Object{
				// configmap whose owner doesn't exist, should not be gc
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "configmap-name", Labels: map[string]string{
					SoftOwnerNameLabel:      "owner-name",
					SoftOwnerNamespaceLabel: "ns",
					SoftOwnerKindLabel:      "ConfigMap",
				}}},
			},
			assert: func(c k8s.Client, t *testing.T) {
				// configmap should still be there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "configmap-name"}, &corev1.ConfigMap{}))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.runtimeObjs...)
			err := GarbageCollectAllSoftOwnedOrphanSecrets(c, ownerKinds)
			require.NoError(t, err)
			var retrievedSecrets corev1.SecretList
			err = c.List(context.Background(), &retrievedSecrets)
			require.NoError(t, err)
			require.Equal(t, len(tt.wantObjs), len(retrievedSecrets.Items))
			for i := range tt.wantObjs {
				require.Equal(t, tt.wantObjs[i].(*corev1.Secret).Name, retrievedSecrets.Items[i].Name)
			}
			if tt.assert != nil {
				tt.assert(c, t)
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
