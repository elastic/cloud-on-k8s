// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
)

const (
	InitConfigContainerName = "logstash-internal-init-config"
	UseTLSEnv               = "USE_TLS"

	// InitConfigScript is a small bash script to prepare the logstash configuration directory
	// Logstash rewrites the configuration file (logstash.yml) in the presence of ${VAR} environment variable,
	// therefore the file is copied to the path instead of creating symbolic link.
	InitConfigScript = `#!/usr/bin/env bash
set -eu

init_config_initialized_flag=` + volume.InitContainerConfigVolumeMountPath + `/elastic-internal-init-config.ok

mount_path=` + volume.InitContainerConfigVolumeMountPath + `
http_cert_path=` + certificates.HTTPCertificatesSecretVolumeMountPath + `

if [[ "$USE_TLS" == "true" ]] && [[ -d "$http_cert_path" ]] && [[ "$(ls -A $http_cert_path)" ]]; then
    echo "Setup Logstash keystore for API server"
	if [[ -e $http_cert_path/` + certificates.CAFileName + ` ]]; then
		ln -sf $http_cert_path/` + certificates.CAFileName + ` $mount_path
	fi
	ln -sf $http_cert_path/` + certificates.CertFileName + ` $mount_path
	ln -sf $http_cert_path/` + certificates.KeyFileName + ` $mount_path
	openssl pkcs12 -export -in $mount_path/` + certificates.CertFileName + ` -inkey $mount_path/` + certificates.KeyFileName + ` -out $mount_path/` + APIKeystoreFileName + ` -name "logstash_tls" -passout "pass:$API_KEYSTORE_PASS"
	echo "Logstash API server keystore successfully prepared."
fi

if [[ -f "${init_config_initialized_flag}" ]]; then
    echo "Logstash configuration already initialized."
    exit 0
fi

echo "Setup Logstash configuration"

cp -f /usr/share/logstash/config/*.* "$mount_path"

cp ` + volume.InternalConfigVolumeMountPath + `/` + ConfigFileName + ` $mount_path
ln -sf ` + volume.InternalPipelineVolumeMountPath + `/` + PipelineFileName + ` $mount_path

touch "${init_config_initialized_flag}"
echo "Logstash configuration successfully prepared."
`
)

// initConfigContainer returns an init container that executes a bash script to
// (1) prepare the logstash config directory.
// This copies files from the `config` folder of the docker image, and creates symlinks for the `logstash.yml` and
// `pipelines.yml` files created by the operator into a shared config folder to be used by the main logstash container.
// This enables dynamic reloads for `pipelines.yml`.
// (2) prepare keystore for API server
// This copies tls.crt, tls.key, ca.crt from Secret http-certs-internal, and creates symlinks
// for openssl to create keystore. Logstash API server (puma jruby) only supports p12 and java keystore
// This enables API server supports https
func initConfigContainer(params Params) corev1.Container {
	ls := params.Logstash
	privileged := false

	container := corev1.Container{
		// Image will be inherited from pod template defaults
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            InitConfigContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command: []string{"/usr/bin/env", "bash", "-c", InitConfigScript},
		Env: []corev1.EnvVar{
			{
				Name:  UseTLSEnv,
				Value: params.APIServerConfig.SSLEnabled,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			volume.ConfigSharedVolume.InitContainerVolumeMount(),
			volume.ConfigVolume(ls).VolumeMount(),
			volume.PipelineVolume(ls).VolumeMount(),
		},
		Resources: corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("50Mi"),
				corev1.ResourceCPU:    resource.MustParse("0.1"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				// Memory limit should be at least 12582912 when running with CRI-O
				corev1.ResourceMemory: resource.MustParse("50Mi"),
				corev1.ResourceCPU:    resource.MustParse("0.1"),
			},
		},
	}

	if params.APIServerConfig.UseTLS() {
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name: APIKeystorePassEnv,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: logstashv1alpha1.ConfigSecretName(params.Logstash.Name),
						},
						Key: APIKeystorePassEnv,
					},
				},
			})
	}

	return container
}
