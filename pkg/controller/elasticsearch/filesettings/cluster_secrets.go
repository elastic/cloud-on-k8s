// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ReconcileClusterSecrets ensures the cluster_secrets field in the file settings Secret is up-to-date.
// If the Secret does not exist yet it is created. On subsequent calls, only the cluster_secrets field
// is updated (other settings are preserved).
// This function is safe to call even when a StackConfigPolicy manages the Secret: the SCP controller
// preserves existing cluster_secrets via newSettingsSecret, so both controllers converge on the same value.
func ReconcileClusterSecrets(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	clusterSecrets *commonv1.Config,
) error {
	secretName := types.NamespacedName{Namespace: es.Namespace, Name: esv1.FileSettingsSecretName(es.Name)}
	meta := metadata.Propagate(&es, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&es))})

	// Retry conflict and already-exists errors to avoid transient lost updates when
	// SCP and ES controllers update/create the same Secret concurrently.
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		// Get the current file settings secret if it exists.
		var currentSecret corev1.Secret
		var currentSecretPtr *corev1.Secret
		if err := c.Get(ctx, secretName, &currentSecret); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		} else {
			currentSecretPtr = &currentSecret
		}

		// Build expected secret with patched cluster_secrets.
		expectedSecret, err := buildExpectedClusterSecrets(ctx, k8s.ExtractNamespacedName(&es), currentSecretPtr, clusterSecrets, meta)
		if err != nil {
			return err
		}

		// Use WithAdditiveMetadata: only merge labels/annotations, never remove
		// SCP-managed metadata (soft-owner refs, secure-settings, etc.).
		return ReconcileSecretWithCurrent(ctx, c, currentSecretPtr, expectedSecret, &es, WithAdditiveMetadata())
	})
}

// buildExpectedClusterSecrets builds the full expected Secret by patching cluster_secrets
// into the current settings state. If current is nil, a new empty settings Secret is created.
func buildExpectedClusterSecrets(
	ctx context.Context,
	es types.NamespacedName,
	current *corev1.Secret,
	clusterSecrets *commonv1.Config,
	meta metadata.Metadata,
) (corev1.Secret, error) {
	// Build a base expected secret with canonical labels/annotations.
	expectedSecret, _, err := NewSettingsSecretWithVersion(ctx, es, true, nil, nil, nil, meta)
	if err != nil {
		return corev1.Secret{}, err
	}

	if current == nil {
		// No existing secret: patch cluster_secrets into the empty settings.
		return patchClusterSecrets(expectedSecret, clusterSecrets)
	}

	// Existing secret: unmarshal, patch cluster_secrets, preserve other fields.
	// We intentionally fail on malformed JSON to avoid overwriting SCP-managed
	// fields such as cluster_settings.
	var settings Settings
	if err := json.Unmarshal(current.Data[SettingsSecretKey], &settings); err != nil {
		return corev1.Secret{}, fmt.Errorf("failed to unmarshal file settings from secret %s: %w", es, err)
	}

	settings.State.ClusterSecrets = clusterSecrets

	// Bump the version only if the hash changed.
	newHash := settings.hash()
	if current.Annotations[commonannotation.SettingsHashAnnotationName] != newHash {
		settings.Metadata.Version = strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	settingsBytes, err := json.Marshal(settings)
	if err != nil {
		return corev1.Secret{}, err
	}

	expectedSecret.Data[SettingsSecretKey] = settingsBytes
	expectedSecret.Annotations[commonannotation.SettingsHashAnnotationName] = newHash

	return expectedSecret, nil
}

// patchClusterSecrets sets cluster_secrets in a Secret's settings.json data.
func patchClusterSecrets(secret corev1.Secret, clusterSecrets *commonv1.Config) (corev1.Secret, error) {
	var settings Settings
	if err := json.Unmarshal(secret.Data[SettingsSecretKey], &settings); err != nil {
		return corev1.Secret{}, err
	}
	settings.State.ClusterSecrets = clusterSecrets
	settingsBytes, err := json.Marshal(settings)
	if err != nil {
		return corev1.Secret{}, err
	}
	secret.Data[SettingsSecretKey] = settingsBytes
	secret.Annotations[commonannotation.SettingsHashAnnotationName] = settings.hash()
	return secret, nil
}
