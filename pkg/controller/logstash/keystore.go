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
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
)

const (
	KeystorePassKey = "LOGSTASH_KEYSTORE_PASS" // #nosec G101
)

var (
	// containerCommand runs in every pod creation to regenerate keystore.
	// `logstash-keystore` allows for adding multiple keys in a single operation.
	// All keys and values must be ASCII and non-empty string. Values are input via stdin, delimited by \n.
	containerCommand = `#!/usr/bin/env bash

set -eu

{{ if not .SkipInitializedFlag -}}
keystore_initialized_flag={{ .KeystoreVolumePath }}/elastic-internal-init-keystore.ok

if [[ -f "${keystore_initialized_flag}" ]]; then
    echo "Keystore already initialized."
	exit 0
fi

{{ end -}}
echo "Initializing keystore."

# create a keystore in the default data path
{{ .KeystoreCreateCommand }}

# add all existing secret entries to keys (Array), vals (String). 
for filename in  {{ .SecureSettingsVolumeMountPath }}/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	keys+=("$key")
	vals+=$(cat "$filename")
	vals+="\n"
done

# remove the trailing '\n' from the end of the vals
vals=${vals%'\n'}

# add multiple keys to keystore
{{ .KeystoreAddCommand }}

{{ if not .SkipInitializedFlag -}}
touch {{ .KeystoreVolumePath }}/elastic-internal-init-keystore.ok
{{ end -}}

echo "Keystore initialization successful."
`

	initContainersParameters = keystore.InitContainerParameters{
		KeystoreCreateCommand:         "echo 'y' | /usr/share/logstash/bin/logstash-keystore create",
		KeystoreAddCommand:            `echo -e "$vals" | /usr/share/logstash/bin/logstash-keystore add "${keys[@]}"`,
		CustomScript:                  containerCommand,
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
		labels.NewLabels(params.Logstash),
		initContainersParameters,
	); err != nil {
		return nil, err
	} else if keystoreResources != nil {
		_, _ = configHash.Write([]byte(keystoreResources.Hash))
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
	if c := pod.ContainerByName(logstash.Spec.PodTemplate.Spec, logstashv1alpha1.LogstashContainerName); c != nil {
		for _, env := range c.Env {
			if env.Name == KeystorePassKey {
				return &env
			}
		}
	}
	return nil
}
