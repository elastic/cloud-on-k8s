// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	commonkeystore "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	esvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// KeystoreFileName is the name of the keystore file inside the Secret.
	// This must match esvolume.KeystoreFileName.
	KeystoreFileName = "elasticsearch.keystore"

	// SettingsHashAnnotation stores the hash of the input settings used to generate the keystore.
	// This is used to detect when the keystore needs to be regenerated.
	SettingsHashAnnotation = "keystore.k8s.elastic.co/settings-hash"
)

// MinESVersion is the minimum Elasticsearch version that supports the reloadable keystore feature
// with digest verification. This feature requires the enhanced _nodes/reload_secure_settings API
// that returns keystore digests, which is available in Elasticsearch 9.3+.
var MinESVersion = version.MinFor(9, 3, 0)

// Reconcile creates or updates the keystore Secret for an Elasticsearch cluster.
// It uses the common keystore package to collect secure settings, then creates
// a pre-built keystore file using the Go implementation.
//
// To avoid unnecessary keystore regeneration, the function computes a hash of
// the input settings and stores it as an annotation. The keystore is only
// regenerated when the settings hash changes.
func Reconcile(
	ctx context.Context,
	d driver.Interface,
	es *esv1.Elasticsearch,
	meta metadata.Metadata,
	additionalSources ...commonv1.NamespacedSecretSource,
) (*commonkeystore.Resources, error) {
	log := ulog.FromContext(ctx)

	// Use the common keystore package to collect and aggregate secure settings
	settings, err := commonkeystore.CollectSecureSettings(ctx, d, es, additionalSources...)
	if err != nil {
		return nil, fmt.Errorf("failed to collect secure settings: %w", err)
	}

	// Even with no user-provided settings, we create a keystore containing just the bootstrap seed.
	// This ensures the keystore infrastructure is in place, so adding secure settings later
	// only requires a hot-reload rather than a pod restart.
	// The settings map may be empty here - EnsureBootstrapSeed will add the seed.
	if settings == nil {
		settings = make(Settings)
	}

	// Clean up the init-container-based secure-settings Secret if it exists
	// (in case user switched from init container approach to Go keystore)
	secureSettingsSecretName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.SecureSettingsSecret(es.Name),
	}
	if err := k8s.DeleteSecretIfExists(ctx, d.K8sClient(), secureSettingsSecretName); err != nil {
		return nil, fmt.Errorf("failed to delete legacy secure-settings secret: %w", err)
	}

	// Compute hash of the input settings to detect changes
	settingsHash := computeSettingsHash(settings)

	// Check if the existing keystore Secret has the same settings hash
	secretName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.KeystoreSecretName(es.Name),
	}
	var existingSecret corev1.Secret
	err = d.K8sClient().Get(ctx, secretName, &existingSecret)
	if err == nil {
		// Secret exists - check if settings have changed
		existingHash := existingSecret.Annotations[SettingsHashAnnotation]
		if existingHash == settingsHash {
			log.V(1).Info("Keystore settings unchanged, skipping regeneration",
				"settings_hash", settingsHash,
			)
			return buildResourcesFromSecret(&existingSecret), nil
		}
		log.V(1).Info("Keystore settings changed, regenerating",
			"old_hash", existingHash,
			"new_hash", settingsHash,
		)
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get existing keystore secret: %w", err)
	}

	log.V(1).Info("Creating keystore", "settings_count", len(settings))

	// Create the keystore file
	keystoreData, err := Create(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to create keystore: %w", err)
	}

	// Compute the SHA-256 digest of the keystore file
	// This is used for convergence checking with the ES reload_secure_settings API (9.3+)
	digest := computeKeystoreDigest(keystoreData)

	// Build annotations for the keystore Secret
	annotations := make(map[string]string, len(meta.Annotations)+2)
	maps.Copy(annotations, meta.Annotations)
	annotations[esv1.KeystoreDigestAnnotation] = digest
	annotations[SettingsHashAnnotation] = settingsHash

	// Create or update the keystore Secret
	keystoreSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        esv1.KeystoreSecretName(es.Name),
			Namespace:   es.Namespace,
			Labels:      meta.Labels,
			Annotations: annotations,
		},
		Data: map[string][]byte{
			KeystoreFileName: keystoreData,
		},
	}

	reconciledSecret, err := reconciler.ReconcileSecret(ctx, d.K8sClient(), keystoreSecret, es)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile keystore secret: %w", err)
	}

	return buildResourcesFromSecret(&reconciledSecret), nil
}

// buildResourcesFromSecret creates the Resources struct from a Secret.
// The keystore is mounted to a separate location (not the config directory) so that
// the Secret mount can auto-update when the Secret changes. The init container
// creates a symlink from the mount location into the config directory.
func buildResourcesFromSecret(secret *corev1.Secret) *commonkeystore.Resources {
	// Mount the Secret to the keystore volume mount path (NOT config directory).
	// This allows the Secret mount to auto-update without needing pod restarts.
	// The init container will create a symlink from this location to the config directory.
	keystoreVolume := volume.NewSecretVolumeWithMountPath(
		secret.Name,
		esvolume.KeystoreVolumeName,
		esvolume.KeystoreVolumeMountPath,
	)

	return &commonkeystore.Resources{
		Volume: keystoreVolume.Volume(),
		// Empty init container - the keystore is pre-built and linked by prepare-fs
		InitContainer: corev1.Container{},
		// VolumeMount is the standard mount (no subPath) so updates propagate automatically
		VolumeMount: keystoreVolume.VolumeMount(),
		// Empty hash - we don't want keystore changes to trigger pod restarts.
		// Instead, we rely on the ES reload_secure_settings API for hot reloading.
		Hash: "",
	}
}

// computeKeystoreDigest computes the SHA-256 digest of the given keystore data.
// This is used for convergence checking with the ES reload_secure_settings API.
func computeKeystoreDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// computeSettingsHash computes a deterministic hash of the input settings.
// This is used to detect when the keystore needs to be regenerated.
// The hash is computed over sorted key-value pairs to ensure determinism.
func computeSettingsHash(settings Settings) string {
	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Hash all key-value pairs
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0}) // separator
		h.Write(settings[k])
		h.Write([]byte{0}) // separator
	}

	return hex.EncodeToString(h.Sum(nil))
}

// DeleteSecretIfExists deletes the keystore Secret if it exists.
// This is used to clean up the Go keystore Secret when switching to the init container approach.
func DeleteSecretIfExists(ctx context.Context, c k8s.Client, es *esv1.Elasticsearch) error {
	secretName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.KeystoreSecretName(es.Name),
	}
	return k8s.DeleteSecretIfExists(ctx, c, secretName)
}

// ShouldUseGoKeystore returns true if the Go-based reloadable keystore feature should be used
// for the given Elasticsearch cluster.
//
// Requirements for the feature to be enabled:
// - Elasticsearch version 9.3.0 or later (for keystore digest in reload API response)
// - The feature is not explicitly disabled via annotation
//
// Note: This returns true even when there are no user-provided secure settings.
// In that case, a keystore with just the bootstrap seed is created, ensuring
// the keystore infrastructure is in place for hot-reloading when settings are added later.
func ShouldUseGoKeystore(es esv1.Elasticsearch, esVersion version.Version) bool {
	// Check if the feature is disabled via annotation
	if es.IsReloadableKeystoreDisabled() {
		return false
	}

	// Check version requirement (9.3+ for keystore digest in reload API)
	return esVersion.GTE(MinESVersion)
}
