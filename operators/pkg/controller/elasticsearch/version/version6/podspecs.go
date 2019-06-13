// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"path"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

var (
	// linkedFiles6 describe how various secrets are mapped into the pod's filesystem.
	linkedFiles6 = initcontainer.LinkedFilesArray{
		Array: []initcontainer.LinkedFile{
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.ElasticUsersFile),
				Target: stringsutil.Concat(initcontainer.EsConfigSharedVolume.EsContainerMountPath, "/", user.ElasticUsersFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.ElasticRolesFile),
				Target: stringsutil.Concat(initcontainer.EsConfigSharedVolume.EsContainerMountPath, "/", user.ElasticRolesFile),
			},
			{
				Source: stringsutil.Concat(esvolume.XPackFileRealmVolumeMountPath, "/", user.ElasticUsersRolesFile),
				Target: stringsutil.Concat(initcontainer.EsConfigSharedVolume.EsContainerMountPath, "/", user.ElasticUsersRolesFile),
			},
			{
				Source: stringsutil.Concat(settings.ConfigVolumeMountPath, "/", settings.ConfigFileName),
				Target: stringsutil.Concat(initcontainer.EsConfigSharedVolume.EsContainerMountPath, "/", settings.ConfigFileName),
			},
			{
				Source: stringsutil.Concat(esvolume.UnicastHostsVolumeMountPath, "/", esvolume.UnicastHostsFile),
				Target: stringsutil.Concat(initcontainer.EsConfigSharedVolume.EsContainerMountPath, "/", esvolume.UnicastHostsFile),
			},
		},
	}
)

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func ExpectedPodSpecs(
	es v1alpha1.Elasticsearch,
	paramsTmpl pod.NewPodSpecParams,
	operatorImage string,
) ([]pod.PodSpecContext, error) {
	// the contents of the file realm volume needs to be symlinked into place
	paramsTmpl.UsersSecretVolume = volume.NewSecretVolumeWithMountPath(
		user.XPackFileRealmSecretName(es.Name),
		esvolume.XPackFileRealmVolumeName,
		esvolume.XPackFileRealmVolumeMountPath,
	)

	return version.NewExpectedPodSpecs(
		es,
		paramsTmpl,
		newEnvironmentVars,
		settings.NewMergedESConfig,
		newInitContainers,
		operatorImage,
	)
}

// newInitContainers returns a list of init containers
func newInitContainers(
	elasticsearchImage string,
	operatorImage string,
	setVMMaxMapCount *bool,
	transportCertificatesVolume volume.SecretVolume,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(
		elasticsearchImage,
		operatorImage,
		linkedFiles6,
		setVMMaxMapCount,
		transportCertificatesVolume,
	)
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func newEnvironmentVars(
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
			ESVersion:          p.Version,
		})...)

	return vars
}
