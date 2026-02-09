// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/bucket"
)

// newBucketConfig builds a bucket.Config from a plan and driver context.
// The bucket name and secret name/namespace are resolved from template variables.
func newBucketConfig(plan Plan, ctx map[string]any, region string) (bucket.Config, error) {
	if plan.Bucket == nil {
		return bucket.Config{}, fmt.Errorf("no bucket configuration in plan")
	}

	name, err := bucket.ResolveName(plan.Bucket.Name, ctx)
	if err != nil {
		return bucket.Config{}, fmt.Errorf("while resolving bucket name: %w", err)
	}

	secretName, err := bucket.ResolveName(plan.Bucket.Secret.Name, ctx)
	if err != nil {
		return bucket.Config{}, fmt.Errorf("while resolving secret name: %w", err)
	}

	secretNamespace, err := bucket.ResolveName(plan.Bucket.Secret.Namespace, ctx)
	if err != nil {
		return bucket.Config{}, fmt.Errorf("while resolving secret namespace: %w", err)
	}

	labels := make(map[string]string)
	for k, v := range elasticTags {
		labels[k] = v
	}
	labels["cluster_name"] = plan.ClusterName
	labels["plan_id"] = plan.Id
	labels["managed_by"] = "eck-deployer"

	cfg := bucket.Config{
		Name:            name,
		StorageClass:    plan.Bucket.StorageClass,
		Labels:          labels,
		Region:          region,
		SecretName:      secretName,
		SecretNamespace: secretNamespace,
	}

	// Wire provider-specific settings
	if plan.Bucket.S3 != nil {
		cfg.S3 = bucket.S3Config{
			IAMUserPath:      plan.Bucket.S3.IamUserPath,
			ManagedPolicyARN: plan.Bucket.S3.ManagedPolicyARN,
		}
	}

	return cfg, nil
}
