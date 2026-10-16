// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ---- helpers ----------------------------------------------------------------

// kbWithBG returns a minimal Kibana at version 8.17.0 with spec.backgroundTasks set.
func kbWithBG(count *int32, overlay *commonv1.Config) *kbv1.Kibana {
	kb := &kbv1.Kibana{
		Name: "test", Namespace: "default",
		Spec: kbv1.KibanaSpec{
			Version: "8.17.0",
			Count:   3,
			BackgroundTasks: &kbv1.KibanaBackgroundTasks{
				Count:  count,
				Config: overlay,
			},
		},
	}
	return kb
}

func newTestDriver(t *testing.T, objects ...client.Object) *driver {
	t.Helper()
	kb := &kbv1.Kibana{
		Name: "test", Namespace: "default",
		Spec: kbv1.KibanaSpec{Version: "8.17.0"},
	}
	d, err := newDriver(k8s.NewFakeClient(objects...), watches.NewDynamicWatches(), toolsevents.NewFakeRecorder(10), kb, corev1.IPv4Protocol)
	require.NoError(t, err)
	return d
}

// ---- poolParams -------------------------------------------------------------

func TestPoolParams(t *testing.T) {
	overlay := &commonv1.Config{Data: map[string]any{"xpack.extra": "v"}}

	tests := []struct {
		name               string
		kb                 *kbv1.Kibana
		role               kblabel.Role
		wantSecretName     string
		wantDeploymentName string
		wantConfig         map[string]string // key -> expected string value in the resulting config
		wantAbsentKeys     []string          // keys that must not be in the resulting config
	}{
		{
			name:               "empty role: base secret and deployment names",
			kb:                 &kbv1.Kibana{Name: "mykb", Spec: kbv1.KibanaSpec{Version: "8.17.0"}},
			role:               kblabel.Role{},
			wantSecretName:     kbv1.ConfigSecret("mykb"),
			wantDeploymentName: kbv1.KBNamer.Suffix("mykb"),
		},
		{
			name:               "UIRole: base secret and deployment names",
			kb:                 kbWithBG(nil, nil),
			role:               kblabel.UIRole,
			wantSecretName:     kbv1.ConfigSecret("test"),
			wantDeploymentName: kbv1.KBNamer.Suffix("test"),
		},
		{
			name:               "BackgroundTasksRole, no overlay: shares base secret",
			kb:                 kbWithBG(nil, nil),
			role:               kblabel.BackgroundTasksRole,
			wantSecretName:     kbv1.ConfigSecret("test"),
			wantDeploymentName: kbv1.BackgroundTasksDeployment("test"),
		},
		{
			name:               "BackgroundTasksRole, overlay: dedicated BG secret",
			kb:                 kbWithBG(nil, overlay),
			role:               kblabel.BackgroundTasksRole,
			wantSecretName:     kbv1.BackgroundTasksConfigSecret("test"),
			wantDeploymentName: kbv1.BackgroundTasksDeployment("test"),
		},
		{
			name:               "BackgroundTasksRole, overlay: config contains overlay key",
			kb:                 kbWithBG(nil, overlay),
			role:               kblabel.BackgroundTasksRole,
			wantSecretName:     kbv1.BackgroundTasksConfigSecret("test"),
			wantDeploymentName: kbv1.BackgroundTasksDeployment("test"),
			wantConfig:         map[string]string{"xpack.extra": "v"},
		},
		{
			name:               "UIRole, primary config: primary values returned, BG overlay does not leak",
			kb:                 kbWithPrimary(kbWithBG(nil, overlay), map[string]any{"server.name": "primary", "logging.root.level": "info"}),
			role:               kblabel.UIRole,
			wantSecretName:     kbv1.ConfigSecret("test"),
			wantDeploymentName: kbv1.KBNamer.Suffix("test"),
			wantConfig:         map[string]string{"server.name": "primary", "logging.root.level": "info"},
			wantAbsentKeys:     []string{"xpack.extra"},
		},
		{
			name:               "BackgroundTasksRole, primary config, no overlay: inherits primary values",
			kb:                 kbWithPrimary(kbWithBG(nil, nil), map[string]any{"server.name": "primary"}),
			role:               kblabel.BackgroundTasksRole,
			wantSecretName:     kbv1.ConfigSecret("test"),
			wantDeploymentName: kbv1.BackgroundTasksDeployment("test"),
			wantConfig:         map[string]string{"server.name": "primary"},
		},
		{
			name: "BackgroundTasksRole, primary and overlay: overlay overwrites shared key, keeps the rest",
			kb: kbWithPrimary(
				kbWithBG(nil, &commonv1.Config{Data: map[string]any{"server.name": "bg", "xpack.extra": "v"}}),
				map[string]any{"server.name": "primary", "logging.root.level": "info"},
			),
			role:               kblabel.BackgroundTasksRole,
			wantSecretName:     kbv1.BackgroundTasksConfigSecret("test"),
			wantDeploymentName: kbv1.BackgroundTasksDeployment("test"),
			wantConfig: map[string]string{
				"server.name":        "bg",   // overwritten by overlay
				"logging.root.level": "info", // inherited from primary
				"xpack.extra":        "v",    // added by overlay
			},
		},
		{
			name: "UIRole, primary and overlay: primary is untouched by overlay",
			kb: kbWithPrimary(
				kbWithBG(nil, &commonv1.Config{Data: map[string]any{"server.name": "bg"}}),
				map[string]any{"server.name": "primary"},
			),
			role:               kblabel.UIRole,
			wantSecretName:     kbv1.ConfigSecret("test"),
			wantDeploymentName: kbv1.KBNamer.Suffix("test"),
			wantConfig:         map[string]string{"server.name": "primary"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opParams := operator.Parameters{OperatorNamespace: "op-ns", SetDefaultSecurityContext: true}
			params, err := newPoolParameters(opParams, tt.kb, tt.role, "/base-path", true)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSecretName, params.SecretName)
			assert.Equal(t, tt.wantDeploymentName, params.DeploymentName)
			for k, want := range tt.wantConfig {
				got, err := params.Config.String(k)
				require.NoError(t, err, "key %q", k)
				assert.Equal(t, want, got, "key %q", k)
			}
			for _, k := range tt.wantAbsentKeys {
				_, err := params.Config.String(k)
				assert.Error(t, err, "key %q must be absent", k)
			}
			// Passthrough fields must be propagated verbatim.
			assert.Equal(t, tt.role, params.Role)
			assert.Equal(t, "/base-path", params.BasePath)
			assert.True(t, params.ScaleToZero)
			assert.Equal(t, opParams.SetDefaultSecurityContext, params.SetDefaultSecurityContext)
			assert.Equal(t, opParams.OperatorNamespace, params.OperatorNamespace)
		})
	}
}

// kbWithPrimary sets spec.config (the primary pool config) on kb and returns it.
func kbWithPrimary(kb *kbv1.Kibana, data map[string]any) *kbv1.Kibana {
	kb.Spec.Config = &commonv1.Config{Data: data}
	return kb
}

// ---- mergePoolPodTemplate ---------------------------------------------------

func TestMergePoolPodTemplate(t *testing.T) {
	base := corev1.PodTemplateSpec{
		Labels:      map[string]string{"base-label": "base"},
		Annotations: map[string]string{"base-ann": "base"},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"base-node": "sel"},
			Tolerations:  []corev1.Toleration{{Key: "base-tol"}},
			Containers: []corev1.Container{
				{Name: "kibana", Image: "base-image", Command: []string{"base"}},
				{Name: "sidecar", Image: "sidecar-image"},
			},
			Volumes: []corev1.Volume{
				{Name: "vol-a", EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
		},
	}

	t.Run("empty overlay returns base unchanged", func(t *testing.T) {
		got, err := mergePoolPodTemplate(base, corev1.PodTemplateSpec{})
		require.NoError(t, err)
		assert.Equal(t, base, got)
	})

	t.Run("nodeSelector from overlay merge with base", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"pool-node": "sel"}}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"base-node": "sel", "pool-node": "sel"}, got.Spec.NodeSelector)
		// Base toleration still intact.
		assert.Equal(t, base.Spec.Tolerations, got.Spec.Tolerations)
	})

	t.Run("tolerations from overlay replace base", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "pool-tol"}}}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		require.Len(t, got.Spec.Tolerations, 1)
		assert.Equal(t, "pool-tol", got.Spec.Tolerations[0].Key)
		// Base nodeSelector still intact.
		assert.Equal(t, base.Spec.NodeSelector, got.Spec.NodeSelector)
	})

	t.Run("affinity from overlay wins; base fields not in pool survive", func(t *testing.T) {
		aff := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: aff}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Equal(t, aff, got.Spec.Affinity)
		assert.Equal(t, base.Spec.NodeSelector, got.Spec.NodeSelector)
	})

	t.Run("labels are merged not replaced", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Labels: map[string]string{"pool-label": "pool"}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Equal(t, "base", got.Labels["base-label"], "base label must survive")
		assert.Equal(t, "pool", got.Labels["pool-label"], "pool label must be added")
	})

	t.Run("annotations are merged not replaced", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Annotations: map[string]string{"pool-ann": "pool"}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Equal(t, "base", got.Annotations["base-ann"], "base annotation must survive")
		assert.Equal(t, "pool", got.Annotations["pool-ann"], "pool annotation must be added")
	})

	t.Run("overlay label overwrites base label with same key", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Labels: map[string]string{"base-label": "overridden"}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Equal(t, "overridden", got.Labels["base-label"])
	})

	t.Run("container with same name replaces base container", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "kibana", Image: "pool-image", Command: []string{"pool"}}},
		}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		require.Len(t, got.Spec.Containers, 2, "sidecar must be preserved")
		var kib corev1.Container
		for _, c := range got.Spec.Containers {
			if c.Name == "kibana" {
				kib = c
			}
		}
		assert.Equal(t, "pool-image", kib.Image)
		assert.Equal(t, []string{"pool"}, kib.Command)
	})

	t.Run("novel container in pool is appended", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "extra", Image: "extra-image"}},
		}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Len(t, got.Spec.Containers, 3)
		names := make([]string, 0, 3)
		for _, c := range got.Spec.Containers {
			names = append(names, c.Name)
		}
		assert.Contains(t, names, "extra")
		assert.Contains(t, names, "kibana")
		assert.Contains(t, names, "sidecar")
	})

	t.Run("volume with same name replaces base volume", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{Name: "vol-a", HostPath: &corev1.HostPathVolumeSource{Path: "/new"}}},
		}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		require.Len(t, got.Spec.Volumes, 1)
		assert.NotNil(t, got.Spec.Volumes[0].HostPath)
		assert.Equal(t, "/new", got.Spec.Volumes[0].HostPath.Path)
	})

	t.Run("novel volume in pool is appended", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{Name: "vol-b", EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		}}
		got, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Len(t, got.Spec.Volumes, 2)
	})

	t.Run("base is not mutated", func(t *testing.T) {
		original := base.DeepCopy()
		pool := corev1.PodTemplateSpec{
			Labels: map[string]string{"x": "y"},
			Spec:   corev1.PodSpec{NodeSelector: map[string]string{"x": "y"}},
		}
		_, err := mergePoolPodTemplate(base, pool)
		require.NoError(t, err)
		assert.Equal(t, *original, base)
	})
}

// ---- GC methods -------------------------------------------------------------

func TestGarbageCollectBackgroundResources(t *testing.T) {
	bgDpName := kbv1.BackgroundTasksDeployment("test")
	bgSecretName := kbv1.BackgroundTasksConfigSecret("test")

	existingBGDeployment := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			Name: bgDpName, Namespace: "default",
		}
	}
	existingBGSecret := func() *corev1.Secret {
		return &corev1.Secret{
			Name: bgSecretName, Namespace: "default",
		}
	}

	kb := &kbv1.Kibana{Name: "test", Namespace: "default"}

	t.Run("both resources deleted when they exist", func(t *testing.T) {
		fakeClient := k8s.NewFakeClient(existingBGDeployment(), existingBGSecret())
		d := &driver{client: fakeClient}
		err := d.garbageCollectBackgroundResources(context.Background(), kb)
		require.NoError(t, err)

		var dp appsv1.Deployment
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGDeployment()), &dp)))
		var sec corev1.Secret
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGSecret()), &sec)))
	})

	t.Run("no error when neither resource exists", func(t *testing.T) {
		d := &driver{client: k8s.NewFakeClient()}
		err := d.garbageCollectBackgroundResources(context.Background(), kb)
		require.NoError(t, err)
	})

	t.Run("deployment deleted but secret already gone", func(t *testing.T) {
		fakeClient := k8s.NewFakeClient(existingBGDeployment())
		d := &driver{client: fakeClient}
		err := d.garbageCollectBackgroundResources(context.Background(), kb)
		require.NoError(t, err)

		var dp appsv1.Deployment
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGDeployment()), &dp)))
	})
}

func TestGarbageCollectBGConfigSecret(t *testing.T) {
	bgSecretName := kbv1.BackgroundTasksConfigSecret("test")
	kb := &kbv1.Kibana{Name: "test", Namespace: "default"}

	t.Run("deletes BG config secret when it exists", func(t *testing.T) {
		sec := &corev1.Secret{Name: bgSecretName, Namespace: "default"}
		fakeClient := k8s.NewFakeClient(sec)
		d := &driver{client: fakeClient}
		require.NoError(t, d.garbageCollectBGConfigSecret(context.Background(), kb))

		var got corev1.Secret
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(sec), &got)))
	})

	t.Run("no error when secret does not exist", func(t *testing.T) {
		d := &driver{client: k8s.NewFakeClient()}
		require.NoError(t, d.garbageCollectBGConfigSecret(context.Background(), kb))
	})
}

// ---- Pause-guard paths ------------------------------------------------------

func TestDeploymentSelector(t *testing.T) {
	kbNoSplit := &kbv1.Kibana{Name: "test", Spec: kbv1.KibanaSpec{Version: "8.17.0"}}
	kbSplit := kbWithBG(nil, nil)

	fullSelector := map[string]string{
		commonv1.TypeLabelName:      "kibana",
		kblabel.KibanaNameLabelName: "test",
		kblabel.RoleLabelName:       kblabel.RolePrimaryValue,
	}
	legacySelector := map[string]string{
		commonv1.TypeLabelName:      "kibana",
		kblabel.KibanaNameLabelName: "test",
	}
	bgSelector := map[string]string{
		commonv1.TypeLabelName:      "kibana",
		kblabel.KibanaNameLabelName: "test",
		kblabel.RoleLabelName:       kblabel.RoleBackgroundTasksValue,
	}

	tests := []struct {
		name     string
		kb       *kbv1.Kibana
		role     kblabel.Role
		existing map[string]string
		want     map[string]string
	}{
		{
			name:     "no existing deployment: full selector with role label",
			kb:       kbNoSplit,
			role:     kblabel.SinglePoolRole,
			existing: nil,
			want:     fullSelector,
		},
		{
			name:     "split off, existing without role label: role stripped (ECK upgrade path)",
			kb:       kbNoSplit,
			role:     kblabel.SinglePoolRole,
			existing: legacySelector,
			want:     legacySelector,
		},
		{
			name:     "split off, existing already has role=primary: full selector kept",
			kb:       kbNoSplit,
			role:     kblabel.SinglePoolRole,
			existing: fullSelector,
			want:     fullSelector,
		},
		{
			name:     "split on, existing without role label: role kept (exception does not apply)",
			kb:       kbSplit,
			role:     kblabel.UIRole,
			existing: legacySelector,
			want:     fullSelector,
		},
		{
			name:     "split on, BG role: background_tasks selector regardless of existing",
			kb:       kbSplit,
			role:     kblabel.BackgroundTasksRole,
			existing: legacySelector,
			want:     bgSelector,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deploymentSelector(tt.kb, tt.role, tt.existing))
		})
	}
}

// ---- reconcilePoolDeployment --------------------------------------------------

func TestReconcilePoolDeployment_SelectorMismatch(t *testing.T) {
	dpName := kbv1.KBNamer.Suffix("test")
	legacyLabels := map[string]string{
		kblabel.KibanaNameLabelName: "test",
		commonv1.TypeLabelName:      "kibana",
	}
	legacyDp := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			Name: dpName, Namespace: "default",
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: legacyLabels},
			},
		}
	}

	// Split enabled: the expected UI selector includes role=primary while the existing
	// deployment has no role label, so a selector mismatch must be detected.
	kbPaused := kbWithBG(nil, nil)
	kbPaused.Annotations = map[string]string{commonv1.PauseOrchestrationAnnotation: "true"}
	kbActive := kbWithBG(nil, nil)

	// a complete driver is needed as the deployment is reconciled when it is not deleted
	newDriverFor := func(t *testing.T, c k8s.Client) *driver {
		t.Helper()
		d, err := newDriver(c, watches.NewDynamicWatches(), toolsevents.NewFakeRecorder(100), kbActive, corev1.IPv4Protocol)
		require.NoError(t, err)
		return d
	}

	poolParamsFor := func(kb *kbv1.Kibana) poolParameters {
		pp, err := newPoolParameters(operator.Parameters{}, kb, kblabel.UIRole, "", false)
		require.NoError(t, err)
		return pp
	}
	meta := metadata.Metadata{Labels: kbActive.GetIdentityLabels()}

	t.Run("paused: mismatch detected, deployment not deleted, Defer returned", func(t *testing.T) {
		dp := legacyDp()
		fakeClient := k8s.NewFakeClient(append(defaultInitialObjects(), dp)...)
		d := newDriverFor(t, fakeClient)

		res, reconciledDp, err := d.reconcilePoolDeployment(context.Background(), kbPaused, poolParamsFor(kbPaused), nil, meta, map[string]struct{}{})

		require.NoError(t, err)
		assert.Equal(t, paused, res)
		assert.Nil(t, reconciledDp)
		var got appsv1.Deployment
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))
	})

	// markSteady simulates the Deployment controller having observed the spec and made all
	// replicas available, which deployment.IsSteady requires before the deletion can happen.
	markSteady := func(t *testing.T, c k8s.Client, dp *appsv1.Deployment) {
		t.Helper()
		var got appsv1.Deployment
		require.NoError(t, c.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))
		replicas := ptr.Deref(got.Spec.Replicas, 1)
		got.Status = appsv1.DeploymentStatus{
			ObservedGeneration: got.Generation,
			Replicas:           replicas,
			UpdatedReplicas:    replicas,
			AvailableReplicas:  replicas,
		}
		require.NoError(t, c.Status().Update(context.Background(), &got))
	}

	t.Run("not paused: mismatch triggers deletion and Requeue", func(t *testing.T) {
		dp := legacyDp()
		fakeClient := k8s.NewFakeClient(append(defaultInitialObjects(), dp)...)
		d := newDriverFor(t, fakeClient)

		// First call: the deployment is reconciled with revisionHistoryLimit 0 (to clean up the old ReplicaSets)
		// and its existing selector. It survives.
		res, reconciledDp, err := d.reconcilePoolDeployment(context.Background(), kbActive, poolParamsFor(kbActive), nil, meta, map[string]struct{}{})
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.NotNil(t, reconciledDp)
		var got appsv1.Deployment
		require.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))
		assert.True(t, hasZeroRevisionHistoryLimit(&got))
		assert.Equal(t, legacyLabels, got.Spec.Selector.MatchLabels)
		markSteady(t, fakeClient, dp)

		// Second call: the deployment is deleted.
		res, reconciledDp, err = d.reconcilePoolDeployment(context.Background(), kbActive, poolParamsFor(kbActive), nil, meta, map[string]struct{}{})
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.Nil(t, reconciledDp)
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got)))
	})

	t.Run("resume after pause: deletion happens on the calls after", func(t *testing.T) {
		dp := legacyDp()
		fakeClient := k8s.NewFakeClient(append(defaultInitialObjects(), dp)...)
		d := newDriverFor(t, fakeClient)

		// First call: paused — deployment survives.
		res, _, err := d.reconcilePoolDeployment(context.Background(), kbPaused, poolParamsFor(kbPaused), nil, meta, map[string]struct{}{})
		require.NoError(t, err)
		assert.Equal(t, paused, res)
		var got appsv1.Deployment
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))

		// Second call: not paused — revisionHistoryLimit set to 0, deployment still there.
		res, _, err = d.reconcilePoolDeployment(context.Background(), kbActive, poolParamsFor(kbActive), nil, meta, map[string]struct{}{})
		require.NoError(t, err)
		markSteady(t, fakeClient, dp)
		assert.Equal(t, requeue, res)
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))

		// Third call: deployment deleted.
		res, _, err = d.reconcilePoolDeployment(context.Background(), kbActive, poolParamsFor(kbActive), nil, meta, map[string]struct{}{})
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got)))
	})

	// reconcileOnce runs one reconciliation for the given existing deployment and returns what is left in the cluster.
	reconcileOnce := func(t *testing.T, kb *kbv1.Kibana, objects ...client.Object) (poolDeploymentResult, *appsv1.Deployment, appsv1.Deployment, error) {
		t.Helper()
		fakeClient := k8s.NewFakeClient(append(defaultInitialObjects(), objects...)...)
		d := newDriverFor(t, fakeClient)
		res, reconciledDp, err := d.reconcilePoolDeployment(context.Background(), kb, poolParamsFor(kb), nil, meta, map[string]struct{}{})
		var got appsv1.Deployment
		getErr := fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: dpName}, &got)
		if getErr != nil {
			require.True(t, apierrors.IsNotFound(getErr), "unexpected error: %v", getErr)
			got = appsv1.Deployment{}
		}
		return res, reconciledDp, got, err
	}
	labeledPod := func() *corev1.Pod {
		podLabels := maps.Clone(legacyLabels)
		podLabels[kblabel.RoleLabelName] = kblabel.RolePrimaryValue
		return &corev1.Pod{Name: "labeled", Namespace: "default", Labels: podLabels}
	}
	unlabeledPod := func() *corev1.Pod {
		return &corev1.Pod{Name: "unlabeled", Namespace: "default", Labels: maps.Clone(legacyLabels)}
	}
	withHistoryLimit := func(dp *appsv1.Deployment, limit int32) *appsv1.Deployment {
		dp.Spec.RevisionHistoryLimit = new(limit)
		return dp
	}

	t.Run("old ReplicaSets not cleaned up yet: reconciled with revisionHistoryLimit 0 and the existing selector", func(t *testing.T) {
		kb := kbWithBG(nil, nil)
		kb.Spec.RevisionHistoryLimit = ptr.To[int32](5)
		res, reconciledDp, got, err := reconcileOnce(t, kb, withHistoryLimit(legacyDp(), 5), labeledPod())
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.NotNil(t, reconciledDp)
		assert.True(t, got.DeletionTimestamp.IsZero())
		require.NotNil(t, got.Spec.RevisionHistoryLimit)
		assert.Equal(t, int32(0), *got.Spec.RevisionHistoryLimit, "the Kibana spec value must be overwritten")
		assert.Equal(t, legacyLabels, got.Spec.Selector.MatchLabels)
		// While the migration is in progress, the pool keeps running all Kibana roles:
		// NODE_ROLES must not be injected yet (omitRoleSplit).
		_, found := findEnvVar(findKibanaContainerEnv(got.Spec.Template.Spec), kblabel.NodeRolesEnvVar)
		assert.False(t, found, "NODE_ROLES must not be set while the selector migration is in progress")
	})

	incomplete := map[string]func(*appsv1.Deployment){
		"the controller has not observed the spec yet": func(dp *appsv1.Deployment) {
			dp.Generation, dp.Status.ObservedGeneration = 2, 1
		},
		"not all replicas are updated": func(dp *appsv1.Deployment) {
			dp.Status.Replicas, dp.Status.UpdatedReplicas, dp.Status.AvailableReplicas = 3, 2, 2
		},
		"not all replicas are available": func(dp *appsv1.Deployment) {
			dp.Status.Replicas, dp.Status.UpdatedReplicas, dp.Status.AvailableReplicas = 3, 3, 2
		},
	}
	for name, makeIncomplete := range incomplete {
		t.Run("revisionHistoryLimit 0 but "+name+": not deleted and the limit is kept at 0", func(t *testing.T) {
			kb := kbWithBG(nil, nil)
			kb.Spec.RevisionHistoryLimit = ptr.To[int32](5)
			dp := withHistoryLimit(legacyDp(), 0)
			makeIncomplete(dp)
			res, _, got, err := reconcileOnce(t, kb, dp, labeledPod())
			require.NoError(t, err)
			assert.Equal(t, requeue, res)
			assert.True(t, got.DeletionTimestamp.IsZero(), "must wait for the deployment to be complete")
			require.NotNil(t, got.Spec.RevisionHistoryLimit)
			assert.Equal(t, int32(0), *got.Spec.RevisionHistoryLimit, "the limit must not be reset while waiting")
		})
	}

	t.Run("revisionHistoryLimit 0 and complete: deleted", func(t *testing.T) {
		kb := kbWithBG(nil, nil)
		dp := withHistoryLimit(legacyDp(), 0)
		dp.Finalizers = []string{metav1.FinalizerOrphanDependents} // keep it around to observe the deletion
		dp.Spec.Replicas = ptr.To[int32](2)
		dp.Generation, dp.Status = 3, appsv1.DeploymentStatus{ObservedGeneration: 3, Replicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2}
		res, reconciledDp, got, err := reconcileOnce(t, kb, dp, labeledPod())
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.Nil(t, reconciledDp)
		assert.False(t, got.DeletionTimestamp.IsZero())
	})

	t.Run("pods without the role label: normal reconcile, the Kibana revisionHistoryLimit is kept", func(t *testing.T) {
		kb := kbWithBG(nil, nil)
		kb.Spec.RevisionHistoryLimit = ptr.To[int32](5)
		res, reconciledDp, got, err := reconcileOnce(t, kb, withHistoryLimit(legacyDp(), 0), labeledPod(), unlabeledPod())
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.NotNil(t, reconciledDp)
		assert.True(t, got.DeletionTimestamp.IsZero())
		require.NotNil(t, got.Spec.RevisionHistoryLimit)
		assert.Equal(t, int32(5), *got.Spec.RevisionHistoryLimit)
		assert.Equal(t, legacyLabels, got.Spec.Selector.MatchLabels)
	})

	t.Run("failed pod without the role label does not block the migration", func(t *testing.T) {
		kb := kbWithBG(nil, nil)
		kb.Spec.RevisionHistoryLimit = ptr.To[int32](5)
		failedPod := unlabeledPod()
		failedPod.Status.Phase = corev1.PodFailed
		res, reconciledDp, got, err := reconcileOnce(t, kb, withHistoryLimit(legacyDp(), 5), labeledPod(), failedPod)
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.NotNil(t, reconciledDp)
		assert.True(t, got.DeletionTimestamp.IsZero())
		require.NotNil(t, got.Spec.RevisionHistoryLimit)
		assert.Equal(t, int32(0), *got.Spec.RevisionHistoryLimit, "the migration must proceed despite the failed pod")
	})

	t.Run("deployment already being deleted: requeue without reconciling it", func(t *testing.T) {
		kb := kbWithBG(nil, nil)
		dp := withHistoryLimit(legacyDp(), 7)
		dp.Finalizers = []string{metav1.FinalizerOrphanDependents} // an object with a deletionTimestamp needs a finalizer
		dp.DeletionTimestamp = new(metav1.Now())
		res, reconciledDp, got, err := reconcileOnce(t, kb, dp, labeledPod())
		require.NoError(t, err)
		assert.Equal(t, requeue, res)
		assert.Nil(t, reconciledDp)
		require.NotNil(t, got.Spec.RevisionHistoryLimit)
		assert.Equal(t, int32(7), *got.Spec.RevisionHistoryLimit, "a terminating deployment must not be updated")
	})
}

func TestReconcilePoolDeployment(t *testing.T) {
	newDriverFor := func(t *testing.T, kb *kbv1.Kibana, objects ...client.Object) *driver {
		t.Helper()
		d, err := newDriver(k8s.NewFakeClient(objects...), watches.NewDynamicWatches(), toolsevents.NewFakeRecorder(100), kb, corev1.IPv4Protocol)
		require.NoError(t, err)
		return d
	}

	t.Run("creates deployment and reconciles the pool config secret", func(t *testing.T) {
		kb := kibanaFixture()
		kb.Spec.Config = &commonv1.Config{Data: map[string]any{"server.name": "from-spec"}}
		d := newDriverFor(t, kb, defaultInitialObjects()...)

		pp, err := newPoolParameters(operator.Parameters{}, kb, kblabel.SinglePoolRole, "", false)
		require.NoError(t, err)
		meta := metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()})

		reconciledSecrets := map[string]struct{}{}
		res, reconciledDp, err := d.reconcilePoolDeployment(context.Background(), kb, pp, nil, meta, reconciledSecrets)
		require.NoError(t, err)

		assert.Equal(t, poolDeploymentResult(""), res)
		require.NotNil(t, reconciledDp)
		assert.Equal(t, kbv1.KBNamer.Suffix(kb.Name), reconciledDp.Name)
		assert.Equal(t, int32(1), *reconciledDp.Spec.Replicas)

		// The deployment must exist in the API server.
		var dp appsv1.Deployment
		require.NoError(t, d.client.Get(context.Background(),
			types.NamespacedName{Namespace: "default", Name: kbv1.KBNamer.Suffix(kb.Name)}, &dp))

		// The config secret must have been rewritten from the pool config.
		var sec corev1.Secret
		require.NoError(t, d.client.Get(context.Background(),
			types.NamespacedName{Namespace: "default", Name: kbv1.ConfigSecret(kb.Name)}, &sec))
		assert.Contains(t, string(sec.Data[SettingsFilename]), "from-spec")

		// The secret must be recorded as reconciled for this pass.
		assert.Contains(t, reconciledSecrets, kbv1.ConfigSecret(kb.Name))
	})

	t.Run("skips secret write when it was already reconciled in this pass", func(t *testing.T) {
		kb := kibanaFixture()
		kb.Spec.Config = &commonv1.Config{Data: map[string]any{"server.name": "from-spec"}}
		d := newDriverFor(t, kb, defaultInitialObjects()...)

		pp, err := newPoolParameters(operator.Parameters{}, kb, kblabel.SinglePoolRole, "", false)
		require.NoError(t, err)
		meta := metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()})

		// Pretend an earlier pool sharing the same secret already wrote it.
		reconciledSecrets := map[string]struct{}{kbv1.ConfigSecret(kb.Name): {}}
		_, reconciledDp, err := d.reconcilePoolDeployment(context.Background(), kb, pp, nil, meta, reconciledSecrets)
		require.NoError(t, err)
		require.NotNil(t, reconciledDp)

		// The secret must keep its initial content (see defaultInitialObjects): no second write.
		var sec corev1.Secret
		require.NoError(t, d.client.Get(context.Background(),
			types.NamespacedName{Namespace: "default", Name: kbv1.ConfigSecret(kb.Name)}, &sec))
		assert.Equal(t, "server.name: test", string(sec.Data["kibana.yml"]))
	})

	t.Run("scaleToZero pool params produce a zero-replicas deployment", func(t *testing.T) {
		kb := kibanaFixture()
		d := newDriverFor(t, kb, defaultInitialObjects()...)

		pp, err := newPoolParameters(operator.Parameters{}, kb, kblabel.SinglePoolRole, "", true)
		require.NoError(t, err)
		meta := metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()})

		_, reconciledDp, err := d.reconcilePoolDeployment(context.Background(), kb, pp, nil, meta, map[string]struct{}{})
		require.NoError(t, err)
		require.NotNil(t, reconciledDp)
		require.NotNil(t, reconciledDp.Spec.Replicas)
		assert.Equal(t, int32(0), *reconciledDp.Spec.Replicas)
	})
}

func TestReconcilePoolDeploymentMetadataMerge(t *testing.T) {
	owner := &kbv1.Kibana{Name: "test", Namespace: "default"}
	dpName := kbv1.BackgroundTasksDeployment("test")

	type metadata struct {
		labels      map[string]string
		annotations map[string]string
	}

	tests := []struct {
		name            string
		existing        *metadata // nil: the deployment does not exist yet
		expected        metadata
		wantLabels      map[string]string
		wantAnnotations map[string]string
	}{
		{
			name:            "prev-replicas removed when no longer expected, other annotations kept",
			existing:        &metadata{annotations: map[string]string{replicasAnnotationName: "4", "user-annotation": "keep-me"}},
			wantAnnotations: map[string]string{"user-annotation": "keep-me"},
		},
		{
			name:            "prev-replicas updated when expected",
			existing:        &metadata{annotations: map[string]string{replicasAnnotationName: "2"}},
			expected:        metadata{annotations: map[string]string{replicasAnnotationName: "4"}},
			wantAnnotations: map[string]string{replicasAnnotationName: "4"},
		},
		{
			name:            "prev-replicas added when expected and not previously set",
			existing:        &metadata{annotations: map[string]string{"user-annotation": "keep-me"}},
			expected:        metadata{annotations: map[string]string{replicasAnnotationName: "4"}},
			wantAnnotations: map[string]string{replicasAnnotationName: "4", "user-annotation": "keep-me"},
		},
		{
			name:            "prev-replicas set on creation when expected",
			expected:        metadata{annotations: map[string]string{replicasAnnotationName: "4"}},
			wantAnnotations: map[string]string{replicasAnnotationName: "4"},
		},
		{
			name:     "annotations merged: expected override, existing extras kept",
			existing: &metadata{annotations: map[string]string{"shared": "old", "user-only": "keep-me"}},
			expected: metadata{annotations: map[string]string{"shared": "new", "added": "yes"}},
			wantAnnotations: map[string]string{
				"shared": "new", "added": "yes", "user-only": "keep-me",
			},
		},
		{
			name:       "labels merged: expected override, existing extras kept",
			existing:   &metadata{labels: map[string]string{"shared": "old", "user-only": "keep-me"}},
			expected:   metadata{labels: map[string]string{"shared": "new", "added": "yes"}},
			wantLabels: map[string]string{"shared": "new", "added": "yes", "user-only": "keep-me"},
		},
		{
			name:       "labels are never removed, even one named like the prev-replicas annotation",
			existing:   &metadata{labels: map[string]string{replicasAnnotationName: "4"}},
			wantLabels: map[string]string{replicasAnnotationName: "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []client.Object
			if tt.existing != nil {
				objs = append(objs, &appsv1.Deployment{
					Name: dpName, Namespace: "default",
					Labels: tt.existing.labels, Annotations: tt.existing.annotations,
					Spec: appsv1.DeploymentSpec{Replicas: ptr.To[int32](0)},
				})
			}
			fakeClient := k8s.NewFakeClient(objs...)

			expected := appsv1.Deployment{
				Name: dpName, Namespace: "default",
				Labels: tt.expected.labels, Annotations: tt.expected.annotations,
				Spec: appsv1.DeploymentSpec{Replicas: ptr.To[int32](4)},
			}
			got, err := ReconcilePoolDeployment(t.Context(), fakeClient, expected, owner)
			require.NoError(t, err)

			// the template hash label is added by the reconciliation, so only check the wanted labels
			for k, v := range tt.wantLabels {
				assert.Equal(t, v, got.Labels[k], "label %q", k)
			}
			assert.Equal(t, tt.wantAnnotations, nilIfEmpty(got.Annotations))

			// the result must be what was persisted
			var persisted appsv1.Deployment
			require.NoError(t, fakeClient.Get(t.Context(), types.NamespacedName{Namespace: "default", Name: dpName}, &persisted))
			assert.Equal(t, got.Annotations, persisted.Annotations)
			assert.Equal(t, got.Labels, persisted.Labels)
		})
	}
}

func nilIfEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

func TestGarbageCollectBGPauseGuard(t *testing.T) {
	bgDpName := kbv1.BackgroundTasksDeployment("test")
	bgSecretName := kbv1.BackgroundTasksConfigSecret("test")

	existingBGDeployment := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			Name: bgDpName, Namespace: "default",
		}
	}
	existingBGSecret := func() *corev1.Secret {
		return &corev1.Secret{
			Name: bgSecretName, Namespace: "default",
		}
	}

	kbPaused := &kbv1.Kibana{
		Name: "test", Namespace: "default",
		Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "true"},
	}
	kbActive := &kbv1.Kibana{
		Name: "test", Namespace: "default",
	}

	t.Run("paused: GC skipped, deployment and secret survive", func(t *testing.T) {
		fakeClient := k8s.NewFakeClient(existingBGDeployment(), existingBGSecret())
		d := &driver{client: fakeClient}
		require.NoError(t, d.garbageCollectBackgroundResources(context.Background(), kbPaused))

		var dp appsv1.Deployment
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGDeployment()), &dp))
		var sec corev1.Secret
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGSecret()), &sec))
	})

	t.Run("not paused: deployment and secret deleted", func(t *testing.T) {
		fakeClient := k8s.NewFakeClient(existingBGDeployment(), existingBGSecret())
		d := &driver{client: fakeClient}
		require.NoError(t, d.garbageCollectBackgroundResources(context.Background(), kbActive))

		var dp appsv1.Deployment
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGDeployment()), &dp)))
		var sec corev1.Secret
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGSecret()), &sec)))
	})

	t.Run("resume after pause: deletion happens on next call", func(t *testing.T) {
		fakeClient := k8s.NewFakeClient(existingBGDeployment(), existingBGSecret())
		d := &driver{client: fakeClient}

		// First call: paused — resources survive.
		require.NoError(t, d.garbageCollectBackgroundResources(context.Background(), kbPaused))
		var dp appsv1.Deployment
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGDeployment()), &dp))

		// Second call: not paused — resources deleted.
		require.NoError(t, d.garbageCollectBackgroundResources(context.Background(), kbActive))
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(existingBGDeployment()), &dp)))
	})
}

// ---- NODE_ROLES env var injection -------------------------------------------

func findKibanaContainerEnv(spec corev1.PodSpec) []corev1.EnvVar {
	for _, c := range spec.Containers {
		if c.Name == kbv1.KibanaContainerName {
			return c.Env
		}
	}
	return nil
}

func findEnvVar(env []corev1.EnvVar, name string) (corev1.EnvVar, bool) {
	for _, e := range env {
		if e.Name == name {
			return e, true
		}
	}
	return corev1.EnvVar{}, false
}

func TestNodeRolesEnvVar(t *testing.T) {
	// Use the existing kibanaFixture (7.17.0) + initialObjects for deploymentParams.
	// The NODE_ROLES injection is independent of the Kibana version.
	objs := defaultInitialObjects()

	kbWithBGTasks := func() *kbv1.Kibana {
		kb := kibanaFixture()
		kb.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{Count: new(int32(1))}
		return kb
	}

	cases := []struct {
		name          string
		role          kblabel.Role
		omitRoleSplit bool
		kb            *kbv1.Kibana
		wantNodeRoles string
		wantPresent   bool
	}{
		{
			name:        "Single-pool mode injects no NODE_ROLES",
			role:        kblabel.SinglePoolRole,
			kb:          kibanaFixture(),
			wantPresent: false,
		},
		{
			name:          "UIRole injects NODE_ROLES=[\"ui\"]",
			role:          kblabel.UIRole,
			kb:            kibanaFixture(),
			wantNodeRoles: `["ui"]`,
			wantPresent:   true,
		},
		{
			name:          "BackgroundTasksRole injects NODE_ROLES=[\"background_tasks\"]",
			role:          kblabel.BackgroundTasksRole,
			kb:            kbWithBGTasks(),
			wantNodeRoles: `["background_tasks"]`,
			wantPresent:   true,
		},
		{
			name:          "UIRole with omitRoleSplit (selector migration in progress) injects no NODE_ROLES",
			role:          kblabel.UIRole,
			omitRoleSplit: true,
			kb:            kbWithBGTasks(),
			wantPresent:   false,
		},
		{
			name:          "BackgroundTasksRole is not affected by omitRoleSplit",
			role:          kblabel.BackgroundTasksRole,
			omitRoleSplit: true,
			kb:            kbWithBGTasks(),
			wantNodeRoles: `["background_tasks"]`,
			wantPresent:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDriver(t, objs...)
			replicas := new(tc.kb.Spec.Count)
			params, err := d.deploymentParams(
				context.Background(),
				tc.kb,
				poolParameters{
					Role:                      tc.role,
					SecretName:                kbv1.ConfigSecret(tc.kb.Name),
					DeploymentName:            kbv1.KBNamer.Suffix(tc.kb.Name),
					SetDefaultSecurityContext: true,
				},
				replicas,
				nil, // replicasAnnotationValue
				nil, // policyAnnotations
				metadata.Propagate(tc.kb, metadata.Metadata{Labels: tc.kb.GetIdentityLabels()}),
				selectorChangeOutcome{expectedSelector: tc.kb.GetIdentityLabels(), omitRoleSplit: tc.omitRoleSplit},
			)
			require.NoError(t, err)

			env := findKibanaContainerEnv(params.PodTemplateSpec.Spec)
			nodeRoles, found := findEnvVar(env, kblabel.NodeRolesEnvVar)
			assert.Equal(t, tc.wantPresent, found, "NODE_ROLES presence mismatch")
			if tc.wantPresent {
				assert.Equal(t, tc.wantNodeRoles, nodeRoles.Value)
				// Verify it is the first env var so it can be overridden by user-supplied vars.
				assert.Equal(t, kblabel.NodeRolesEnvVar, env[0].Name, "NODE_ROLES must be first env var")
			}
			// The role label always reflects the pool role, even when omitRoleSplit remaps the
			// NODE_ROLES injection to the single-pool behavior.
			assert.Equal(t, tc.role.LabelValue, params.PodTemplateSpec.Labels[kblabel.RoleLabelName])
		})
	}
}

// ---- Service selector -------------------------------------------------------

func TestServiceSelector(t *testing.T) {
	kbNoSplit := func() *kbv1.Kibana {
		return &kbv1.Kibana{Name: "mykb", Namespace: "default", Spec: kbv1.KibanaSpec{Version: "8.17.0"}}
	}
	kbSplit := func() *kbv1.Kibana {
		return &kbv1.Kibana{Name: "mykb", Namespace: "default", Spec: kbv1.KibanaSpec{
			Version:         "8.17.0",
			BackgroundTasks: &kbv1.KibanaBackgroundTasks{},
		}}
	}

	svcWith := func(selector map[string]string) *corev1.Service {
		svc := &corev1.Service{}
		svc.Name = kbv1.HTTPService("mykb")
		svc.Namespace = "default"
		svc.Spec.Selector = selector
		return svc
	}

	tests := []struct {
		name        string
		kb          *kbv1.Kibana
		existingSvc *corev1.Service // nil means no service in API server
		wantRole    string          // "" means role label must be absent
	}{
		{
			name:     "no existing service: returns full selector with role=primary",
			kb:       kbNoSplit(),
			wantRole: kblabel.RolePrimaryValue,
		},
		{
			name: "split disabled, existing has role=primary: no change",
			kb:   kbNoSplit(),
			existingSvc: svcWith(map[string]string{
				kblabel.KibanaNameLabelName: "mykb",
				kblabel.RoleLabelName:       kblabel.RolePrimaryValue,
			}),
			wantRole: kblabel.RolePrimaryValue,
		},
		{
			name: "split disabled, existing has no role label: strips role (ECK upgrade path)",
			kb:   kbNoSplit(),
			existingSvc: svcWith(map[string]string{
				kblabel.KibanaNameLabelName: "mykb",
			}),
			wantRole: "", // absent
		},
		{
			name: "split enabled, existing has no role label: keeps role=primary (exception does not apply)",
			kb:   kbSplit(),
			existingSvc: svcWith(map[string]string{
				kblabel.KibanaNameLabelName: "mykb",
			}),
			wantRole: kblabel.RolePrimaryValue,
		},
		{
			name:     "split enabled, no existing service: returns full selector with role=primary",
			kb:       kbSplit(),
			wantRole: kblabel.RolePrimaryValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var existingSelector map[string]string
			if tt.existingSvc != nil {
				existingSelector = tt.existingSvc.Spec.Selector
			}

			sel := serviceSelector(tt.kb, existingSelector)

			if tt.wantRole == "" {
				_, hasRole := sel[kblabel.RoleLabelName]
				assert.False(t, hasRole, "role label must be absent")
			} else {
				assert.Equal(t, tt.wantRole, sel[kblabel.RoleLabelName])
			}
			assert.Equal(t, "mykb", sel[kblabel.KibanaNameLabelName])
		})
	}
}

// ---- Deployment selector labels (via GetPoolIdentityLabels) -----------------

func TestGetPoolIdentityLabels_SplitOnOff(t *testing.T) {
	t.Run("split disabled: single pool always gets role=primary", func(t *testing.T) {
		kb := &kbv1.Kibana{Name: "x"}
		labels := kb.GetPoolIdentityLabels(kblabel.UIRole)
		assert.Equal(t, kblabel.RolePrimaryValue, labels[kblabel.RoleLabelName])
	})

	t.Run("split enabled: UI selector gets primary, BG selector gets background_tasks", func(t *testing.T) {
		kb := &kbv1.Kibana{
			Name: "x",
			Spec: kbv1.KibanaSpec{BackgroundTasks: &kbv1.KibanaBackgroundTasks{}},
		}
		uiLabels := kb.GetPoolIdentityLabels(kblabel.UIRole)
		assert.Equal(t, kblabel.RolePrimaryValue, uiLabels[kblabel.RoleLabelName])

		bgLabels := kb.GetPoolIdentityLabels(kblabel.BackgroundTasksRole)
		assert.Equal(t, kblabel.RoleBackgroundTasksValue, bgLabels[kblabel.RoleLabelName])

		// The two selectors are mutually exclusive — same key, different values.
		assert.NotEqual(t, uiLabels[kblabel.RoleLabelName], bgLabels[kblabel.RoleLabelName])
	})
}

// ---- getPodTemplateSpecForRole ----------------------------------------------

func TestGetPodTemplateSpecForRole(t *testing.T) {
	basePodTemplate := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "kibana", Image: "base-image"},
			},
			NodeName: "base-node",
		},
	}

	bgOverlay := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "kibana", Image: "overlay-image"},
			},
			NodeSelector: map[string]string{"role": "bg"},
		},
	}

	tests := []struct {
		name      string
		kb        kbv1.Kibana
		role      kblabel.Role
		wantImage string            // expected image on the kibana container
		wantNode  string            // expected NodeName (base field preserved by merge)
		wantNS    map[string]string // expected NodeSelector, nil means not checked
	}{
		{
			name: "single-pool: returns spec.podTemplate unchanged",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{PodTemplate: basePodTemplate},
			},
			role:      kblabel.SinglePoolRole,
			wantImage: "base-image",
			wantNode:  "base-node",
		},
		{
			name: "split, UIRole: returns spec.podTemplate unchanged",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					PodTemplate:     basePodTemplate,
					BackgroundTasks: &kbv1.KibanaBackgroundTasks{PodTemplate: bgOverlay},
				},
			},
			role:      kblabel.UIRole,
			wantImage: "base-image",
			wantNode:  "base-node",
		},
		{
			name: "split, SinglePoolRole: BackgroundTasks set but role has no name, returns spec.podTemplate",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					PodTemplate:     basePodTemplate,
					BackgroundTasks: &kbv1.KibanaBackgroundTasks{PodTemplate: bgOverlay},
				},
			},
			role:      kblabel.SinglePoolRole,
			wantImage: "base-image",
			wantNode:  "base-node",
		},
		{
			name: "split, BGRole, empty overlay: returns spec.podTemplate unchanged",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					PodTemplate:     basePodTemplate,
					BackgroundTasks: &kbv1.KibanaBackgroundTasks{},
				},
			},
			role:      kblabel.BackgroundTasksRole,
			wantImage: "base-image",
			wantNode:  "base-node",
		},
		{
			name: "split, BGRole: overlay container image replaces base; base-only fields preserved",
			kb: kbv1.Kibana{
				Spec: kbv1.KibanaSpec{
					PodTemplate:     basePodTemplate,
					BackgroundTasks: &kbv1.KibanaBackgroundTasks{PodTemplate: bgOverlay},
				},
			},
			role:      kblabel.BackgroundTasksRole,
			wantImage: "overlay-image",
			wantNode:  "base-node", // base NodeName preserved; overlay did not set it
			wantNS:    map[string]string{"role": "bg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPodTemplateSpecForRole(tt.kb, tt.role)
			require.NoError(t, err)

			require.Len(t, got.Spec.Containers, 1)
			assert.Equal(t, tt.wantImage, got.Spec.Containers[0].Image)
			assert.Equal(t, tt.wantNode, got.Spec.NodeName)
			if tt.wantNS != nil {
				assert.Equal(t, tt.wantNS, got.Spec.NodeSelector)
			}
		})
	}
}

// ---- shouldScaleAllPoolsToZeroForUpgrade ------------------------------------------------------

func makePod(name, version string, role kblabel.Role) corev1.Pod {
	return corev1.Pod{
		Name: name,
		Labels: map[string]string{
			kblabel.KibanaVersionLabelName: version,
			kblabel.RoleLabelName:          role.LabelValue,
		},
	}
}

func TestShouldScaleAllPoolsToZeroForUpgrade(t *testing.T) {
	const (
		oldVersion = "8.17.0"
		newVersion = "8.18.0"
	)

	kbSingle := kbv1.Kibana{Spec: kbv1.KibanaSpec{Version: newVersion}}
	kbSplit := kbv1.Kibana{Spec: kbv1.KibanaSpec{
		Version:         newVersion,
		BackgroundTasks: &kbv1.KibanaBackgroundTasks{},
	}}

	makePodInPhase := func(name, version string, role kblabel.Role, phase corev1.PodPhase) corev1.Pod {
		p := makePod(name, version, role)
		p.Status.Phase = phase
		return p
	}

	tests := []struct {
		name string
		kb   kbv1.Kibana
		pods []corev1.Pod
		want bool
	}{
		{
			name: "single-pool, no pods: not needed",
			kb:   kbSingle,
			pods: nil,
			want: false,
		},
		{
			name: "single-pool, stale pod: not needed (Recreate handles single Deployment)",
			kb:   kbSingle,
			pods: []corev1.Pod{makePod("kb-0", oldVersion, kblabel.SinglePoolRole)},
			want: false,
		},
		{
			name: "split, no pods: not needed",
			kb:   kbSplit,
			pods: nil,
			want: false,
		},
		{
			name: "split, all pods at new version: not needed",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePod("kb-ui-0", newVersion, kblabel.UIRole),
				makePod("kb-bg-0", newVersion, kblabel.BackgroundTasksRole),
			},
			want: false,
		},
		{
			name: "split, both pools stale: needed",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePod("kb-ui-0", oldVersion, kblabel.UIRole),
				makePod("kb-bg-0", oldVersion, kblabel.BackgroundTasksRole),
			},
			want: true,
		},
		{
			// Unequal shutdown times: UI pool drained first, BG pod still terminating.
			// The stop phase must remain active until ALL stale pods are gone.
			name: "split, UI pod gone but BG pod still stale: needed",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePod("kb-bg-0", oldVersion, kblabel.BackgroundTasksRole),
			},
			want: true,
		},
		{
			// Mirror case: BG pool drained first, UI pod still terminating.
			name: "split, BG pod gone but UI pod still stale: needed",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePod("kb-ui-0", oldVersion, kblabel.UIRole),
			},
			want: true,
		},
		{
			name: "split, pod with missing version label: needed (treated as stale)",
			kb:   kbSplit,
			pods: []corev1.Pod{
				{Name: "kb-ui-0"},
			},
			want: true,
		},
		{
			// Terminal pods never restart, so they cannot run stale code: they must not
			// keep the stop phase active.
			name: "split, stale pod in Failed phase: not needed (terminal pods ignored)",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePodInPhase("kb-bg-0", oldVersion, kblabel.BackgroundTasksRole, corev1.PodFailed),
			},
			want: false,
		},
		{
			name: "split, stale pod in Succeeded phase: not needed (terminal pods ignored)",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePodInPhase("kb-bg-0", oldVersion, kblabel.BackgroundTasksRole, corev1.PodSucceeded),
			},
			want: false,
		},
		{
			name: "split, Failed stale pod plus running stale pod: needed (running pod still counts)",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePodInPhase("kb-bg-0", oldVersion, kblabel.BackgroundTasksRole, corev1.PodFailed),
				makePodInPhase("kb-ui-0", oldVersion, kblabel.UIRole, corev1.PodRunning),
			},
			want: true,
		},
		{
			name: "split, Failed pod with missing version label: not needed (terminal pods ignored)",
			kb:   kbSplit,
			pods: []corev1.Pod{
				{Name: "kb-ui-0", Status: corev1.PodStatus{Phase: corev1.PodFailed}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldScaleAllPoolsToZeroForUpgrade(&tt.kb, tt.pods))
		})
	}
}

// ---- deploymentReplicas -----------------------------------------------------

func TestDeploymentReplicas(t *testing.T) {
	ptr := func(n int32) *int32 { return &n }

	kbSplit := func(bgCount *int32) *kbv1.Kibana {
		return &kbv1.Kibana{Spec: kbv1.KibanaSpec{
			Count:           3,
			BackgroundTasks: &kbv1.KibanaBackgroundTasks{Count: bgCount},
		}}
	}
	kbSingle := &kbv1.Kibana{Spec: kbv1.KibanaSpec{Count: 3}}

	dp := func(deploymentReplicas *int32, annotations map[string]string) *appsv1.Deployment {
		d := &appsv1.Deployment{}
		d.Name = "test-dp"
		d.Annotations = annotations
		d.Spec.Replicas = deploymentReplicas
		return d
	}
	ann := func(v string) map[string]string {
		return map[string]string{replicasAnnotationName: v}
	}

	// backgroundReplicas priority: Count > HPA replicas (> 0) > annotation.
	// deploymentReplicas does not mutate the deployment: the annotation is persisted only
	// when replicasToSave is non-nil (deploymentParams) and dropped otherwise by the
	// ReconcilePoolDeployment annotations merge strategy.
	tests := []struct {
		name        string
		scaleToZero bool
		kb          *kbv1.Kibana
		role        kblabel.Role
		existingDp  *appsv1.Deployment
		wantNew     *int32
		wantOld     *int32
	}{
		// ---- no stop phase --------------------------------------------------
		{
			name:       "single-pool: returns kb.Spec.Count",
			kb:         kbSingle,
			role:       kblabel.SinglePoolRole,
			existingDp: dp(ptr(3), nil),
			wantNew:    ptr(3),
		},
		{
			name:       "BG, explicit count: Count wins",
			kb:         kbSplit(ptr(2)),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(2), nil),
			wantNew:    ptr(2),
		},
		{
			name:       "BG, explicit count ignores annotation: Count wins over annotation",
			kb:         kbSplit(ptr(2)),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(0), ann("5")),
			wantNew:    ptr(2),
		},
		{
			name:       "BG, Count nil, Deployment value: HPA replicas win",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(4), nil),
			wantNew:    ptr(4),
		},
		{
			name:       "BG, Count nil, Deployment nil, annotation: annotation wins as last resort",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(nil, ann("5")),
			wantNew:    ptr(5),
		},
		{
			name:       "BG, Count nil, Deployment nil, no annotation: returns nil",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(nil, nil),
			wantNew:    nil,
		},
		{
			name:       "BG, Count nil, Deployment nil, malformed annotation: returns nil",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(nil, ann("not-a-number")),
			wantNew:    nil,
		},
		{
			name:       "BG, Count nil, Deployment zeroed, with annotation: restores annotation value",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(0), ann("4")),
			wantNew:    ptr(4),
		},
		// ---- stop phase -----------------------------------------------------
		{
			name:        "stop, UI role: scales to zero, no replicasToSave",
			scaleToZero: true,
			kb:          kbSplit(ptr(2)),
			role:        kblabel.UIRole,
			existingDp:  dp(ptr(3), nil),
			wantNew:     ptr(0),
			wantOld:     nil,
		},
		{
			name:        "stop, BG, explicit count: Count saved as replicasToSave",
			scaleToZero: true,
			kb:          kbSplit(ptr(2)),
			role:        kblabel.BackgroundTasksRole,
			existingDp:  dp(ptr(2), nil),
			wantNew:     ptr(0),
			wantOld:     ptr(2),
		},
		{
			name:        "stop, BG, explicit count ignores annotation: Count wins over annotation",
			scaleToZero: true,
			kb:          kbSplit(ptr(2)),
			role:        kblabel.BackgroundTasksRole,
			existingDp:  dp(nil, ann("4")),
			wantNew:     ptr(0),
			wantOld:     ptr(2),
		},
		{
			name:        "stop, BG, Count nil, Deployment value: HPA replicas saved as replicasToSave",
			scaleToZero: true,
			kb:          kbSplit(nil),
			role:        kblabel.BackgroundTasksRole,
			existingDp:  dp(ptr(4), nil),
			wantNew:     ptr(0),
			wantOld:     ptr(4),
		},
		{
			name:        "stop, BG, Count nil, Deployment nil, annotation: annotation saved as replicasToSave and kept",
			scaleToZero: true,
			kb:          kbSplit(nil),
			role:        kblabel.BackgroundTasksRole,
			existingDp:  dp(nil, ann("4")),
			wantNew:     ptr(0),
			wantOld:     ptr(4),
		},
		{
			name:        "stop, BG, Count nil, Deployment nil, no annotation: replicasToSave fallbacks to 1",
			scaleToZero: true,
			kb:          kbSplit(nil),
			role:        kblabel.BackgroundTasksRole,
			existingDp:  dp(nil, nil),
			wantNew:     ptr(0),
			wantOld:     ptr(1),
		},
		{
			name:        "stop, BG, Count nil, Deployment nil, malformed annotation: replicasToSave fallbacks to 1",
			scaleToZero: true,
			kb:          kbSplit(nil),
			role:        kblabel.BackgroundTasksRole,
			existingDp:  dp(nil, ann("not-a-number")),
			wantNew:     ptr(0),
			wantOld:     ptr(1),
		},
		{
			name:        "stop, BG, Count nil, Deployment zero, with annotation: replicasToSave from annotation",
			scaleToZero: true,
			kb:          kbSplit(nil),
			role:        kblabel.BackgroundTasksRole,
			existingDp:  dp(ptr(0), ann("4")),
			wantNew:     ptr(0),
			wantOld:     ptr(4),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationsBefore := maps.Clone(tt.existingDp.Annotations)
			gotNew, gotOld := deploymentReplicas(t.Context(), tt.kb, poolParameters{Role: tt.role, ScaleToZero: tt.scaleToZero}, tt.existingDp)

			cmpPtrInt32 := func(a, b *int32, msg string) {
				if a != nil && b != nil {
					assert.Equal(t, *a, *b, msg)
				} else {
					assert.Equal(t, a, b, msg)
				}
			}
			cmpPtrInt32(tt.wantNew, gotNew, "new values mismatch")
			cmpPtrInt32(tt.wantOld, gotOld, "old values mismatch")

			// deploymentReplicas must not mutate the existing deployment.
			assert.Equal(t, annotationsBefore, tt.existingDp.Annotations)
		})
	}
}

// ---- deploymentParams: replicas annotation ----------------------------------

func TestDeploymentParamsReplicasAnnotation(t *testing.T) {
	ptr := func(n int32) *int32 { return &n }

	// Kibana with background tasks enabled (needed for BG-role calls to succeed).
	kbWithBG := func() *kbv1.Kibana {
		kb := kibanaFixture()
		kb.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{Count: ptr(2)}
		return kb
	}

	// bgConfigSecret is needed when deploymentParams is called with BackgroundTasksRole,
	// because it fetches the config secret by name from the API server.
	bgConfigSecret := &corev1.Secret{
		Data: map[string][]byte{"kibana.yml": []byte("server.name: test")},
	}
	bgConfigSecret.Name = kbv1.BackgroundTasksConfigSecret("test")
	bgConfigSecret.Namespace = "default"

	tests := []struct {
		name                string
		role                kblabel.Role
		replicasAnnotation  *int32
		wantAnnotationValue string // "" means the key must be absent
	}{
		{
			name:                "BG role, non-nil value: annotation written",
			role:                kblabel.BackgroundTasksRole,
			replicasAnnotation:  ptr(5),
			wantAnnotationValue: "5",
		},
		{
			name:               "BG role, nil value: annotation absent",
			role:               kblabel.BackgroundTasksRole,
			replicasAnnotation: nil,
		},
		{
			name:               "SinglePool role, non-nil value: annotation absent",
			role:               kblabel.SinglePoolRole,
			replicasAnnotation: ptr(5),
		},
		{
			name:               "UI role, non-nil value: annotation absent",
			role:               kblabel.UIRole,
			replicasAnnotation: ptr(5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := kbWithBG()

			initialObjects := append(defaultInitialObjects(), bgConfigSecret)
			c := k8s.NewFakeClient(initialObjects...)
			w := watches.NewDynamicWatches()
			d, err := newDriver(c, w, toolsevents.NewFakeRecorder(100), kb, corev1.IPv4Protocol)
			require.NoError(t, err)

			configSecret := kbv1.ConfigSecret(kb.Name)
			dpName := kbv1.KBNamer.Suffix(kb.Name)
			if tt.role == kblabel.BackgroundTasksRole {
				configSecret = kbv1.BackgroundTasksConfigSecret(kb.Name)
				dpName = kbv1.BackgroundTasksDeployment(kb.Name)
			}
			replicas := ptr(kb.Spec.Count)
			meta := metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()})

			got, err := d.deploymentParams(
				context.Background(), kb,
				poolParameters{Role: tt.role, SecretName: configSecret, DeploymentName: dpName},
				replicas, tt.replicasAnnotation,
				nil,
				meta, selectorChangeOutcome{expectedSelector: kb.GetIdentityLabels()},
			)
			require.NoError(t, err)

			val, exists := got.Metadata.Annotations[replicasAnnotationName]
			if tt.wantAnnotationValue == "" {
				assert.False(t, exists, "expected annotation to be absent, got %q", val)
			} else {
				assert.True(t, exists, "expected annotation to be present")
				assert.Equal(t, tt.wantAnnotationValue, val)
			}
		})
	}
}

// ---- buildStatus --------------------------------------------------------------

func TestBuildStatus(t *testing.T) {
	// splitSelector is the role-less top-level selector set when the split is enabled.
	const splitSelector = "common.k8s.elastic.co/type=kibana,kibana.k8s.elastic.co/name=test"

	const elkVer = "8.17.0"

	primaryPoolStatus := commonv1.DeploymentStatus{
		Selector:       "kibana.k8s.elastic.co/role=primary",
		Count:          3,
		AvailableNodes: 3,
		Health:         commonv1.GreenHealth,
		Version:        elkVer,
	}
	bgPoolStatus := commonv1.DeploymentStatus{
		Selector:       "kibana.k8s.elastic.co/role=background_tasks",
		Count:          2,
		AvailableNodes: 1,
		Health:         commonv1.RedHealth,
		Version:        elkVer,
	}

	kbStatus := kbv1.KibanaStatus{
		Count:          primaryPoolStatus.Count + bgPoolStatus.Count,
		AvailableNodes: primaryPoolStatus.AvailableNodes + bgPoolStatus.AvailableNodes,
		Version:        elkVer,
		Health:         commonv1.GreenHealth,
		Pools: kbv1.KibanaPoolsStatuses{
			Primary:         primaryPoolStatus,
			BackgroundTasks: &bgPoolStatus,
		},
	}

	// withCounts returns a copy of the given status with the count fields replaced.
	withCounts := func(s commonv1.DeploymentStatus, count, available int32) commonv1.DeploymentStatus {
		s.Count = count
		s.AvailableNodes = available
		return s
	}

	// topStatus returns the expected top-level aggregate: the role-less split selector,
	// the given counts and health, the version from the pools.
	topStatus := func(count, available int32, health commonv1.DeploymentHealth) commonv1.DeploymentStatus {
		return commonv1.DeploymentStatus{
			Selector:       splitSelector,
			Count:          count,
			AvailableNodes: available,
			Health:         health,
			Version:        elkVer,
		}
	}

	tests := []struct {
		name       string
		singlePool bool // the Kibana under test has no spec.backgroundTasks
		initial    kbv1.KibanaStatus
		entries    []poolStatusEntry
		want       kbv1.KibanaStatus
	}{
		{
			name:    "no pool entries: status left untouched",
			initial: kbStatus,
			entries: nil,
			want:    kbStatus,
		},
		{
			name:       "single pool: pool status copied verbatim, leftover background pool cleared",
			singlePool: true,
			initial: kbv1.KibanaStatus{
				Pools: kbv1.KibanaPoolsStatuses{BackgroundTasks: &commonv1.DeploymentStatus{Count: 1, Version: elkVer}},
			},
			entries: []poolStatusEntry{{role: kblabel.SinglePoolRole, status: primaryPoolStatus}},
			want: kbv1.KibanaStatus{
				DeploymentStatus: primaryPoolStatus,
				Pools:            kbv1.KibanaPoolsStatuses{Primary: primaryPoolStatus},
			},
		},
		{
			name:    "split, primary pool: pools.primary set and top level aggregated",
			entries: []poolStatusEntry{{role: kblabel.UIRole, status: primaryPoolStatus}},
			want: kbv1.KibanaStatus{
				DeploymentStatus: topStatus(3, 3, commonv1.GreenHealth),
				Pools:            kbv1.KibanaPoolsStatuses{Primary: primaryPoolStatus},
			},
		},
		{
			// the background tasks pool must not overwrite the primary-derived top-level
			// status, only the aggregate fields
			name: "split, both pools: top level sums counts and degrades health",
			entries: []poolStatusEntry{
				{role: kblabel.UIRole, status: primaryPoolStatus},
				{role: kblabel.BackgroundTasksRole, status: bgPoolStatus},
			},
			want: kbv1.KibanaStatus{
				DeploymentStatus: topStatus(5, 4, commonv1.YellowHealth),
				Pools: kbv1.KibanaPoolsStatuses{
					Primary:         primaryPoolStatus,
					BackgroundTasks: &bgPoolStatus,
				},
			},
		},
		{
			name:    "split, primary entry only: aggregate includes the carried-over background pool",
			initial: kbStatus,
			entries: []poolStatusEntry{{role: kblabel.UIRole, status: withCounts(primaryPoolStatus, 5, 5)}},
			want: kbv1.KibanaStatus{
				DeploymentStatus: topStatus(7, 6, commonv1.YellowHealth),
				Pools: kbv1.KibanaPoolsStatuses{
					Primary:         withCounts(primaryPoolStatus, 5, 5),
					BackgroundTasks: &bgPoolStatus,
				},
			},
		},
		{
			name:    "split, background entry only: aggregate includes the carried-over primary pool",
			initial: kbStatus,
			entries: []poolStatusEntry{{role: kblabel.BackgroundTasksRole, status: withCounts(bgPoolStatus, 5, 6)}},
			want: kbv1.KibanaStatus{
				DeploymentStatus: topStatus(8, 9, commonv1.YellowHealth),
				Pools: kbv1.KibanaPoolsStatuses{
					Primary:         primaryPoolStatus,
					BackgroundTasks: new(withCounts(bgPoolStatus, 5, 6)),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := kbWithBG(ptr.To[int32](2), nil)
			if tt.singlePool {
				kb = &kbv1.Kibana{Name: "test", Namespace: "default", Spec: kbv1.KibanaSpec{Count: 3}}
			}
			kb.Status = tt.initial
			state := &State{Kibana: kb}

			buildStatus(state, kb, tt.entries)

			assert.Equal(t, tt.want, state.Kibana.Status)
		})
	}
}

func TestReconcileServicePauseGuard(t *testing.T) {
	// Selector of a Service created by an older ECK version, before the role label existed.
	rolelessSelector := map[string]string{
		commonv1.TypeLabelName:      "kibana",
		kblabel.KibanaNameLabelName: "test",
	}
	fullSelector := map[string]string{
		commonv1.TypeLabelName:      "kibana",
		kblabel.KibanaNameLabelName: "test",
		kblabel.RoleLabelName:       kblabel.RolePrimaryValue,
	}

	mkSvc := func(selector map[string]string) *corev1.Service {
		svc := &corev1.Service{}
		svc.Name = kbv1.HTTPService("test")
		svc.Namespace = "default"
		svc.Spec.Selector = selector
		return svc
	}
	mkKb := func(split, paused bool) *kbv1.Kibana {
		kb := &kbv1.Kibana{}
		kb.Name = "test"
		kb.Namespace = "default"
		kb.Spec.Version = "8.17.0"
		if split {
			kb.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
		}
		if paused {
			kb.Annotations = map[string]string{commonv1.PauseOrchestrationAnnotation: "true"}
		}
		return kb
	}
	mkDp := func(selector map[string]string) *appsv1.Deployment {
		return &appsv1.Deployment{
			Name: kbv1.KBNamer.Suffix("test"), Namespace: "default",
			Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: selector}},
		}
	}

	tests := []struct {
		name       string
		split      bool
		paused     bool
		existing   *corev1.Service    // nil: the Service does not exist yet
		deployment *appsv1.Deployment // nil: the primary Deployment does not exist
		// wantSelector is the selector of the Service in the API server after reconciliation.
		wantSelector map[string]string
	}{
		{
			// Split enabled means the upgrade-path exception does not apply, so the expected
			// selector includes role=primary and differs from the roleless existing one.
			name:         "paused, selector differs: reconciliation skipped",
			split:        true,
			paused:       true,
			existing:     mkSvc(rolelessSelector),
			wantSelector: rolelessSelector,
		},
		{
			name:         "paused, same selector: reconciliation proceeds",
			split:        true,
			paused:       true,
			existing:     mkSvc(fullSelector),
			wantSelector: fullSelector,
		},
		{
			name:         "not paused, selector differs: selector updated",
			split:        true,
			paused:       false,
			existing:     mkSvc(rolelessSelector),
			wantSelector: fullSelector,
		},
		{
			name:         "paused, no existing service: service created with full selector",
			split:        true,
			paused:       true,
			existing:     nil,
			wantSelector: fullSelector,
		},
		{
			// Split disabled: the ECK upgrade-path exception strips the role label, the
			// selectors match and the roleless Service is left untouched.
			name:         "not paused, split off, roleless existing: selector kept roleless",
			split:        false,
			paused:       false,
			existing:     mkSvc(rolelessSelector),
			wantSelector: rolelessSelector,
		},
		{
			// The primary Deployment still has the legacy selector: its pods may not carry the
			// role label yet, so the Service must keep the legacy selector to not drop them from
			// the endpoints.
			name:         "not paused, selector differs, Deployment selector still legacy: selector kept",
			split:        true,
			paused:       false,
			existing:     mkSvc(rolelessSelector),
			deployment:   mkDp(rolelessSelector),
			wantSelector: rolelessSelector,
		},
		{
			// The primary Deployment was recreated with the role label in its selector: the
			// migration is done and the Service can switch.
			name:         "not paused, selector differs, Deployment selector has the role: selector updated",
			split:        true,
			paused:       false,
			existing:     mkSvc(rolelessSelector),
			deployment:   mkDp(fullSelector),
			wantSelector: fullSelector,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []client.Object
			if tt.existing != nil {
				objs = append(objs, tt.existing)
			}
			if tt.deployment != nil {
				objs = append(objs, tt.deployment)
			}
			client := k8s.NewFakeClient(objs...)

			kb := mkKb(tt.split, tt.paused)
			meta := metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()})
			got, err := reconcileService(t.Context(), client, kb, meta)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSelector, got.Spec.Selector)

			var inAPIServer corev1.Service
			require.NoError(t, client.Get(context.Background(),
				types.NamespacedName{Namespace: "default", Name: kbv1.HTTPService("test")}, &inAPIServer))
			assert.Equal(t, tt.wantSelector, inAPIServer.Spec.Selector)
		})
	}
}

func TestCalculateAggregateHealth(t *testing.T) {
	const (
		green  = commonv1.GreenHealth
		yellow = commonv1.YellowHealth
		red    = commonv1.RedHealth
	)
	tests := []struct {
		name string
		pr   commonv1.DeploymentHealth
		bg   commonv1.DeploymentHealth
		want commonv1.DeploymentHealth
	}{
		{name: "green + green = green", pr: green, bg: green, want: green},
		{name: "red + red = red", pr: red, bg: red, want: red},
		{name: "yellow + yellow = yellow", pr: yellow, bg: yellow, want: yellow},
		{name: "green + red = yellow", pr: green, bg: red, want: yellow},
		// A red primary pool means no UI pod serves requests: the aggregate is red regardless
		// of the background pool health.
		{name: "red + green = red", pr: red, bg: green, want: red},
		{name: "green + yellow = yellow", pr: green, bg: yellow, want: yellow},
		{name: "yellow + green = yellow", pr: yellow, bg: green, want: yellow},
		{name: "red + yellow = red", pr: red, bg: yellow, want: red},
		{name: "yellow + red = yellow", pr: yellow, bg: red, want: yellow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, calculateAggregateHealth(tt.pr, tt.bg))
		})
	}
}

func TestAllPodsHaveRoleLabel(t *testing.T) {
	selector := map[string]string{kblabel.KibanaNameLabelName: "test"}
	deployment := func(sel *metav1.LabelSelector) *appsv1.Deployment {
		return &appsv1.Deployment{
			Name: "test-kb", Namespace: "default",
			Spec: appsv1.DeploymentSpec{Selector: sel},
		}
	}
	pod := func(name, namespace string, labels map[string]string) *corev1.Pod {
		return &corev1.Pod{Name: name, Namespace: namespace, Labels: labels}
	}
	podInPhase := func(name, namespace string, labels map[string]string, phase corev1.PodPhase) *corev1.Pod {
		p := pod(name, namespace, labels)
		p.Status.Phase = phase
		return p
	}
	withRole := func(role string) map[string]string {
		l := maps.Clone(selector)
		l[kblabel.RoleLabelName] = role
		return l
	}

	tests := []struct {
		name    string
		dep     *appsv1.Deployment
		pods    []client.Object
		want    bool
		wantPod string
	}{
		{
			name: "nil selector",
			dep:  deployment(nil),
			pods: []client.Object{pod("p1", "default", withRole("primary"))},
			want: false,
		},
		{
			name: "no pods",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			want: true,
		},
		{
			name: "all pods have the role label",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			pods: []client.Object{
				pod("p1", "default", withRole("primary")),
				pod("p2", "default", withRole("primary")),
			},
			want: true,
		},
		{
			name: "one pod is missing the role label",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			pods: []client.Object{
				pod("p1", "default", withRole("primary")),
				pod("p2", "default", selector),
			},
			want:    false,
			wantPod: "p2",
		},
		{
			name: "failed pod missing the role label is ignored",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			pods: []client.Object{
				pod("p1", "default", withRole("primary")),
				podInPhase("p2", "default", selector, corev1.PodFailed),
			},
			want: true,
		},
		{
			name: "succeeded pod missing the role label is ignored",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			pods: []client.Object{
				pod("p1", "default", withRole("primary")),
				podInPhase("p2", "default", selector, corev1.PodSucceeded),
			},
			want: true,
		},
		{
			name: "running pod missing the role label is not ignored",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			pods: []client.Object{
				pod("p1", "default", withRole("primary")),
				podInPhase("p2", "default", selector, corev1.PodRunning),
			},
			want:    false,
			wantPod: "p2",
		},
		{
			name: "pods not matching the selector are ignored",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			pods: []client.Object{
				pod("p1", "default", withRole("primary")),
				pod("other", "default", map[string]string{kblabel.KibanaNameLabelName: "other"}),
			},
			want: true,
		},
		{
			name: "pods in other namespaces are ignored",
			dep:  deployment(&metav1.LabelSelector{MatchLabels: selector}),
			pods: []client.Object{
				pod("p1", "default", withRole("primary")),
				pod("p2", "other-ns", selector),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.pods...)
			got, blockingPod, err := allPodsHaveRoleLabel(context.Background(), c, tt.dep)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantPod, blockingPod)
		})
	}
}

func TestDeletePoolDeployment(t *testing.T) {
	newDeployment := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			Name: "test-kb", Namespace: "default", UID: "uid-1", Finalizers: []string{metav1.FinalizerOrphanDependents},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{kblabel.KibanaNameLabelName: "test"}},
			},
		}
	}
	kb := &kbv1.Kibana{Name: "test", Namespace: "default"}

	t.Run("deletes the deployment and emits an event", func(t *testing.T) {
		dep := newDeployment()
		d := newTestDriver(t, dep)
		recorder, is := d.recorder.(*toolsevents.FakeRecorder)
		require.True(t, is, "recorder should be of type *toolsevents.FakeRecorder")

		result, err := d.deletePoolDeployment(context.Background(), kb, nil, dep)
		require.NoError(t, err)
		assert.Equal(t, requeue, result)
		assert.Len(t, recorder.Events, 1)

		// the finalizer keeps the object around, but it must now be marked for deletion
		var got appsv1.Deployment
		require.NoError(t, d.client.Get(context.Background(), k8s.ExtractNamespacedName(dep), &got))
		assert.False(t, got.DeletionTimestamp.IsZero())
	})

	t.Run("deployment that is already gone is not an error", func(t *testing.T) {
		d := newTestDriver(t) // no deployment in the cluster
		result, err := d.deletePoolDeployment(context.Background(), kb, nil, newDeployment())
		require.NoError(t, err)
		assert.Equal(t, requeue, result)
	})
}

func TestDeploymentParams_RevisionHistoryLimit(t *testing.T) {
	tests := []struct {
		name      string
		specLimit *int32
		overwrite *int32
		want      *int32
	}{
		{name: "nothing set", want: nil},
		{name: "spec value used when there is no overwrite", specLimit: ptr.To[int32](5), want: ptr.To[int32](5)},
		{name: "overwrite used when the spec has no value", overwrite: ptr.To[int32](0), want: ptr.To[int32](0)},
		{name: "overwrite takes precedence over the spec value", specLimit: ptr.To[int32](5), overwrite: ptr.To[int32](0), want: ptr.To[int32](0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := kibanaFixture()
			kb.Spec.RevisionHistoryLimit = tt.specLimit
			d := newTestDriver(t, defaultInitialObjects()...)

			got, err := d.deploymentParams(
				context.Background(), kb,
				poolParameters{Role: kblabel.SinglePoolRole, SecretName: kbv1.ConfigSecret(kb.Name), DeploymentName: kbv1.KBNamer.Suffix(kb.Name)},
				new(kb.Spec.Count), nil, nil,
				metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}),
				selectorChangeOutcome{expectedSelector: kb.GetIdentityLabels(), revisionHistoryLimitOverride: tt.overwrite},
			)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.RevisionHistoryLimit)
		})
	}
}
