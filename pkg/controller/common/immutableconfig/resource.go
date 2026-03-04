// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// ConfigTypeLabelName distinguishes immutable config resources from dynamic (hot-reloadable) resources.
	ConfigTypeLabelName = "common.k8s.elastic.co/config-type"
	// ConfigHashLabelName stores a truncated SHA-256 hash of the immutable config content for debugging.
	ConfigHashLabelName = "common.k8s.elastic.co/config-hash"
	// ConfigTypeImmutable is the value of ConfigTypeLabelName for revision-specific immutable config resources.
	ConfigTypeImmutable = "immutable"
	// ConfigTypeDynamic is the value of ConfigTypeLabelName for the stable, mutable resource containing hot-reloadable settings.
	ConfigTypeDynamic = "dynamic"

	// hashLabelLen is the length of the hash stored in the ConfigHashLabelName label (longer than name suffix for debugging).
	hashLabelLen = 12
)

// BuildImmutableSecret creates an immutable Secret with a content-hash suffix in its name.
// The returned Secret has:
//   - Name: "{baseName}-{shortHash}" where shortHash is the first 8 chars of the content hash
//   - Labels: includes ConfigTypeLabelName=immutable and ConfigHashLabelName with truncated hash
//   - Immutable: true
func BuildImmutableSecret(baseName, namespace string, data map[string][]byte, labels map[string]string) corev1.Secret {
	fullHash := ComputeContentHash(data)
	name := ImmutableName(baseName, fullHash)

	secretLabels := maps.Merge(labels, map[string]string{
		ConfigTypeLabelName: ConfigTypeImmutable,
		ConfigHashLabelName: ShortHash(fullHash, hashLabelLen),
	})

	immutable := true
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    secretLabels,
		},
		Immutable: &immutable,
		Data:      data,
	}
}

// BuildImmutableConfigMap creates an immutable-like ConfigMap with a content-hash suffix in its name.
// Note: ConfigMaps don't have an immutable field in older K8s versions, but we use the same naming pattern.
// The returned ConfigMap has:
//   - Name: "{baseName}-{shortHash}" where shortHash is the first 8 chars of the content hash
//   - Labels: includes ConfigTypeLabelName=immutable and ConfigHashLabelName with truncated hash
//   - Immutable: true (for K8s 1.21+)
func BuildImmutableConfigMap(baseName, namespace string, data map[string]string, labels map[string]string) corev1.ConfigMap {
	fullHash := ComputeStringContentHash(data)
	name := ImmutableName(baseName, fullHash)

	cmLabels := maps.Merge(labels, map[string]string{
		ConfigTypeLabelName: ConfigTypeImmutable,
		ConfigHashLabelName: ShortHash(fullHash, hashLabelLen),
	})

	immutable := true
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    cmLabels,
		},
		Immutable: &immutable,
		Data:      data,
	}
}

// BuildDynamicSecretLabels returns labels for a dynamic (hot-reloadable) config secret.
func BuildDynamicSecretLabels(labels map[string]string) map[string]string {
	return maps.Merge(labels, map[string]string{
		ConfigTypeLabelName: ConfigTypeDynamic,
	})
}
