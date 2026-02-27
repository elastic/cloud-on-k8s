// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newBucketConfig(t *testing.T) {
	ctx := map[string]any{"ClusterName": "my-cluster"}

	plan := Plan{
		Id:          "test",
		ClusterName: "my-cluster",
		Bucket: &BucketSettings{
			Name:         "{{ .ClusterName }}-dev",
			StorageClass: "STANDARD",
			Secret: BucketSecretSettings{
				Name:      "my-secret",
				Namespace: "default",
			},
		},
	}

	cfg, err := newBucketConfig(plan, ctx, "eu-west-2")
	require.NoError(t, err)
	assert.Equal(t, "my-cluster-dev", cfg.Name)
	assert.Equal(t, "STANDARD", cfg.StorageClass)
	assert.Equal(t, "eu-west-2", cfg.Region)
	assert.Equal(t, "my-secret", cfg.SecretName)
	assert.Equal(t, "default", cfg.SecretNamespace)
	assert.Equal(t, "my-cluster", cfg.Labels["cluster_name"])
	assert.Equal(t, "test", cfg.Labels["plan_id"])
	assert.Equal(t, "eck-deployer", cfg.Labels["managed_by"])
}
