// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	testResourceSelector = client.MatchingLabels{
		"app":               "elasticsearch",
		ConfigTypeLabelName: ConfigTypeImmutable,
	}
	testRSLabels = client.MatchingLabels{
		"app": "elasticsearch",
	}
)

func secretVolumeClassifier(volumeNames ...string) MapClassifier {
	c := make(MapClassifier, len(volumeNames))
	for _, name := range volumeNames {
		c[name] = Immutable
	}
	return c
}

func configMapVolumeClassifier(volumeNames ...string) MapClassifier {
	return secretVolumeClassifier(volumeNames...)
}

func testRevisions(t *testing.T, c client.Client, owner client.Object) Revisions {
	revisions, err := NewRevisions(c, owner, "default").
		WithConfigResourceSelector(testResourceSelector).
		WithPodTemplateSource(NewReplicaSetExtractor(testRSLabels)).
		Build()
	require.NoError(t, err)
	return revisions
}

func TestSecretRevision_Reconcile(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		owner           *corev1.Secret
		secretToCreate  corev1.Secret
		wantNamePrefix  string
		wantExactName   string
		wantData        map[string][]byte
		wantOwnerRef    bool
	}{
		{
			name:            "creates secret and tracks name",
			existingObjects: nil,
			owner: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default", UID: "uid-1"},
			},
			secretToCreate: BuildImmutableSecret("my-config", "default", map[string][]byte{"key": []byte("val")}, nil),
			wantNamePrefix: "my-config-",
			wantData:       map[string][]byte{"key": []byte("val")},
			wantOwnerRef:   true,
		},
		{
			name: "idempotent on existing secret",
			existingObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "my-config-aabbccdd", Namespace: "default"},
					Data:       map[string][]byte{"key": []byte("existing")},
				},
			},
			owner: nil,
			secretToCreate: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "my-config-aabbccdd", Namespace: "default"},
				Data:       map[string][]byte{"key": []byte("new")},
			},
			wantExactName: "my-config-aabbccdd",
			wantData:      map[string][]byte{"key": []byte("existing")},
			wantOwnerRef:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithRuntimeObjects(tt.existingObjects...).Build()
			var owner client.Object
			if tt.owner != nil {
				owner = tt.owner
			}
			rev := testRevisions(t, k8sClient, owner).ForSecretVolumes(secretVolumeClassifier("config"))

			name, err := rev.Reconcile(ctx, &tt.secretToCreate)
			require.NoError(t, err)

			if tt.wantExactName != "" {
				assert.Equal(t, tt.wantExactName, name)
			} else {
				assert.Contains(t, name, tt.wantNamePrefix)
			}

			var got corev1.Secret
			require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, &got))
			assert.Equal(t, tt.wantData, got.Data)

			if tt.wantOwnerRef {
				require.Len(t, got.OwnerReferences, 1)
				assert.Equal(t, "owner", got.OwnerReferences[0].Name)
			}
			assert.True(t, rev.reconciled.Has(name))
		})
	}

	t.Run("tracks multiple names", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().Build()
		rev := testRevisions(t, k8sClient, nil).ForSecretVolumes(secretVolumeClassifier("config"))

		s1 := BuildImmutableSecret("cfg", "default", map[string][]byte{"a": []byte("1")}, nil)
		s2 := BuildImmutableSecret("cfg", "default", map[string][]byte{"b": []byte("2")}, nil)

		name1, err := rev.Reconcile(ctx, &s1)
		require.NoError(t, err)
		name2, err := rev.Reconcile(ctx, &s2)
		require.NoError(t, err)

		assert.NotEqual(t, name1, name2)
		assert.True(t, rev.reconciled.Has(name1))
		assert.True(t, rev.reconciled.Has(name2))
	})

	t.Run("rejects namespace mismatch", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().Build()
		rev := testRevisions(t, k8sClient, nil).ForSecretVolumes(secretVolumeClassifier("config"))

		secret := BuildImmutableSecret("cfg", "other-namespace", map[string][]byte{"a": []byte("1")}, nil)
		_, err := rev.Reconcile(ctx, &secret)
		require.EqualError(t, err, `object namespace "other-namespace" does not match Revision namespace "default"`)
	})
}

func TestSecretRevision_PatchVolumes(t *testing.T) {
	tests := []struct {
		name           string
		classifier     MapClassifier
		volumes        []corev1.Volume
		newSecretName  string
		wantSecretName []string
	}{
		{
			name:       "single volume",
			classifier: secretVolumeClassifier("config-volume"),
			volumes: []corev1.Volume{
				{
					Name: "config-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "old-config"},
					},
				},
				{
					Name: "other-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "other-secret"},
					},
				},
			},
			newSecretName:  "new-config-a1b2c3d4",
			wantSecretName: []string{"new-config-a1b2c3d4", "other-secret"},
		},
		{
			name:       "multiple volumes",
			classifier: secretVolumeClassifier("config-volume", "jvm-options-volume"),
			volumes: []corev1.Volume{
				{
					Name: "config-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "old-config"},
					},
				},
				{
					Name: "jvm-options-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "old-jvm"},
					},
				},
				{
					Name: "other-volume",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "other-secret"},
					},
				},
			},
			newSecretName:  "new-immutable-a1b2c3d4",
			wantSecretName: []string{"new-immutable-a1b2c3d4", "new-immutable-a1b2c3d4", "other-secret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rev := testRevisions(t, fake.NewClientBuilder().Build(), nil).ForSecretVolumes(tt.classifier)
			rev.PatchVolumes(tt.volumes, tt.newSecretName)

			for i, want := range tt.wantSecretName {
				assert.Equal(t, want, tt.volumes[i].Secret.SecretName)
			}
		})
	}
}

func TestSecretRevision_GC(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{
		"app":               "elasticsearch",
		ConfigTypeLabelName: ConfigTypeImmutable,
	}

	t.Run("deletes unreferenced secrets", func(t *testing.T) {
		staleSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-stale", Namespace: "default", Labels: labels},
		}
		otherSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default", Labels: map[string]string{"x": "y"}},
		}
		k8sClient := fake.NewClientBuilder().WithRuntimeObjects(staleSecret, otherSecret).Build()
		rev := testRevisions(t, k8sClient, nil).ForSecretVolumes(secretVolumeClassifier("config"))

		// Reconcile creates a new secret and protects it
		currentSecret := BuildImmutableSecret("cfg", "default", map[string][]byte{"key": []byte("value")}, labels)
		currentName, err := rev.Reconcile(ctx, &currentSecret)
		require.NoError(t, err)

		require.NoError(t, rev.GC(ctx))

		var s corev1.Secret
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: currentName, Namespace: "default"}, &s), "current should be kept")
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: "other", Namespace: "default"}, &s), "non-matching labels should be kept")
		assert.Error(t, k8sClient.Get(ctx, types.NamespacedName{Name: "cfg-stale", Namespace: "default"}, &s), "stale should be deleted")
	})

	t.Run("protects secrets referenced by ReplicaSets", func(t *testing.T) {
		rsProtectedSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-rs-protected", Namespace: "default", Labels: labels},
		}
		staleSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-stale", Namespace: "default", Labels: labels},
		}
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-rs", Namespace: "default",
				Labels: map[string]string{"app": "elasticsearch"},
			},
			Spec: appsv1.ReplicaSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: "cfg-rs-protected"},
							},
						}},
					},
				},
			},
		}
		k8sClient := fake.NewClientBuilder().WithRuntimeObjects(rsProtectedSecret, staleSecret, rs).Build()
		rev := testRevisions(t, k8sClient, nil).ForSecretVolumes(secretVolumeClassifier("config"))

		// Reconcile creates a new secret and protects it
		currentSecret := BuildImmutableSecret("cfg", "default", map[string][]byte{"key": []byte("value")}, labels)
		currentName, err := rev.Reconcile(ctx, &currentSecret)
		require.NoError(t, err)

		require.NoError(t, rev.GC(ctx))

		var s corev1.Secret
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: currentName, Namespace: "default"}, &s), "current should be kept")
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: "cfg-rs-protected", Namespace: "default"}, &s), "RS-referenced should be kept")
		assert.Error(t, k8sClient.Get(ctx, types.NamespacedName{Name: "cfg-stale", Namespace: "default"}, &s), "stale should be deleted")
	})
}

func TestConfigMapRevision_Reconcile(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		owner           *corev1.ConfigMap
		cmToCreate      corev1.ConfigMap
		wantNamePrefix  string
		wantExactName   string
		wantData        map[string]string
		wantOwnerRef    bool
	}{
		{
			name:            "creates configmap and tracks name",
			existingObjects: nil,
			owner: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "default", UID: "uid-2"},
			},
			cmToCreate:     BuildImmutableConfigMap("my-scripts", "default", map[string]string{"s.sh": "echo hi"}, nil),
			wantNamePrefix: "my-scripts-",
			wantData:       map[string]string{"s.sh": "echo hi"},
			wantOwnerRef:   true,
		},
		{
			name: "idempotent on existing configmap",
			existingObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "scripts-aabbccdd", Namespace: "default"},
					Data:       map[string]string{"s.sh": "existing"},
				},
			},
			owner: nil,
			cmToCreate: corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "scripts-aabbccdd", Namespace: "default"},
				Data:       map[string]string{"s.sh": "new"},
			},
			wantExactName: "scripts-aabbccdd",
			wantData:      map[string]string{"s.sh": "existing"},
			wantOwnerRef:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().WithRuntimeObjects(tt.existingObjects...).Build()
			var owner client.Object
			if tt.owner != nil {
				owner = tt.owner
			}
			rev := testRevisions(t, k8sClient, owner).ForConfigMapVolumes(configMapVolumeClassifier("scripts"))

			name, err := rev.Reconcile(ctx, &tt.cmToCreate)
			require.NoError(t, err)

			if tt.wantExactName != "" {
				assert.Equal(t, tt.wantExactName, name)
			} else {
				assert.Contains(t, name, tt.wantNamePrefix)
			}

			var got corev1.ConfigMap
			require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, &got))
			assert.Equal(t, tt.wantData, got.Data)

			if tt.wantOwnerRef {
				require.Len(t, got.OwnerReferences, 1)
				assert.Equal(t, "owner", got.OwnerReferences[0].Name)
			}
			assert.True(t, rev.reconciled.Has(name))
		})
	}
}

func TestConfigMapRevision_PatchVolumes(t *testing.T) {
	rev := testRevisions(t, fake.NewClientBuilder().Build(), nil).ForConfigMapVolumes(configMapVolumeClassifier("scripts-volume"))

	volumes := []corev1.Volume{
		{
			Name: "scripts-volume",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "old-scripts"},
				},
			},
		},
		{
			Name: "other-volume",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "other-configmap"},
				},
			},
		},
	}

	rev.PatchVolumes(volumes, "new-scripts-a1b2c3d4")

	assert.Equal(t, "new-scripts-a1b2c3d4", volumes[0].ConfigMap.Name)
	assert.Equal(t, "other-configmap", volumes[1].ConfigMap.Name)
}

func TestConfigMapRevision_GC(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{
		"app":               "elasticsearch",
		ConfigTypeLabelName: ConfigTypeImmutable,
	}

	t.Run("deletes unreferenced configmaps", func(t *testing.T) {
		staleConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "scripts-stale", Namespace: "default", Labels: labels},
		}
		k8sClient := fake.NewClientBuilder().WithRuntimeObjects(staleConfigMap).Build()
		rev := testRevisions(t, k8sClient, nil).ForConfigMapVolumes(configMapVolumeClassifier("scripts"))

		// Reconcile creates a new configmap and protects it
		currentCM := BuildImmutableConfigMap("scripts", "default", map[string]string{"key": "value"}, labels)
		currentName, err := rev.Reconcile(ctx, &currentCM)
		require.NoError(t, err)

		require.NoError(t, rev.GC(ctx))

		var cm corev1.ConfigMap
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: currentName, Namespace: "default"}, &cm), "current should be kept")
		assert.Error(t, k8sClient.Get(ctx, types.NamespacedName{Name: "scripts-stale", Namespace: "default"}, &cm), "stale should be deleted")
	})

	t.Run("protects configmaps referenced by ReplicaSets", func(t *testing.T) {
		rsProtectedCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "scripts-rs-protected", Namespace: "default", Labels: labels},
		}
		staleCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "scripts-stale", Namespace: "default", Labels: labels},
		}
		rs := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-rs", Namespace: "default",
				Labels: map[string]string{"app": "elasticsearch"},
			},
			Spec: appsv1.ReplicaSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{
							Name: "scripts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "scripts-rs-protected"},
								},
							},
						}},
					},
				},
			},
		}
		k8sClient := fake.NewClientBuilder().WithRuntimeObjects(rsProtectedCM, staleCM, rs).Build()
		rev := testRevisions(t, k8sClient, nil).ForConfigMapVolumes(configMapVolumeClassifier("scripts"))

		// Reconcile creates a new configmap and protects it
		currentCM := BuildImmutableConfigMap("scripts", "default", map[string]string{"key": "value"}, labels)
		currentName, err := rev.Reconcile(ctx, &currentCM)
		require.NoError(t, err)

		require.NoError(t, rev.GC(ctx))

		var cm corev1.ConfigMap
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: currentName, Namespace: "default"}, &cm), "current should be kept")
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: "scripts-rs-protected", Namespace: "default"}, &cm), "RS-referenced should be kept")
		assert.Error(t, k8sClient.Get(ctx, types.NamespacedName{Name: "scripts-stale", Namespace: "default"}, &cm), "stale should be deleted")
	})
}

func TestGCAll(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{
		"app":               "elasticsearch",
		ConfigTypeLabelName: ConfigTypeImmutable,
	}

	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg-11223344", Namespace: "default", Labels: labels},
	}
	oldCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "scripts-11223344", Namespace: "default", Labels: labels},
	}

	k8sClient := fake.NewClientBuilder().WithObjects(oldSecret, oldCM).Build()
	revs := testRevisions(t, k8sClient, nil)
	secretRev := revs.ForSecretVolumes(secretVolumeClassifier("config"))
	cmRev := revs.ForConfigMapVolumes(configMapVolumeClassifier("scripts"))

	require.NoError(t, GCAll(ctx, secretRev, cmRev))

	var s corev1.Secret
	assert.Error(t, k8sClient.Get(ctx, types.NamespacedName{Name: "cfg-11223344", Namespace: "default"}, &s))
	var cm corev1.ConfigMap
	assert.Error(t, k8sClient.Get(ctx, types.NamespacedName{Name: "scripts-11223344", Namespace: "default"}, &cm))
}

func TestSecretRevision_GC_MultipleVolumes(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{
		"app":               "elasticsearch",
		ConfigTypeLabelName: ConfigTypeImmutable,
	}

	// Secrets protected by ReplicaSets referencing different volumes
	configProtected := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg-config-protected", Namespace: "default", Labels: labels},
	}
	jvmProtected := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg-jvm-protected", Namespace: "default", Labels: labels},
	}
	staleSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cfg-stale", Namespace: "default", Labels: labels},
	}
	rsConfig := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rs-config", Namespace: "default",
			Labels: map[string]string{"app": "elasticsearch"},
		},
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "cfg-config-protected"},
						},
					}},
				},
			},
		},
	}
	rsJvm := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "rs-jvm", Namespace: "default",
			Labels: map[string]string{"app": "elasticsearch"},
		},
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name: "jvm-options",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "cfg-jvm-protected"},
						},
					}},
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithRuntimeObjects(configProtected, jvmProtected, staleSecret, rsConfig, rsJvm).Build()
	rev := testRevisions(t, k8sClient, nil).ForSecretVolumes(secretVolumeClassifier("config", "jvm-options"))

	// Reconcile creates a new secret and protects it
	currentSecret := BuildImmutableSecret("cfg", "default", map[string][]byte{"key": []byte("value")}, labels)
	currentName, err := rev.Reconcile(ctx, &currentSecret)
	require.NoError(t, err)

	require.NoError(t, rev.GC(ctx))

	var s corev1.Secret
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: currentName, Namespace: "default"}, &s), "current should be protected")
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: "cfg-config-protected", Namespace: "default"}, &s), "referenced by config volume should be protected")
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: "cfg-jvm-protected", Namespace: "default"}, &s), "referenced by jvm-options volume should be protected")
	assert.Error(t, k8sClient.Get(ctx, types.NamespacedName{Name: "cfg-stale", Namespace: "default"}, &s), "unreferenced should be deleted")
}

func TestForSecretVolumes_WithMixedClassifier(t *testing.T) {
	classifier := MapClassifier{
		"config-volume":      Immutable,
		"jvm-options-volume": Immutable,
		"dynamic-volume":     Dynamic,
	}

	rev := testRevisions(t, fake.NewClientBuilder().Build(), nil).ForSecretVolumes(classifier)

	volumes := []corev1.Volume{
		{
			Name: "config-volume",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "old-config"},
			},
		},
		{
			Name: "jvm-options-volume",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "old-jvm"},
			},
		},
		{
			Name: "dynamic-volume",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "dynamic-secret"},
			},
		},
	}

	rev.PatchVolumes(volumes, "new-immutable-a1b2c3d4")

	assert.Equal(t, "new-immutable-a1b2c3d4", volumes[0].Secret.SecretName, "config volume should be patched")
	assert.Equal(t, "new-immutable-a1b2c3d4", volumes[1].Secret.SecretName, "jvm-options volume should be patched")
	assert.Equal(t, "dynamic-secret", volumes[2].Secret.SecretName, "dynamic volume should not be patched")
}

func TestRevisionsBuilder_Build(t *testing.T) {
	tests := []struct {
		name    string
		builder RevisionsBuilder
		wantErr string
	}{
		{
			name: "fails when client is missing",
			builder: NewRevisions(nil, nil, "default").
				WithConfigResourceSelector(testResourceSelector).
				WithPodTemplateSource(NewReplicaSetExtractor(testRSLabels)),
			wantErr: "client is required",
		},
		{
			name: "fails when namespace is missing",
			builder: NewRevisions(fake.NewClientBuilder().Build(), nil, "").
				WithConfigResourceSelector(testResourceSelector).
				WithPodTemplateSource(NewReplicaSetExtractor(testRSLabels)),
			wantErr: "namespace is required",
		},
		{
			name: "fails when config resource selector is missing",
			builder: NewRevisions(fake.NewClientBuilder().Build(), nil, "default").
				WithPodTemplateSource(NewReplicaSetExtractor(testRSLabels)),
			wantErr: "config resource selector is required (use WithConfigResourceSelector)",
		},
		{
			name: "fails when config resource selector has wrong immutable label value",
			builder: NewRevisions(fake.NewClientBuilder().Build(), nil, "default").
				WithConfigResourceSelector(client.MatchingLabels{
					"app":               "elasticsearch",
					ConfigTypeLabelName: ConfigTypeDynamic,
				}).
				WithPodTemplateSource(NewReplicaSetExtractor(testRSLabels)),
			wantErr: "config resource selector must include common.k8s.elastic.co/config-type=immutable",
		},
		{
			name: "fails when pod template source is missing",
			builder: NewRevisions(fake.NewClientBuilder().Build(), nil, "default").
				WithConfigResourceSelector(testResourceSelector),
			wantErr: "pod template source is required (use WithPodTemplateSource)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.builder.Build()
			require.EqualError(t, err, tt.wantErr)
		})
	}

	t.Run("succeeds when all required fields are set", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().Build()
		gcLabels := client.MatchingLabels{
			"app":               "elasticsearch",
			ConfigTypeLabelName: ConfigTypeImmutable,
		}
		rsLabels := client.MatchingLabels{
			"app": "elasticsearch",
		}

		revisions, err := NewRevisions(k8sClient, nil, "default").
			WithConfigResourceSelector(gcLabels).
			WithPodTemplateSource(NewReplicaSetExtractor(rsLabels)).
			Build()
		require.NoError(t, err)
		gcLabels["app"] = "mutated"
		assert.Equal(t, "default", revisions.namespace)
		assert.Equal(t, "elasticsearch", revisions.resourceSelector["app"])
	})

	t.Run("adds immutable config-type label when missing", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().Build()
		selectorWithoutType := client.MatchingLabels{
			"app": "elasticsearch",
		}

		revisions, err := NewRevisions(k8sClient, nil, "default").
			WithConfigResourceSelector(selectorWithoutType).
			WithPodTemplateSource(NewReplicaSetExtractor(testRSLabels)).
			Build()
		require.NoError(t, err)
		assert.Equal(t, "elasticsearch", revisions.resourceSelector["app"])
		assert.Equal(t, ConfigTypeImmutable, revisions.resourceSelector[ConfigTypeLabelName])
	})
}

func TestReplicaSetExtractor_ListPodTemplates(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-rs",
			Namespace: "default",
			Labels:    map[string]string{"app": "elasticsearch"},
		},
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: "my-config-def456"},
							},
						},
					},
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithObjects(rs).Build()

	extractor := NewReplicaSetExtractor(client.MatchingLabels{"app": "elasticsearch"})
	templates, err := extractor.ListPodTemplates(context.Background(), k8sClient, "default")
	require.NoError(t, err)
	require.Len(t, templates, 1)
	require.Len(t, templates[0].Spec.Volumes, 1)
	assert.Equal(t, "config", templates[0].Spec.Volumes[0].Name)
	assert.Equal(t, "my-config-def456", templates[0].Spec.Volumes[0].Secret.SecretName)
}
