// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("keystore")

// Resources holds all the resources needed to create a keystore in Kibana or in the APM server.
type Resources struct {
	// volume which contains the keystore data as provided by the user
	KeystoreVolume corev1.Volume
	// init container used to create the keystore
	KeystoreInitContainer corev1.Container
	// version of the secret provided by the user
	KeystoreVersion string
}

// NewResources optionally returns a volume and init container to include in pods,
// in order to create a Keystore from secure settings referenced in the Kibana spec.
func NewResources(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	object runtime.Object,
	userSecretRef *commonv1alpha1.SecretRef,
	dataVolumePath string,
) (*Resources, error) {
	// setup a volume from the user-provided secure settings secret
	secretVolume, version, err := secureSettingsVolume(c, recorder, watches, object, userSecretRef)
	if err != nil {
		return nil, err
	}
	if secretVolume == nil {
		// nothing to do
		return nil, nil
	}

	// build an init container to create Kibana keystore from the secure settings volume
	initContainer, err := initContainer(
		object,
		*secretVolume,
		dataVolumePath,
		"/usr/share/apm-server/apm-server keystore create --force",
		"/usr/share/apm-server/apm-server keystore add",
	)
	if err != nil {
		return nil, err
	}

	return &Resources{
		KeystoreVolume:        secretVolume.Volume(),
		KeystoreInitContainer: initContainer,
		KeystoreVersion:       version,
	}, nil
}
