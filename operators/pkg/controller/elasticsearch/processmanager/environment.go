// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	EnvProcName        = "PM_PROC_NAME"
	EnvProcCmd         = "PM_PROC_CMD"
	EnvReaper          = "PM_REAPER"
	EnvHTTPPort        = "PM_HTTP_PORT"
	EnvTLS             = "PM_TLS"
	EnvCertPath        = "PM_CERT_PATH"
	EnvKeyPath         = "PM_KEY_PATH"
	EnvKeystoreUpdater = "PM_KEYSTORE_UPDATER"
	EnvExpVars         = "PM_EXP_VARS"
	EnvProfiler        = "PM_PROFILER"

	CommandPath          = volume.ProcessManagerEmptyDirMountPath + "/process-manager"
	ElasticsearchCommand = "/usr/local/bin/docker-entrypoint.sh"
)

func NewEnvVars(httpCertsSecretVolume volume.VolumeLike) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: EnvProcName, Value: "es"},
		{Name: EnvProcCmd, Value: ElasticsearchCommand},
		{Name: EnvTLS, Value: "true"},
		{Name: EnvCertPath, Value: path.Join(httpCertsSecretVolume.VolumeMount().MountPath, certificates.CertFileName)},
		{Name: EnvKeyPath, Value: path.Join(httpCertsSecretVolume.VolumeMount().MountPath, certificates.KeyFileName)},
	}
}
