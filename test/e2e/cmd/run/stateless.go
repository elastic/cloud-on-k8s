// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package run

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

// Annotation keys set by the deployer's bucket managers.
// These must match the constants in hack/deployer/runner/bucket/bucket.go.
const (
	bucketAnnotationProvider       = "eck-deployer/provider"
	bucketAnnotationBucket         = "eck-deployer/bucket"
	bucketAnnotationStorageAccount = "eck-deployer/storage-account"
	bucketAnnotationContainer      = "eck-deployer/container"
)

// initStatelessConfig reads the bucket Secret and populates Provider, Bucket, and StorageAccount
// in the StatelessConfig. Called from initTestContext when stateless is enabled.
func (h *helper) initStatelessConfig() error {
	secretName := h.testContext.Stateless.SecretName
	sourceNS := h.testContext.Stateless.SecretNamespace

	if secretName == "" || sourceNS == "" {
		return fmt.Errorf("stateless enabled but secret not configured: secretName=%q, secretNamespace=%q", secretName, sourceNS)
	}

	log.Info("Reading stateless bucket configuration from secret",
		"secret", secretName,
		"namespace", sourceNS)

	client, err := h.createKubeClient()
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes client")
	}

	sourceSecret, err := client.CoreV1().Secrets(sourceNS).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get stateless bucket secret %s/%s", sourceNS, secretName)
	}

	// Extract bucket config from annotations (set by deployer bucket managers)
	if err := extractBucketConfig(sourceSecret.Annotations, h.testContext.Stateless); err != nil {
		return errors.Wrap(err, "failed to extract bucket config from secret annotations")
	}

	log.Info("Stateless bucket configuration initialized",
		"provider", h.testContext.Stateless.Provider,
		"bucket", h.testContext.Stateless.Bucket)

	return nil
}

// copyStatelessBucketSecret copies the bucket credentials Secret to all managed namespaces.
func (h *helper) copyStatelessBucketSecret() error {
	if h.testContext.Stateless == nil {
		return nil
	}

	secretName := h.testContext.Stateless.SecretName
	sourceNS := h.testContext.Stateless.SecretNamespace

	log.Info("Copying stateless bucket secret to managed namespaces",
		"secret", secretName,
		"source_namespace", sourceNS,
		"target_namespaces", h.testContext.Operator.ManagedNamespaces)

	client, err := h.createKubeClient()
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes client")
	}

	sourceSecret, err := client.CoreV1().Secrets(sourceNS).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get stateless bucket secret %s/%s", sourceNS, secretName)
	}

	// Copy to each managed namespace (create or update if exists)
	for _, targetNS := range h.testContext.Operator.ManagedNamespaces {
		targetSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: targetNS,
			},
			Type: sourceSecret.Type,
			Data: sourceSecret.Data,
		}
		_, err := client.CoreV1().Secrets(targetNS).Create(context.Background(), targetSecret, metav1.CreateOptions{})
		if err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create stateless bucket secret in namespace %s", targetNS)
			}
			// Secret already exists, update it
			_, err = client.CoreV1().Secrets(targetNS).Update(context.Background(), targetSecret, metav1.UpdateOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to update stateless bucket secret in namespace %s", targetNS)
			}
			log.Info("Updated existing stateless bucket secret", "namespace", targetNS)
		} else {
			log.Info("Copied stateless bucket secret", "namespace", targetNS)
		}
	}

	return nil
}

// extractBucketConfig populates StatelessConfig fields from Secret annotations.
// The deployer sets these annotations consistently for both Vault and dynamic bucket creation.
func extractBucketConfig(annotations map[string]string, cfg *test.StatelessConfig) error {
	provider := annotations[bucketAnnotationProvider]
	switch provider {
	case "gcs":
		bucket := annotations[bucketAnnotationBucket]
		if bucket == "" {
			return fmt.Errorf("missing %s annotation for GCS provider", bucketAnnotationBucket)
		}
		cfg.Provider = provider
		cfg.Bucket = bucket
	case "s3":
		bucket := annotations[bucketAnnotationBucket]
		if bucket == "" {
			return fmt.Errorf("missing %s annotation for S3 provider", bucketAnnotationBucket)
		}
		cfg.Provider = provider
		cfg.Bucket = bucket
	case "azure":
		storageAccount := annotations[bucketAnnotationStorageAccount]
		container := annotations[bucketAnnotationContainer]
		if storageAccount == "" || container == "" {
			return fmt.Errorf("missing %s or %s annotation for Azure provider",
				bucketAnnotationStorageAccount, bucketAnnotationContainer)
		}
		cfg.Provider = provider
		cfg.Bucket = container
		cfg.StorageAccount = storageAccount
	case "":
		return fmt.Errorf("missing %s annotation on bucket secret", bucketAnnotationProvider)
	default:
		return fmt.Errorf("unknown provider %q in %s annotation", provider, bucketAnnotationProvider)
	}
	return nil
}
