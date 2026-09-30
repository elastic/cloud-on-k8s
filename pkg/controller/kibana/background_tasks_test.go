// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolsevents "k8s.io/client-go/tools/events"
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
	baseCfg := CanonicalConfig{settings.MustCanonicalConfig(map[string]any{"server.host": "0.0.0.0"})}

	t.Run("empty role returns base config and base names", func(t *testing.T) {
		kb := &kbv1.Kibana{Name: "mykb", Spec: kbv1.KibanaSpec{Version: "8.17.0"}}
		d := &driver{}
		cfg, secretName, deployName, err := d.poolParams(kb, kblabel.Role{}, baseCfg)
		require.NoError(t, err)
		assert.Equal(t, baseCfg.CanonicalConfig, cfg.CanonicalConfig)
		assert.Equal(t, kbv1.ConfigSecret("mykb"), secretName)
		assert.Equal(t, kbv1.KBNamer.Suffix("mykb"), deployName)
	})

	t.Run("UIRole returns base config and base names", func(t *testing.T) {
		kb := kbWithBG(nil, nil)
		d := &driver{}
		cfg, secretName, deployName, err := d.poolParams(kb, kblabel.UIRole, baseCfg)
		require.NoError(t, err)
		assert.Equal(t, baseCfg.CanonicalConfig, cfg.CanonicalConfig)
		assert.Equal(t, kbv1.ConfigSecret("test"), secretName)
		assert.Equal(t, kbv1.KBNamer.Suffix("test"), deployName)
	})

	t.Run("BackgroundTasksRole without overlay shares base secret", func(t *testing.T) {
		kb := kbWithBG(nil, nil)
		d := &driver{}
		cfg, secretName, deployName, err := d.poolParams(kb, kblabel.BackgroundTasksRole, baseCfg)
		require.NoError(t, err)
		// Same config pointer — no deep copy done for the no-overlay case.
		assert.Equal(t, baseCfg.CanonicalConfig, cfg.CanonicalConfig)
		// Uses shared base secret, not a separate BG secret.
		assert.Equal(t, kbv1.ConfigSecret("test"), secretName)
		assert.Equal(t, kbv1.BackgroundTasksDeployment("test"), deployName)
	})

	t.Run("BackgroundTasksRole with overlay gets dedicated secret", func(t *testing.T) {
		overlay := &commonv1.Config{Data: map[string]any{"xpack.extra": "v"}}
		kb := kbWithBG(nil, overlay)
		d := &driver{}
		_, secretName, deployName, err := d.poolParams(kb, kblabel.BackgroundTasksRole, baseCfg)
		require.NoError(t, err)
		assert.Equal(t, kbv1.BackgroundTasksConfigSecret("test"), secretName)
		assert.Equal(t, kbv1.BackgroundTasksDeployment("test"), deployName)
	})
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
		kblabel.RoleLabelName:        kblabel.RolePrimeValue,
	}
	legacyDp := func() *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: dpName, Namespace: "default"},
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
			ObjectMeta: metav1.ObjectMeta{Name: bgDpName, Namespace: "default"},
		}
	}
	existingBGSecret := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: bgSecretName, Namespace: "default"},
		}
	}

	kbPaused := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test", Namespace: "default",
			Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "true"},
		},
	}
	kbActive := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
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
				nil,
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

func TestNewService_BackgroundTasksSplit(t *testing.T) {
	t.Run("split disabled: service selector has role=prime (single-pool)", func(t *testing.T) {
		kb := kbv1.Kibana{
			Name: "mykb", Namespace: "default",
			Spec: kbv1.KibanaSpec{Version: "8.17.0"},
		}
		svc := NewService(kb, metadata.Propagate(&kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}))
		assert.Equal(t, kblabel.RolePrimeValue, svc.Spec.Selector[kblabel.RoleLabelName],
			"service always selects role=prime pods; the single pool carries that label too")
		assert.Equal(t, "mykb", svc.Spec.Selector[kblabel.KibanaNameLabelName])
	})

	t.Run("split enabled: selector targets UI-role (prime) pods only", func(t *testing.T) {
		kb := kbv1.Kibana{
			Name: "mykb", Namespace: "default",
			Spec: kbv1.KibanaSpec{
				Version:         "8.17.0",
				BackgroundTasks: &kbv1.KibanaBackgroundTasks{},
			},
		}
		svc := NewService(kb, metadata.Propagate(&kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}))
		assert.Equal(t, kblabel.RolePrimeValue, svc.Spec.Selector[kblabel.RoleLabelName], "role=prime must be in selector")
	})
}

// ---- Deployment selector labels (via GetPoolIdentityLabels) -----------------

func TestGetPoolIdentityLabels_SplitOnOff(t *testing.T) {
	t.Run("split disabled: single pool always gets role=prime", func(t *testing.T) {
		kb := &kbv1.Kibana{Name: "x"}
		labels := kb.GetPoolIdentityLabels(kblabel.UIRole)
		assert.Equal(t, kblabel.RolePrimeValue, labels[kblabel.RoleLabelName])
	})

	t.Run("split enabled: UI selector gets prime, BG selector gets background_tasks", func(t *testing.T) {
		kb := &kbv1.Kibana{
			Name: "x",
			Spec: kbv1.KibanaSpec{BackgroundTasks: &kbv1.KibanaBackgroundTasks{}},
		}
		uiLabels := kb.GetPoolIdentityLabels(kblabel.UIRole)
		assert.Equal(t, kblabel.RolePrimeValue, uiLabels[kblabel.RoleLabelName])

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

// ---- upgradeStopNeeded ------------------------------------------------------

func makePod(name, version string, role kblabel.Role) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				kblabel.KibanaVersionLabelName: version,
				kblabel.RoleLabelName:          role.LabelValue,
			},
		},
	}
}

func TestUpgradeStopNeeded(t *testing.T) {
	const (
		old = "8.17.0"
		new = "8.18.0"
	)

	kbSingle := kbv1.Kibana{Spec: kbv1.KibanaSpec{Version: new}}
	kbSplit := kbv1.Kibana{Spec: kbv1.KibanaSpec{
		Version:         new,
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
			pods: []corev1.Pod{makePod("kb-0", old, kblabel.SinglePoolRole)},
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
				makePod("kb-ui-0", new, kblabel.UIRole),
				makePod("kb-bg-0", new, kblabel.BackgroundTasksRole),
			},
			want: false,
		},
		{
			name: "split, both pools stale: needed",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePod("kb-ui-0", old, kblabel.UIRole),
				makePod("kb-bg-0", old, kblabel.BackgroundTasksRole),
			},
			want: true,
		},
		{
			// Unequal shutdown times: UI pool drained first, BG pod still terminating.
			// The stop phase must remain active until ALL stale pods are gone.
			name: "split, UI pod gone but BG pod still stale: needed",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePod("kb-bg-0", old, kblabel.BackgroundTasksRole),
			},
			want: true,
		},
		{
			// Mirror case: BG pool drained first, UI pod still terminating.
			name: "split, BG pod gone but UI pod still stale: needed",
			kb:   kbSplit,
			pods: []corev1.Pod{
				makePod("kb-ui-0", old, kblabel.UIRole),
			},
			want: true,
		},
		{
			name: "split, pod with missing version label: needed (treated as stale)",
			kb:   kbSplit,
			pods: []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "kb-ui-0"}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, upgradeStopNeeded(&tt.kb, tt.pods))
		})
	}
}
