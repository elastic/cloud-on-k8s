// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
)

const (
	ConfigFilename  = "config.yml"
	ConfigMountPath = "/package-registry/config.yml"
)

func configSecretVolume(epr eprv1alpha1.PackageRegistry) volume.SecretVolume {
	return volume.NewSecretVolume(ConfigName(epr.Name), "config", ConfigMountPath, ConfigFilename, 0444)
}

func reconcileConfig(ctx context.Context, driver driver.Interface, epr eprv1alpha1.PackageRegistry, meta metadata.Metadata) (corev1.Secret, error) {
	cfg, err := newConfig(driver, epr)
	if err != nil {
		return corev1.Secret{}, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return corev1.Secret{}, err
	}

	// Reconcile the configuration in a secret
	expectedConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   epr.Namespace,
			Name:        ConfigName(epr.Name),
			Labels:      maps.Clone(meta.Labels),
			Annotations: meta.Annotations,
		},
		Data: map[string][]byte{
			ConfigFilename: cfgBytes,
		},
	}

	return reconciler.ReconcileSecret(ctx, driver.K8sClient(), expectedConfigSecret, &epr)
}

func newConfig(d driver.Interface, epr eprv1alpha1.PackageRegistry) (*settings.CanonicalConfig, error) {
	cfg := settings.NewCanonicalConfig()

	inlineUserCfg, err := inlineUserConfig(epr.Spec.Config)
	if err != nil {
		return cfg, err
	}

	refUserCfg, err := common.ParseConfigRef(d, &epr, epr.Spec.ConfigRef, ConfigFilename)
	if err != nil {
		return cfg, err
	}
	defaults := defaultConfig()

	err = cfg.MergeWith(defaults, inlineUserCfg, refUserCfg)
	return cfg, err
}

func inlineUserConfig(cfg *commonv1.Config) (*settings.CanonicalConfig, error) {
	if cfg == nil {
		cfg = &commonv1.Config{}
	}
	return settings.NewCanonicalConfigFrom(cfg.Data)
}

func defaultConfig() *settings.CanonicalConfig {
	return settings.MustCanonicalConfig(map[string]interface{}{
		"package_paths": []string{"/packages/package-registry", "/packages/package-storage"},
		"cache_time": map[string]interface{}{
			"index":      "10s",
			"search":     "10m",
			"categories": "10m",
			"catch_all":  "10m",
		},
	})
}
