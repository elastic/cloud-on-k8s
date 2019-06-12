// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package securesettings

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("secure-settings")

// Resources optionally returns a volume and init container to include in Kibana pods,
// in order to create a Keystore from secure settings referenced in the Kibana spec.
func Resources(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	kb v1alpha1.Kibana,
) ([]corev1.Volume, []corev1.Container, string, error) {
	// setup a volume from the user-provided secure settings secret
	secretVolume, version, err := secureSettingsVolume(c, recorder, watches, kb)
	if err != nil {
		return nil, nil, "", err
	}
	if secretVolume == nil {
		// nothing to do
		return nil, nil, "", nil
	}

	// build an init container to create Kibana keystore from the secure settings volume
	initContainer := initContainer(*secretVolume)

	return []corev1.Volume{secretVolume.Volume()}, []corev1.Container{initContainer}, version, nil
}
