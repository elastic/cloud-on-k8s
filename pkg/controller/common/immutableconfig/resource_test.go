// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

func TestBuildImmutableSecret(t *testing.T) {
	data := map[string][]byte{
		"config.yml": []byte("key: value"),
	}
	labels := map[string]string{
		"app": "elasticsearch",
	}

	secret := BuildImmutableSecret("my-config", "default", data, labels)

	// Check name has content-addressed suffix
	assert.Contains(t, secret.Name, "my-config-")
	assert.Greater(t, len(secret.Name), len("my-config-"))

	// Check namespace
	assert.Equal(t, "default", secret.Namespace)

	// Check data
	assert.Equal(t, data, secret.Data)

	// Check immutable flag
	require.NotNil(t, secret.Immutable)
	assert.True(t, *secret.Immutable)

	// Check labels
	assert.Equal(t, "elasticsearch", secret.Labels["app"])
	assert.Equal(t, ConfigTypeImmutable, secret.Labels[ConfigTypeLabelName])
	assert.NotEmpty(t, secret.Labels[ConfigHashLabelName])
	assert.Len(t, secret.Labels[ConfigHashLabelName], hashLabelLen)
}

func TestBuildImmutableSecret_DeterministicName(t *testing.T) {
	data := map[string][]byte{"config.yml": []byte("content")}

	secret1 := BuildImmutableSecret("my-config", "default", data, nil)
	secret2 := BuildImmutableSecret("my-config", "default", data, nil)

	assert.Equal(t, secret1.Name, secret2.Name, "same content should produce same name")
}

func TestBuildImmutableSecret_DifferentContentDifferentName(t *testing.T) {
	data1 := map[string][]byte{"config.yml": []byte("content1")}
	data2 := map[string][]byte{"config.yml": []byte("content2")}

	secret1 := BuildImmutableSecret("my-config", "default", data1, nil)
	secret2 := BuildImmutableSecret("my-config", "default", data2, nil)

	assert.NotEqual(t, secret1.Name, secret2.Name, "different content should produce different names")
}

func TestBuildImmutableConfigMap(t *testing.T) {
	data := map[string]string{
		"script.sh": "#!/bin/bash\necho hello",
	}
	labels := map[string]string{
		"app": "elasticsearch",
	}

	cm := BuildImmutableConfigMap("my-scripts", "default", data, labels)

	// Check name has content-addressed suffix
	assert.Contains(t, cm.Name, "my-scripts-")
	assert.Greater(t, len(cm.Name), len("my-scripts-"))

	// Check that labels contain the discovery label with the expected value.
	assert.Equal(t, commonv1.LabelBasedDiscoveryLabelValue, cm.Labels[commonv1.LabelBasedDiscoveryLabelName])

	// Check namespace
	assert.Equal(t, "default", cm.Namespace)

	// Check data
	assert.Equal(t, data, cm.Data)

	// Check immutable flag
	require.NotNil(t, cm.Immutable)
	assert.True(t, *cm.Immutable)

	// Check labels
	assert.Equal(t, "elasticsearch", cm.Labels["app"])
	assert.Equal(t, ConfigTypeImmutable, cm.Labels[ConfigTypeLabelName])
	assert.NotEmpty(t, cm.Labels[ConfigHashLabelName])
}

func TestBuildDynamicSecretLabels(t *testing.T) {
	baseLabels := map[string]string{
		"app":     "elasticsearch",
		"cluster": "my-cluster",
	}

	labels := BuildDynamicSecretLabels(baseLabels)

	assert.Equal(t, "elasticsearch", labels["app"])
	assert.Equal(t, "my-cluster", labels["cluster"])
	assert.Equal(t, ConfigTypeDynamic, labels[ConfigTypeLabelName])
}
