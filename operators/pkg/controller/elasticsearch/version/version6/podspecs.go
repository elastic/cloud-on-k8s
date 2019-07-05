// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	corev1 "k8s.io/api/core/v1"
)

//
//// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
//func ExpectedPodSpecs(
//	es v1alpha1.Elasticsearch,
//	paramsTmpl pod.NewPodSpecParams,
//	operatorImage string,
//) ([]pod.PodSpecContext, error) {
//	// the contents of the file realm volume needs to be symlinked into place
//	paramsTmpl.UsersSecretVolume = volume.NewSecretVolumeWithMountPath(
//		user.XPackFileRealmSecretName(es.Name),
//		esvolume.XPackFileRealmVolumeName,
//		esvolume.XPackFileRealmVolumeMountPath,
//	)
//
//	return version.NewExpectedPodSpecs(
//		es,
//		paramsTmpl,
//		NewEnvironmentVars,
//		settings.NewMergedESConfig,
//		initcontainer.NewInitContainers,
//		operatorImage,
//	)
//}

// NewEnvironmentVars returns the environment vars to be associated to a pod
func NewEnvironmentVars(
	p pod.NewPodSpecParams,
	httpCertificatesVolume volume.SecretVolume,
	keystoreUserSecretVolume volume.SecretVolume,
	secureSettingsSecretVolume volume.SecretVolume,
) []corev1.EnvVar {
	vars := []corev1.EnvVar{
		// inject pod name and IP as environment variables dynamically,
		// to be referenced in elasticsearch configuration file
		{Name: settings.EnvPodName, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
		}},
		{Name: settings.EnvPodIP, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
		}},
		{Name: settings.EnvReadinessProbeProtocol, Value: "https"},
		{Name: settings.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: settings.EnvProbePasswordFile, Value: path.Join(esvolume.ProbeUserSecretMountPath, p.ProbeUser.Name)},
	}

	vars = append(vars, processmanager.NewEnvVars(httpCertificatesVolume)...)
	vars = append(vars, keystore.NewEnvVars(
		keystore.NewEnvVarsParams{
			SourceDir:          secureSettingsSecretVolume.VolumeMount().MountPath,
			ESUsername:         p.KeystoreUser.Name,
			ESPasswordFilepath: path.Join(keystoreUserSecretVolume.VolumeMount().MountPath, p.KeystoreUser.Name),
			ESCertsPath:        path.Join(httpCertificatesVolume.VolumeMount().MountPath, certificates.CertFileName),
			ESVersion:          p.Elasticsearch.Spec.Version,
		})...)

	return vars
}
