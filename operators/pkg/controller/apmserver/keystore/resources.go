// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("keystore")

// KeystoreResources holds all the resources needed to create a keystore in Kibana or in the APM server.
type KeystoreResources struct {
	// volume which contains the keystore data as provided by the user
	KeystoreVolume corev1.Volume
	// init container used to create the keystore
	KeystoreInitContainer corev1.Container
	// version of the secret provided by the user
	KeystoreVersion string
}

// Resources optionally returns a volume and init container to include in pods,
// in order to create a Keystore from secure settings referenced in the Kibana spec.
func Resources(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	as v1alpha1.ApmServer,
) (*KeystoreResources, error) {
	// setup a volume from the user-provided secure settings secret
	secretVolume, version, err := secureSettingsVolume(c, recorder, watches, as)
	if err != nil {
		return nil, err
	}
	if secretVolume == nil {
		// nothing to do
		return nil, nil
	}

	// build an init container to create Kibana keystore from the secure settings volume
	initContainer := initContainer(*secretVolume)

	return &KeystoreResources{
		KeystoreVolume:        secretVolume.Volume(),
		KeystoreInitContainer: initContainer,
		KeystoreVersion:       version,
	}, nil
}
