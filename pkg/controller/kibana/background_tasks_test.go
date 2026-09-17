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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
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
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       kbv1.KibanaSpec{Version: "8.17.0"},
	}
	d, err := newDriver(k8s.NewFakeClient(objects...), watches.NewDynamicWatches(), toolsevents.NewFakeRecorder(10), kb, corev1.IPv4Protocol)
	require.NoError(t, err)
	return d
}

// ---- WithPoolOverlay --------------------------------------------------------

func TestWithPoolOverlay(t *testing.T) {
	// Use simple non-dotted keys — ucfg treats dots as path separators so dotted
	// keys in map literals produce nested structures, not flat string keys.
	base := CanonicalConfig{settings.MustCanonicalConfig(map[string]interface{}{
		"basekey": "basevalue",
		"shared":  "original",
	})}

	t.Run("nil overlay returns base unchanged", func(t *testing.T) {
		got, err := base.WithPoolOverlay(nil)
		require.NoError(t, err)
		// Identity: nil overlay returns the original config, not a copy.
		assert.Equal(t, base.CanonicalConfig, got.CanonicalConfig)
	})

	t.Run("overlay adds new key while preserving base", func(t *testing.T) {
		overlay := &commonv1.Config{Data: map[string]interface{}{"newkey": "newvalue"}}
		got, err := base.WithPoolOverlay(overlay)
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
		overlay := &commonv1.Config{Data: map[string]interface{}{"shared": "overridden"}}
		got, err := base.WithPoolOverlay(overlay)
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
		overlay := &commonv1.Config{Data: map[string]interface{}{"extra": "v"}}
		got, err := base.WithPoolOverlay(overlay)
		require.NoError(t, err)

		rendered, err := got.Render()
		require.NoError(t, err)
		assert.NotContains(t, string(rendered), "node.roles")
	})

	t.Run("base config is not mutated by overlay", func(t *testing.T) {
		originalRendered, err := base.Render()
		require.NoError(t, err)

		overlay := &commonv1.Config{Data: map[string]interface{}{"shared": "overridden"}}
		_, err = base.WithPoolOverlay(overlay)
		require.NoError(t, err)

		afterRendered, err := base.Render()
		require.NoError(t, err)
		assert.Equal(t, string(originalRendered), string(afterRendered), "base must not be mutated")
	})
}

// ---- poolParams -------------------------------------------------------------

func TestPoolParams(t *testing.T) {
	baseCfg := CanonicalConfig{settings.MustCanonicalConfig(map[string]interface{}{"server.host": "0.0.0.0"})}

	t.Run("empty role returns base config and base names", func(t *testing.T) {
		kb := &kbv1.Kibana{ObjectMeta: metav1.ObjectMeta{Name: "mykb"}, Spec: kbv1.KibanaSpec{Version: "8.17.0"}}
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
		overlay := &commonv1.Config{Data: map[string]interface{}{"xpack.extra": "v"}}
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
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"base-label": "base"},
			Annotations: map[string]string{"base-ann": "base"},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"base-node": "sel"},
			Tolerations:  []corev1.Toleration{{Key: "base-tol"}},
			Containers: []corev1.Container{
				{Name: "kibana", Image: "base-image", Command: []string{"base"}},
				{Name: "sidecar", Image: "sidecar-image"},
			},
			Volumes: []corev1.Volume{
				{Name: "vol-a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}

	t.Run("empty pool returns base unchanged", func(t *testing.T) {
		got := mergePoolPodTemplate(base, corev1.PodTemplateSpec{})
		assert.Equal(t, base, got)
	})

	t.Run("nodeSelector from pool replaces base", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: map[string]string{"pool-node": "sel"}}}
		got := mergePoolPodTemplate(base, pool)
		assert.Equal(t, map[string]string{"pool-node": "sel"}, got.Spec.NodeSelector)
		// Base toleration still intact.
		assert.Equal(t, base.Spec.Tolerations, got.Spec.Tolerations)
	})

	t.Run("tolerations from pool replace base", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "pool-tol"}}}}
		got := mergePoolPodTemplate(base, pool)
		require.Len(t, got.Spec.Tolerations, 1)
		assert.Equal(t, "pool-tol", got.Spec.Tolerations[0].Key)
		// Base nodeSelector still intact.
		assert.Equal(t, base.Spec.NodeSelector, got.Spec.NodeSelector)
	})

	t.Run("affinity from pool wins; base fields not in pool survive", func(t *testing.T) {
		aff := &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Affinity: aff}}
		got := mergePoolPodTemplate(base, pool)
		assert.Equal(t, aff, got.Spec.Affinity)
		assert.Equal(t, base.Spec.NodeSelector, got.Spec.NodeSelector)
	})

	t.Run("labels are merged not replaced", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"pool-label": "pool"}}}
		got := mergePoolPodTemplate(base, pool)
		assert.Equal(t, "base", got.Labels["base-label"], "base label must survive")
		assert.Equal(t, "pool", got.Labels["pool-label"], "pool label must be added")
	})

	t.Run("annotations are merged not replaced", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"pool-ann": "pool"}}}
		got := mergePoolPodTemplate(base, pool)
		assert.Equal(t, "base", got.Annotations["base-ann"], "base annotation must survive")
		assert.Equal(t, "pool", got.Annotations["pool-ann"], "pool annotation must be added")
	})

	t.Run("pool label overwrites base label with same key", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"base-label": "overridden"}}}
		got := mergePoolPodTemplate(base, pool)
		assert.Equal(t, "overridden", got.Labels["base-label"])
	})

	t.Run("container with same name replaces base container", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "kibana", Image: "pool-image", Command: []string{"pool"}}},
		}}
		got := mergePoolPodTemplate(base, pool)
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
		got := mergePoolPodTemplate(base, pool)
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
			Volumes: []corev1.Volume{{Name: "vol-a", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/new"}}}},
		}}
		got := mergePoolPodTemplate(base, pool)
		require.Len(t, got.Spec.Volumes, 1)
		assert.NotNil(t, got.Spec.Volumes[0].HostPath)
		assert.Equal(t, "/new", got.Spec.Volumes[0].HostPath.Path)
	})

	t.Run("novel volume in pool is appended", func(t *testing.T) {
		pool := corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{Name: "vol-b", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
		}}
		got := mergePoolPodTemplate(base, pool)
		assert.Len(t, got.Spec.Volumes, 2)
	})

	t.Run("base is not mutated", func(t *testing.T) {
		original := base.DeepCopy()
		pool := corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"x": "y"}},
			Spec:       corev1.PodSpec{NodeSelector: map[string]string{"x": "y"}},
		}
		_ = mergePoolPodTemplate(base, pool)
		assert.Equal(t, *original, base)
	})
}

// ---- GC methods -------------------------------------------------------------

func TestGarbageCollectBackgroundResources(t *testing.T) {
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

	kb := &kbv1.Kibana{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}

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
	kb := &kbv1.Kibana{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}

	t.Run("deletes BG config secret when it exists", func(t *testing.T) {
		sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: bgSecretName, Namespace: "default"}}
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
			)
			require.NoError(t, err)

			env := findKibanaContainerEnv(params.PodTemplateSpec.Spec)
			nodeRoles, found := findEnvVar(env, nodeRolesEnvVarName)
			assert.Equal(t, tc.wantPresent, found, "NODE_ROLES presence mismatch")
			if tc.wantPresent {
				assert.Equal(t, tc.wantNodeRoles, nodeRoles.Value)
				// Verify it is the first env var so it can be overridden by user-supplied vars.
				assert.Equal(t, nodeRolesEnvVarName, env[0].Name, "NODE_ROLES must be first env var")
			}
		})
	}
}

// ---- Service selector -------------------------------------------------------

func TestNewService_BackgroundTasksSplit(t *testing.T) {
	t.Run("split disabled: selector has no role label", func(t *testing.T) {
		kb := kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{Name: "mykb", Namespace: "default"},
			Spec:       kbv1.KibanaSpec{Version: "8.17.0"},
		}
		svc := NewService(kb, metadata.Propagate(&kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}))
		_, hasUILabel := svc.Spec.Selector[kblabel.UIRoleLabelName]
		assert.False(t, hasUILabel, "role-ui label must not appear when split is off")
		assert.Equal(t, "mykb", svc.Spec.Selector[kblabel.KibanaNameLabelName])
	})

	t.Run("split enabled: selector targets UI-role pods only", func(t *testing.T) {
		kb := kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{Name: "mykb", Namespace: "default"},
			Spec: kbv1.KibanaSpec{
				Version:         "8.17.0",
				BackgroundTasks: &kbv1.KibanaBackgroundTasks{},
			},
		}
		svc := NewService(kb, metadata.Propagate(&kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}))
		assert.Equal(t, kblabel.RoleLabelValue, svc.Spec.Selector[kblabel.UIRoleLabelName], "role-ui must be in selector")
		_, hasBGLabel := svc.Spec.Selector[kblabel.BackgroundTasksRoleLabelName]
		assert.False(t, hasBGLabel, "background_tasks label must NOT be in service selector")
	})
}

// ---- Deployment selector labels (via GetPoolIdentityLabels) -----------------

func TestGetPoolIdentityLabels_SplitOnOff(t *testing.T) {
	t.Run("split disabled: both pools use identity labels only", func(t *testing.T) {
		kb := &kbv1.Kibana{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
		labels := kb.GetPoolIdentityLabels(kblabel.UIRole)
		_, hasUI := labels[kblabel.UIRoleLabelName]
		_, hasBG := labels[kblabel.BackgroundTasksRoleLabelName]
		assert.False(t, hasUI)
		assert.False(t, hasBG)
	})

	t.Run("split enabled: UI pool selector includes role-ui", func(t *testing.T) {
		kb := &kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{Name: "x"},
			Spec:       kbv1.KibanaSpec{BackgroundTasks: &kbv1.KibanaBackgroundTasks{}},
		}
		uiLabels := kb.GetPoolIdentityLabels(kblabel.UIRole)
		assert.Equal(t, kblabel.RoleLabelValue, uiLabels[kblabel.UIRoleLabelName])
		_, hasBG := uiLabels[kblabel.BackgroundTasksRoleLabelName]
		assert.False(t, hasBG, "UI selector must not include BG label")

		bgLabels := kb.GetPoolIdentityLabels(kblabel.BackgroundTasksRole)
		assert.Equal(t, kblabel.RoleLabelValue, bgLabels[kblabel.BackgroundTasksRoleLabelName])
		_, hasUI := bgLabels[kblabel.UIRoleLabelName]
		assert.False(t, hasUI, "BG selector must not include UI label")
	})
}
