// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

var (
	// linkedFiles6 describe how various secrets are mapped into the pod's filesystem.
	linkedFiles6 = initcontainer.LinkedFilesArray{
		Array: []initcontainer.LinkedFile{
			{
				Source: stringsutil.Concat(volume.DefaultSecretMountPath, "/", user.ElasticUsersFile),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", user.ElasticUsersFile),
			},
			{
				Source: stringsutil.Concat(volume.DefaultSecretMountPath, "/", user.ElasticRolesFile),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", user.ElasticRolesFile),
			},
			{
				Source: stringsutil.Concat(volume.DefaultSecretMountPath, "/", user.ElasticUsersRolesFile),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", user.ElasticUsersRolesFile),
			},
			{
				Source: stringsutil.Concat(settings.ConfigVolumeMountPath, "/", settings.ConfigFileName),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", settings.ConfigFileName),
			},
			{
				Source: stringsutil.Concat(volume.UnicastHostsVolumeMountPath, "/", volume.UnicastHostsFile),
				Target: stringsutil.Concat("/usr/share/elasticsearch/config", "/", volume.UnicastHostsFile),
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
	// we mount the elastic users secret over at /secrets, which needs to match the "linkedFiles" in the init-container
	// creation below.
	// TODO: make this association clearer.
	paramsTmpl.UsersSecretVolume = volume.NewSecretVolume(
		user.ElasticUsersRolesSecretName(es.Name),
		"users",
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
	nodeCertificatesVolume volume.SecretVolume,
) ([]corev1.Container, error) {
	return initcontainer.NewInitContainers(
		elasticsearchImage,
		operatorImage,
		linkedFiles6,
		setVMMaxMapCount,
		nodeCertificatesVolume,
	)
}

// newEnvironmentVars returns the environment vars to be associated to a pod
func newEnvironmentVars(
	p pod.NewPodSpecParams,
	heapSize int,
	nodeCertificatesVolume volume.SecretVolume,
	privateKeySecretVolume volume.SecretVolume,
	reloadCredsUserSecretVolume volume.SecretVolume,
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

		// TODO: the JVM options are hardcoded, but should be configurable
		{Name: settings.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM -Djava.security.properties=%s", heapSize, heapSize, version.SecurityPropsFile)},

		{Name: settings.EnvReadinessProbeProtocol, Value: "https"},
		{Name: settings.EnvProbeUsername, Value: p.ProbeUser.Name},
		{Name: settings.EnvProbePasswordFile, Value: path.Join(volume.ProbeUserSecretMountPath, p.ProbeUser.Name)},
	}

	vars = append(vars, processmanager.NewEnvVars(nodeCertificatesVolume, privateKeySecretVolume)...)
	vars = append(vars, keystore.NewEnvVars(
		keystore.NewEnvVarsParams{
			SourceDir:          secureSettingsSecretVolume.VolumeMount().MountPath,
			ESUsername:         p.ReloadCredsUser.Name,
			ESPasswordFilepath: path.Join(reloadCredsUserSecretVolume.VolumeMount().MountPath, p.ReloadCredsUser.Name),
			ESCaCertPath:       path.Join(nodeCertificatesVolume.VolumeMount().MountPath, certificates.CAFileName),
			ESVersion:          p.Version,
		})...)

	return vars
}
