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
	// Retry conflict and already-exists errors to avoid transient lost updates when
	// SCP and ES controllers update/create the same Secret concurrently.
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		// Get the file settings secret. Create it if it doesn't exist.
		// If the SCP controller creates it concurrently, ReconcileSecret (used by createFileSettingsSecret)
		// returns an AlreadyExists error which triggers a requeue. The next reconciliation loop will find
		// the secret in the cache and take the update path.
		var currentSecret corev1.Secret
		if err := c.Get(ctx, secretName, &currentSecret); err != nil {
			if apierrors.IsNotFound(err) {
				return createFileSettingsSecret(ctx, c, es, clusterSecrets)
			}
			return err
		}

		// Unmarshal current settings.
		// We intentionally fail here (instead of best-effort ignore) to avoid
		// reconstructing settings.json and potentially overwriting SCP-managed
		// fields such as cluster_settings.
		var settings Settings
		if err := json.Unmarshal(currentSecret.Data[SettingsSecretKey], &settings); err != nil {
			return fmt.Errorf("failed to unmarshal file settings from secret %s: %w", secretName, err)
		}

		// Update cluster_secrets
		settings.State.ClusterSecrets = clusterSecrets

		// Check if the hash has changed; skip update if nothing changed.
		newHash := settings.hash()
		if currentSecret.Annotations[commonannotation.SettingsHashAnnotationName] == newHash {
			return nil
		}

		// Bump the version so Elasticsearch picks up the change.
		settings.Metadata.Version = strconv.FormatInt(time.Now().UnixNano(), 10)

		settingsBytes, err := json.Marshal(settings)
		if err != nil {
			return err
		}

		// Update secret data and hash annotation in-place.
		currentSecret.Data[SettingsSecretKey] = settingsBytes
		if currentSecret.Annotations == nil {
			currentSecret.Annotations = make(map[string]string)
		}
		currentSecret.Annotations[commonannotation.SettingsHashAnnotationName] = newHash

		return c.Update(ctx, &currentSecret)
	})
}

// createFileSettingsSecret creates a new file settings Secret with the given cluster_secrets.
func createFileSettingsSecret(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	clusterSecrets *commonv1.Config,
) error {
	meta := metadata.Propagate(&es, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&es))})
	expectedSecret, _, err := NewSettingsSecretWithVersion(ctx, k8s.ExtractNamespacedName(&es), true, nil, nil, nil, meta)
	if err != nil {
		return err
	}

	// Set cluster_secrets in the newly created settings.
	var settings Settings
	if err := json.Unmarshal(expectedSecret.Data[SettingsSecretKey], &settings); err != nil {
		return err
	}
	settings.State.ClusterSecrets = clusterSecrets
	settingsBytes, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	expectedSecret.Data[SettingsSecretKey] = settingsBytes
	expectedSecret.Annotations[commonannotation.SettingsHashAnnotationName] = settings.hash()

	return ReconcileSecret(ctx, c, expectedSecret, &es)
}
