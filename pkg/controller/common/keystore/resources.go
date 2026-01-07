// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
)

// Resources holds all the resources needed to create a keystore in Kibana, APM server, or Elasticsearch.
type Resources struct {
	// Volume contains the keystore data as provided by the user (for init container approach)
	// or the pre-built keystore file (for direct mount approach).
	Volume corev1.Volume
	// InitContainer is used to create the keystore from secure settings.
	// If empty (Name == ""), no init container is needed (pre-built keystore approach).
	InitContainer corev1.Container
	// VolumeMount is the mount for the keystore in the main container.
	// Used for pre-built keystores that are mounted directly without an init container.
	VolumeMount corev1.VolumeMount
	// Hash of the secret data provided by the user.
	// Used to detect changes and trigger pod restarts (for init container approach)
	// or left empty for hot-reload approach.
	Hash string
}

// HasInitContainer returns true if an init container is needed to create the keystore.
// This is the case when InitContainer has been configured (has a non-empty name).
func (r *Resources) HasInitContainer() bool {
	return r.InitContainer.Name != ""
}

// HasVolume returns true if a volume is configured for the keystore.
func (r *Resources) HasVolume() bool {
	return r.Volume.Name != ""
}

// HasVolumeMount returns true if a volume mount is configured for the keystore.
// This is used for pre-built keystores that are mounted directly without an init container.
func (r *Resources) HasVolumeMount() bool {
	return r.VolumeMount.Name != ""
}

// HasKeystore interface represents an Elastic Stack application that offers a keystore which in ECK
// is populated using a user-provided secret containing secure settings.
type HasKeystore interface {
	metav1.Object
	runtime.Object
	SecureSettings() []commonv1.SecretSource
}

// WatchedSecretNames returns the name of all secure settings secrets to watch.
func WatchedSecretNames(hasKeystore HasKeystore) []commonv1.NamespacedSecretSource {
	nsns := make([]commonv1.NamespacedSecretSource, 0, len(hasKeystore.SecureSettings()))
	for _, s := range hasKeystore.SecureSettings() {
		nsns = append(nsns, commonv1.NamespacedSecretSource{
			Namespace:  hasKeystore.GetNamespace(),
			SecretName: s.SecretName,
			Entries:    s.Entries,
		})
	}
	return nsns
}

// ReconcileResources optionally returns a volume and init container to include in Pods,
// in order to create a Keystore from a Secret containing secure settings provided by
// the user and referenced in the Elastic Stack application spec.
// It reconciles the backing secret with the API server and sets up the necessary watches.
func ReconcileResources(
	ctx context.Context,
	r driver.Interface,
	hasKeystore HasKeystore,
	namer name.Namer,
	meta metadata.Metadata,
	initContainerParams InitContainerParameters,
	additionalSources ...commonv1.NamespacedSecretSource,
) (*Resources, error) {
	// setup a volume from the user-provided secure settings secret
	secretVolume, hash, err := secureSettingsVolume(ctx, r, hasKeystore, meta, namer, additionalSources...)
	if err != nil {
		return nil, err
	}
	if secretVolume == nil {
		// nothing to do
		return nil, nil
	}

	// build an init container to create the keystore from the secure settings volume
	initContainer, err := initContainer(*secretVolume, initContainerParams)
	if err != nil {
		return nil, err
	}

	return &Resources{
		Volume:        secretVolume.Volume(),
		InitContainer: initContainer,
		Hash:          hash,
	}, nil
}
