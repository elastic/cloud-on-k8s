// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolsevents "k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
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

// ---- WithPoolOverlay --------------------------------------------------------

func TestWithOverlay(t *testing.T) {
	// Use simple non-dotted keys — ucfg treats dots as path separators so dotted
	// keys in map literals produce nested structures, not flat string keys.
	base := CanonicalConfig{settings.MustCanonicalConfig(map[string]any{
		"basekey": "basevalue",
		"shared":  "original",
	})}

	t.Run("nil overlay returns base unchanged", func(t *testing.T) {
		got, err := base.WithOverlay(nil)
		require.NoError(t, err)
		// Identity: nil overlay returns the original config, not a copy.
		assert.Equal(t, base.CanonicalConfig, got.CanonicalConfig)
	})

	t.Run("overlay adds new key while preserving base", func(t *testing.T) {
		overlay := &commonv1.Config{Data: map[string]any{"newkey": "newvalue"}}
		got, err := base.WithOverlay(overlay)
		require.NoError(t, err)

		rendered, err := got.Render()
		require.NoError(t, err)
		// All three keys must appear in the rendered YAML.
		assert.Contains(t, string(rendered), "basekey")
		assert.Contains(t, string(rendered), "basevalue")
		assert.Contains(t, string(rendered), "newkey")
		assert.Contains(t, string(rendered), "newvalue")
	})

	t.Run("overlay replaces existing key", func(t *testing.T) {
		overlay := &commonv1.Config{Data: map[string]any{"shared": "overridden"}}
		got, err := base.WithOverlay(overlay)
		require.NoError(t, err)

		rendered, err := got.Render()
		require.NoError(t, err)
		// "original" must be gone; "overridden" must appear.
		assert.Contains(t, string(rendered), "overridden")
		assert.NotContains(t, string(rendered), "original")
		// Base-only key must still be present.
		assert.Contains(t, string(rendered), "basekey")
	})

	t.Run("overlay does not inject node.roles into YAML", func(t *testing.T) {
		overlay := &commonv1.Config{Data: map[string]any{"extra": "v"}}
		got, err := base.WithOverlay(overlay)
		require.NoError(t, err)

		rendered, err := got.Render()
		require.NoError(t, err)
		assert.NotContains(t, string(rendered), "node.roles")
	})

	t.Run("base config is not mutated by overlay", func(t *testing.T) {
		originalRendered, err := base.Render()
		require.NoError(t, err)

		overlay := &commonv1.Config{Data: map[string]any{"shared": "overridden"}}
		_, err = base.WithOverlay(overlay)
		require.NoError(t, err)

		afterRendered, err := base.Render()
		require.NoError(t, err)
		assert.Equal(t, string(originalRendered), string(afterRendered), "base must not be mutated")
	})
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
			params, err := (&driver{}).poolParams(tt.kb, tt.role)
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

func TestHandleDeploymentSelectorMismatch(t *testing.T) {
	dpName := kbv1.KBNamer.Suffix("test")
	legacyLabels := map[string]string{
		kblabel.KibanaNameLabelName:  "test",
		"common.k8s.elastic.co/type": "kibana",
	}
	newSelector := map[string]string{
		kblabel.KibanaNameLabelName:  "test",
		"common.k8s.elastic.co/type": "kibana",
		kblabel.RoleLabelName:        kblabel.RolePrimaryValue,
	}
	legacyDp := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			Name: dpName, Namespace: "default",
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: legacyLabels},
			},
		}
	}

	kbPaused := kbWithBG(nil, nil)
	kbPaused.Annotations = map[string]string{commonv1.PauseOrchestrationAnnotation: "true"}
	kbActive := kbWithBG(nil, nil)

	t.Run("paused: mismatch detected but deployment not deleted", func(t *testing.T) {
		dp := legacyDp()
		fakeClient := k8s.NewFakeClient(dp)
		d := &driver{client: fakeClient, recorder: toolsevents.NewFakeRecorder(10)}

		res, err := d.handleDeploymentSelectorMismatch(context.Background(), kbPaused, dp, legacyLabels, newSelector, dpName)

		require.NoError(t, err)
		assert.True(t, res.mismatch)
		assert.False(t, res.requeue)
		var got appsv1.Deployment
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))
	})

	t.Run("not paused: mismatch triggers deletion and requeue", func(t *testing.T) {
		dp := legacyDp()
		fakeClient := k8s.NewFakeClient(dp)
		d := &driver{client: fakeClient, recorder: toolsevents.NewFakeRecorder(10)}

		res, err := d.handleDeploymentSelectorMismatch(context.Background(), kbActive, dp, legacyLabels, newSelector, dpName)

		require.NoError(t, err)
		assert.True(t, res.mismatch)
		assert.True(t, res.requeue)
		var got appsv1.Deployment
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got)))
	})

	t.Run("no mismatch: not handled, deployment untouched", func(t *testing.T) {
		dp := legacyDp()
		fakeClient := k8s.NewFakeClient(dp)
		d := &driver{client: fakeClient, recorder: toolsevents.NewFakeRecorder(10)}

		// selector equals existingMatchLabels — no mismatch
		res, err := d.handleDeploymentSelectorMismatch(context.Background(), kbActive, dp, legacyLabels, legacyLabels, dpName)

		require.NoError(t, err)
		assert.False(t, res.mismatch)
		assert.False(t, res.requeue)
		var got appsv1.Deployment
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))
	})

	t.Run("resume after pause: deletion happens on next call", func(t *testing.T) {
		dp := legacyDp()
		fakeClient := k8s.NewFakeClient(dp)
		d := &driver{client: fakeClient, recorder: toolsevents.NewFakeRecorder(10)}

		// First call: paused — deployment survives.
		res, err := d.handleDeploymentSelectorMismatch(context.Background(), kbPaused, dp, legacyLabels, newSelector, dpName)
		require.NoError(t, err)
		assert.True(t, res.mismatch)
		var got appsv1.Deployment
		assert.NoError(t, fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got))

		// Second call: not paused — deployment deleted.
		res, err = d.handleDeploymentSelectorMismatch(context.Background(), kbActive, dp, legacyLabels, newSelector, dpName)
		require.NoError(t, err)
		assert.True(t, res.mismatch)
		assert.True(t, res.requeue)
		assert.True(t, apierrors.IsNotFound(fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(dp), &got)))
	})
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
	kb := kibanaFixture()

	cases := []struct {
		name          string
		role          kblabel.Role
		wantNodeRoles string
		wantPresent   bool
	}{
		{
			name:        "empty role (single-pool mode) injects no NODE_ROLES",
			role:        kblabel.Role{},
			wantPresent: false,
		},
		{
			name:        "Single-pool mode injects no NODE_ROLES",
			role:        kblabel.SinglePoolRole,
			wantPresent: false,
		},
		{
			name:          "UIRole injects NODE_ROLES=[\"ui\"]",
			role:          kblabel.UIRole,
			wantNodeRoles: `["ui"]`,
			wantPresent:   true,
		},
		{
			name:          "BackgroundTasksRole injects NODE_ROLES=[\"background_tasks\"]",
			role:          kblabel.BackgroundTasksRole,
			wantNodeRoles: `["background_tasks"]`,
			wantPresent:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDriver(t, objs...)
			replicas := new(kb.Spec.Count)
			params, err := d.deploymentParams(
				context.Background(),
				kb,
				tc.role,
				kbv1.ConfigSecret(kb.Name),
				kbv1.KBNamer.Suffix(kb.Name),
				replicas,
				nil, // replicasAnnotationValue
				nil, // policyAnnotations
				"",
				true,
				"",
				metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}),
				kb.GetIdentityLabels(),
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

	driverWith := func(objects ...client.Object) *driver {
		return &driver{client: k8s.NewFakeClient(objects...)}
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
			var objects []client.Object
			if tt.existingSvc != nil {
				objects = append(objects, tt.existingSvc)
			}
			d := driverWith(objects...)

			sel, err := d.serviceSelector(context.Background(), tt.kb)
			require.NoError(t, err)

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

	dp := func(replicas *int32, annotations map[string]string) *appsv1.Deployment {
		d := &appsv1.Deployment{}
		d.Name = "test-dp"
		d.Annotations = annotations
		d.Spec.Replicas = replicas
		return d
	}
	ann := func(v string) map[string]string {
		return map[string]string{replicasAnnotationName: v}
	}

	// getBackgroundReplicas priority: Count > HPA replicas (> 0) > annotation.
	tests := []struct {
		name       string
		stopNeeded bool
		kb         *kbv1.Kibana
		role       kblabel.Role
		existingDp *appsv1.Deployment
		wantNew    *int32
		wantOld    *int32
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
			name:       "BG, Count nil, HPA value: HPA replicas win",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(4), nil),
			wantNew:    ptr(4),
		},
		{
			name:       "BG, Count nil, HPA zeroed, annotation: annotation wins as last resort",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(0), ann("5")),
			wantNew:    ptr(5),
		},
		{
			name:       "BG, Count nil, no HPA value, no annotation: returns nil",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(nil, nil),
			wantNew:    nil,
		},
		{
			name:       "BG, Count nil, HPA zeroed, malformed annotation: returns nil",
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(0), ann("not-a-number")),
			wantNew:    nil,
		},
		// ---- stop phase -----------------------------------------------------
		{
			name:       "stop, UI role: scales to zero, no oldValue",
			stopNeeded: true,
			kb:         kbSplit(ptr(2)),
			role:       kblabel.UIRole,
			existingDp: dp(ptr(3), nil),
			wantNew:    ptr(0),
			wantOld:    nil,
		},
		{
			name:       "stop, BG, explicit count: Count saved as oldValue",
			stopNeeded: true,
			kb:         kbSplit(ptr(2)),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(2), nil),
			wantNew:    ptr(0),
			wantOld:    ptr(2),
		},
		{
			name:       "stop, BG, explicit count ignores annotation: Count wins over annotation",
			stopNeeded: true,
			kb:         kbSplit(ptr(2)),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(0), ann("4")),
			wantNew:    ptr(0),
			wantOld:    ptr(2),
		},
		{
			name:       "stop, BG, Count nil, HPA value: HPA replicas saved as oldValue",
			stopNeeded: true,
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(4), nil),
			wantNew:    ptr(0),
			wantOld:    ptr(4),
		},
		{
			name:       "stop, BG, Count nil, HPA zeroed, annotation: annotation saved as oldValue",
			stopNeeded: true,
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(0), ann("4")),
			wantNew:    ptr(0),
			wantOld:    ptr(4),
		},
		{
			name:       "stop, BG, Count nil, no HPA, no annotation: oldValue is nil",
			stopNeeded: true,
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(nil, nil),
			wantNew:    ptr(0),
			wantOld:    nil,
		},
		{
			name:       "stop, BG, Count nil, HPA zeroed, malformed annotation: oldValue is nil",
			stopNeeded: true,
			kb:         kbSplit(nil),
			role:       kblabel.BackgroundTasksRole,
			existingDp: dp(ptr(0), ann("not-a-number")),
			wantNew:    ptr(0),
			wantOld:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &driver{}
			gotNew, gotOld := d.deploymentReplicas(logr.Discard(), tt.stopNeeded, tt.kb, tt.role, tt.existingDp)
			assert.Equal(t, tt.wantNew, gotNew)
			assert.Equal(t, tt.wantOld, gotOld)
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
				context.Background(), kb, tt.role,
				configSecret, dpName,
				replicas, tt.replicasAnnotation,
				nil, "", false, "",
				meta, kb.GetIdentityLabels(),
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

	primaryPoolStatus := commonv1.DeploymentStatus{
		Selector:       "kibana.k8s.elastic.co/role=primary",
		Count:          3,
		AvailableNodes: 3,
		Health:         commonv1.GreenHealth,
		Version:        "8.17.0",
	}
	bgPoolStatus := commonv1.DeploymentStatus{
		Selector:       "kibana.k8s.elastic.co/role=background_tasks",
		Count:          2,
		AvailableNodes: 1,
		Health:         commonv1.RedHealth,
		Version:        "8.17.0",
	}

	t.Run("single pool: pool status copied verbatim, pools cleared, aggregates ignored", func(t *testing.T) {
		kb := &kbv1.Kibana{Name: "test", Namespace: "default", Spec: kbv1.KibanaSpec{Version: "8.17.0", Count: 3}}
		state := &State{Kibana: kb}
		// simulate a leftover pools status from a previously enabled split
		state.Kibana.Status.Pools = &kbv1.KibanaPoolsStatuses{Primary: &kbv1.KibanaPoolStatus{Count: 1}}

		// aggregates intentionally diverge from the pool status to prove they are not applied
		buildStatus(state, kb, kblabel.SinglePoolRole, primaryPoolStatus, commonv1.RedHealth, 99, 99)

		assert.Equal(t, primaryPoolStatus, state.Kibana.Status.DeploymentStatus)
		assert.Nil(t, state.Kibana.Status.Pools)
	})

	t.Run("split, primary pool: pools.primary set and top level aggregated", func(t *testing.T) {
		kb := kbWithBG(ptr.To[int32](2), nil)
		state := &State{Kibana: kb}

		buildStatus(state, kb, kblabel.UIRole, primaryPoolStatus, commonv1.GreenHealth, 3, 3)

		require.NotNil(t, state.Kibana.Status.Pools)
		require.NotNil(t, state.Kibana.Status.Pools.Primary)
		assert.Equal(t, kbv1.KibanaPoolStatus{
			Selector:       primaryPoolStatus.Selector,
			Count:          3,
			AvailableNodes: 3,
			Health:         commonv1.GreenHealth,
		}, *state.Kibana.Status.Pools.Primary)
		assert.Nil(t, state.Kibana.Status.Pools.BackgroundTasks)

		// top level carries the aggregates and the role-less selector, version comes from the primary pool
		assert.Equal(t, commonv1.DeploymentStatus{
			Selector:       splitSelector,
			Count:          3,
			AvailableNodes: 3,
			Health:         commonv1.GreenHealth,
			Version:        "8.17.0",
		}, state.Kibana.Status.DeploymentStatus)
	})

	t.Run("split, both pools: top level sums counts and degrades health", func(t *testing.T) {
		kb := kbWithBG(ptr.To[int32](2), nil)
		state := &State{Kibana: kb}

		// simulate the driver loop: primary first, then background tasks with updated aggregates
		buildStatus(state, kb, kblabel.UIRole, primaryPoolStatus, commonv1.GreenHealth, 3, 3)
		buildStatus(state, kb, kblabel.BackgroundTasksRole, bgPoolStatus, commonv1.RedHealth, 4, 5)

		require.NotNil(t, state.Kibana.Status.Pools)
		assert.Equal(t, &kbv1.KibanaPoolStatus{
			Selector:       primaryPoolStatus.Selector,
			Count:          3,
			AvailableNodes: 3,
			Health:         commonv1.GreenHealth,
		}, state.Kibana.Status.Pools.Primary)
		assert.Equal(t, &kbv1.KibanaPoolStatus{
			Selector:       bgPoolStatus.Selector,
			Count:          2,
			AvailableNodes: 1,
			Health:         commonv1.RedHealth,
		}, state.Kibana.Status.Pools.BackgroundTasks)

		// the background tasks pool must not overwrite the primary-derived top-level status,
		// only the aggregate fields
		assert.Equal(t, commonv1.DeploymentStatus{
			Selector:       splitSelector,
			Count:          5,
			AvailableNodes: 4,
			Health:         commonv1.RedHealth,
			Version:        "8.17.0",
		}, state.Kibana.Status.DeploymentStatus)
	})

	t.Run("split, background pool only: primary pool status left unset", func(t *testing.T) {
		kb := kbWithBG(ptr.To[int32](2), nil)
		state := &State{Kibana: kb}

		buildStatus(state, kb, kblabel.BackgroundTasksRole, bgPoolStatus, commonv1.RedHealth, 1, 2)

		require.NotNil(t, state.Kibana.Status.Pools)
		assert.Nil(t, state.Kibana.Status.Pools.Primary)
		require.NotNil(t, state.Kibana.Status.Pools.BackgroundTasks)
		assert.Equal(t, commonv1.RedHealth, state.Kibana.Status.DeploymentStatus.Health)
		assert.Equal(t, int32(2), state.Kibana.Status.DeploymentStatus.Count)
		assert.Equal(t, int32(1), state.Kibana.Status.DeploymentStatus.AvailableNodes)
	})
}
