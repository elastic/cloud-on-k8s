// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
)

// Resources holds all the resources needed to create a keystore in Kibana or in the APM server.
type Resources struct {
	// volume which contains the keystore data as provided by the user
	Volume corev1.Volume
	// init container used to create the keystore
	InitContainer corev1.Container
	// hash of the secret data provided by the user
	Hash string
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
	labels map[string]string,
	initContainerParams InitContainerParameters,
) (*Resources, error) {
	// setup a volume from the user-provided secure settings secret
	secretVolume, hash, err := secureSettingsVolume(ctx, r, hasKeystore, labels, namer)
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
