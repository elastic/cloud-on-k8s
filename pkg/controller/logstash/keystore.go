// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
)

const (
	KeystorePassKey = "LOGSTASH_KEYSTORE_PASS" // #nosec G101
)

var (
	keystoreCommand          = "echo 'y' | /usr/share/logstash/bin/logstash-keystore"
	initContainersParameters = keystore.InitContainerParameters{
		KeystoreCreateCommand:         keystoreCommand + " create",
		KeystoreAddCommand:            keystoreCommand + ` add "$key" --stdin < "$filename"`,
		SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
		KeystoreVolumePath:            volume.ConfigMountPath,
		Resources: corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				corev1.ResourceCPU:    resource.MustParse("1000m"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				corev1.ResourceCPU:    resource.MustParse("1000m"),
			},
		},
	}
)

func reconcileKeystore(params Params, configHash hash.Hash) (*keystore.Resources, error) {
	if keystoreResources, err := keystore.ReconcileResources(
		params.Context,
		params,
		&params.Logstash,
		logstashv1alpha1.Namer,
		NewLabels(params.Logstash),
		initContainersParameters,
	); err != nil {
		return nil, err
	} else if keystoreResources != nil {
		_, _ = configHash.Write([]byte(keystoreResources.Version))
		// set keystore password in init container
		if env := getKeystorePass(params.Logstash); env != nil {
			keystoreResources.InitContainer.Env = append(keystoreResources.InitContainer.Env, *env)
		}

		return keystoreResources, nil
	}

	return nil, nil
}

// getKeystorePass return env LOGSTASH_KEYSTORE_PASS from main container if set
func getKeystorePass(logstash logstashv1alpha1.Logstash) *corev1.EnvVar {
	for _, c := range logstash.Spec.PodTemplate.Spec.Containers {
		if c.Name == logstashv1alpha1.LogstashContainerName {
			for _, env := range c.Env {
				if env.Name == KeystorePassKey {
					return &env
				}
			}
		}
	}

	return nil
}
