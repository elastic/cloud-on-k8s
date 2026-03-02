// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"maps"
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/bucket"
)

func createBucketIfConfigured(plan Plan, newManager func() (bucket.Manager, error)) error {
	if plan.Bucket == nil {
		return nil
	}
	mgr, err := newManager()
	if err != nil {
		return err
	}
	return mgr.Create()
}

func deleteBucketIfConfigured(plan Plan, newManager func() (bucket.Manager, error)) error {
	if plan.Bucket == nil {
		return nil
	}
	mgr, err := newManager()
	if err != nil {
		return err
	}
	return mgr.Delete()
}

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
	if err := bucket.ValidateName(name, "bucket name"); err != nil {
		return bucket.Config{}, err
	}

	secretName, err := bucket.ResolveName(plan.Bucket.Secret.Name, ctx)
	if err != nil {
		return bucket.Config{}, fmt.Errorf("while resolving secret name: %w", err)
	}
	if err := bucket.ValidateName(secretName, "secret name"); err != nil {
		return bucket.Config{}, err
	}

	secretNamespace, err := bucket.ResolveName(plan.Bucket.Secret.Namespace, ctx)
	if err != nil {
		return bucket.Config{}, fmt.Errorf("while resolving secret namespace: %w", err)
	}
	if err := bucket.ValidateName(secretNamespace, "secret namespace"); err != nil {
		return bucket.Config{}, err
	}

	// Validate label values that will be interpolated into shell commands (cloud CLI --labels/--tags flags).
	if err := bucket.ValidateName(plan.ClusterName, "cluster name"); err != nil {
		return bucket.Config{}, err
	}
	if err := bucket.ValidateName(plan.Id, "plan ID"); err != nil {
		return bucket.Config{}, err
	}
	if err := bucket.ValidateName(region, "region"); err != nil {
		return bucket.Config{}, err
	}

	labels := make(map[string]string)
	maps.Copy(labels, elasticTags)
	labels["cluster_name"] = plan.ClusterName
	labels["plan_id"] = plan.Id
	labels["managed_by"] = "eck-deployer"

	cfg := bucket.Config{
		Name:            name,
		Labels:          labels,
		Region:          region,
		SecretName:      secretName,
		SecretNamespace: secretNamespace,
	}

	return cfg, nil
}

// newLocalGCSBucketManager creates a GCS bucket manager for local drivers (Kind, K3D)
// that don't have a GCP project in their plan configuration.
func newLocalGCSBucketManager(plan Plan) (*bucket.GCSManager, error) {
	ctx := map[string]any{
		"ClusterName": plan.ClusterName,
		"PlanId":      plan.Id,
	}
	project, err := exec.NewCommand(`gcloud config get-value project`).WithoutStreaming().Output()
	if err != nil {
		return nil, fmt.Errorf("while getting GCP project for bucket: %w (ensure gcloud is authenticated)", err)
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, fmt.Errorf("no GCP project configured; run 'gcloud config set project <PROJECT>' first")
	}
	if err := bucket.ValidateShellArg(project, "GCP project"); err != nil {
		return nil, err
	}

	region := plan.Bucket.Region
	if region == "" {
		region = "us-central1"
	}

	if plan.Bucket.StorageClass != "" {
		if err := bucket.ValidateShellArg(plan.Bucket.StorageClass, "storage class"); err != nil {
			return nil, err
		}
	}

	cfg, err := newBucketConfig(plan, ctx, region)
	if err != nil {
		return nil, err
	}
	return bucket.NewGCSManager(cfg, project, plan.Bucket.StorageClass), nil
}
