// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"fmt"
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

const (
	CAFileName = "ca.crt"

	ContainerName = "agent"

	ConfigVolumeName = "config"
	ConfigMountPath  = "/etc/agent.yml"
	ConfigFileName   = "agent.yml"

	FleetSetupVolumeName = "fleet-setup-config"
	FleetSetupMountPath  = "/usr/share/elastic-agent/fleet-setup.yml"
	FleetSetupFileName   = "fleet-setup.yml"

	FleetCertVolumeName = "fleet-certs"
	FleetCertMountPath  = "/usr/share/fleet-server/config/http-certs"

	DataVolumeName            = "agent-data"
	DataMountHostPathTemplate = "/var/lib/%s/%s/agent-data"
	DataMountPath             = "/usr/share/data"

	// ConfigChecksumLabel is a label used to store Agent config checksum.
	ConfigChecksumLabel = "agent.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Agent Pod.
	VersionLabelName = "agent.k8s.elastic.co/version"
)

var (
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("350Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("350Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
	}

	// defaultFleetResources defines default resources to use in case fleet mode is enabled.
	// System+Kubernetes integrations takes Elastic Agent to 70%, Fleet Server to 60% memory
	// usage of the below as of 7.14.0.
	defaultFleetResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
	}
)

func buildPodTemplate(params Params, configHash hash.Hash, fleetCerts *certificates.CertificatesSecret) (corev1.PodTemplateSpec, error) {
	defer tracing.Span(&params.Context)()

	spec := &params.Agent.Spec

	builder := defaults.NewPodTemplateBuilder(params.GetPodTemplate(), ContainerName)

	vols := []volume.VolumeLike{
		// volume with agent configuration file
		volume.NewSecretVolume(
			ConfigSecretName(params.Agent.Name),
			ConfigVolumeName,
			ConfigMountPath,
			ConfigFileName,
			0440),
		// volume with agent data path
		createDataVolume(params),
	}

	// all volumes with CAs of direct associations
	vols = append(vols, getVolumesFromAssociations(params.Agent.GetAssociations())...)

	// fleet mode requires some special treatment
	if spec.Mode == agentv1alpha1.AgentFleetMode {
		// enabling fleet requires configuring fleet setup, agent enrollment, fleet server connection information, etc.
		// all this is defined in fleet-setup.yml file in the volume below
		fleetSetupConfigVol := volume.NewSecretVolume(
			ConfigSecretName(params.Agent.Name),
			FleetSetupVolumeName,
			FleetSetupMountPath,
			FleetSetupFileName,
			0440,
		)
		vols = append(vols, fleetSetupConfigVol)

		if spec.EnableFleetServer {
			// ECK creates CA and a certificate for Fleet Server to use. This volume contains those.
			fleetCAVolume := volume.NewSecretVolumeWithMountPath(
				fleetCerts.Name,
				FleetCertVolumeName,
				FleetCertMountPath,
			)
			vols = append(vols, fleetCAVolume)

			// Beats managed by the Elastic Agent does not trust the Elasticsearch CA that Elastic Agent itself
			// is configured to trust. There is currently no way to configure those Beats to trust a particular CA.
			// The intended way to handle this is to allow Fleet to provide Beat output settings, but due to
			// https://github.com/elastic/kibana/issues/102794 this is not supported outside of UI. To workaround this
			// limitation the Agent is going to update Pod-wide CA store before starting Elastic Agent.
			esAssoc := association.GetAssociationOfType(params.Agent.GetAssociations(), commonv1.ElasticsearchAssociationType)
			if esAssoc != nil {
				builder = builder.
					WithCommand([]string{"/usr/bin/env", "bash", "-c", trustCAScript(path.Join(certificatesDir(esAssoc), CAFileName))})
			}
		} else {
			// See the long comment above.
			// We are processing Elastic Agent resource. As the reference chain is:
			// Elastic Agent ---> Fleet Server ---> Elasticsearch
			// we need first to identify the Fleet Server.
			fs, err := getFsAssociation(params)
			if err != nil {
				return corev1.PodTemplateSpec{}, err
			}

			// now we can find the Elasticsearch association and mount its CA Secret
			esfsAssoc := association.GetAssociationOfType(params.Agent.GetAssociations(), commonv1.ElasticsearchAssociationType)
			if esfsAssoc != nil {
				caSecretName := esfsAssoc.AssociationConf().GetCASecretName()
				caVolume := volume.NewSecretVolumeWithMountPath(
					caSecretName,
					fmt.Sprintf("%s-certs", esfsAssoc.AssociationType()),
					certificatesDir(esfsAssoc),
				)
				vols = append(vols, caVolume)

				// because of the reference chain (Elastic Agent ---> Fleet Server ---> Elasticsearch), we are going to get
				// notified when CA of Elasticsearch changes as Fleet Server resource will get updated as well. But what we
				// also need to do is to roll Elastic Agent Pods to pick up the update CA. To do be able to do that, we are
				// adding Fleet Server associations (which includes Elasticsearch) to config hash attached to Elastic Agent
				// Pods.
				if err := commonassociation.WriteAssocsToConfigHash(params.Client, fs.GetAssociations(), configHash); err != nil {
					return corev1.PodTemplateSpec{}, err
				}

				builder = builder.
					WithCommand([]string{"/usr/bin/env", "bash", "-c", trustCAScript(path.Join(certificatesDir(esfsAssoc), CAFileName))})
			}
		}
	}

	volumes := make([]corev1.Volume, 0, len(vols))
	volumeMounts := make([]corev1.VolumeMount, 0, len(vols))

	for _, v := range vols {
		volumes = append(volumes, v.Volume())
		volumeMounts = append(volumeMounts, v.VolumeMount())
	}

	labels := maps.Merge(NewLabels(params.Agent), map[string]string{
		ConfigChecksumLabel: fmt.Sprintf("%x", configHash.Sum(nil)),
		VersionLabelName:    spec.Version})

	builder = builder.
		WithLabels(labels).
		WithDockerImage(spec.Image, container.ImageRepository(container.AgentImage, spec.Version)).
		WithAutomountServiceAccountToken().
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithEnv(
			corev1.EnvVar{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			}},
		)

	if params.Agent.Spec.Mode == agentv1alpha1.AgentStandaloneMode {
		builder = builder.
			WithResources(defaultResources).
			WithArgs("-e", "-c", path.Join(ConfigMountPath, ConfigFileName))
	} else if params.Agent.Spec.Mode == agentv1alpha1.AgentFleetMode {
		builder = builder.
			WithResources(defaultFleetResources).
			// needed to pick up fleet-setup.yml correctly
			WithEnv(corev1.EnvVar{Name: "CONFIG_PATH", Value: "/usr/share/elastic-agent"})
	}

	return builder.PodTemplate, nil
}

func getVolumesFromAssociations(associations []commonv1.Association) []volume.VolumeLike {
	var vols []volume.VolumeLike
	for i, association := range associations {
		if !association.AssociationConf().CAIsConfigured() {
			return nil
		}
		caSecretName := association.AssociationConf().GetCASecretName()
		vols = append(vols, volume.NewSecretVolumeWithMountPath(
			caSecretName,
			fmt.Sprintf("%s-certs-%d", association.AssociationType(), i),
			certificatesDir(association),
		))
	}
	return vols
}

func getFsAssociation(params Params) (commonv1.Associated, error) {
	assoc := association.GetAssociationOfType(params.Agent.GetAssociations(), commonv1.FleetServerAssociationType)
	if assoc == nil {
		return nil, nil
	}

	fsRef := assoc.AssociationRef()
	fs := agentv1alpha1.Agent{}

	if err := association.FetchWithAssociations(
		params.Context,
		params.Client,
		reconcile.Request{NamespacedName: fsRef.NamespacedName()},
		&fs,
	); err != nil {
		return nil, err
	}

	return &fs, nil
}

func trustCAScript(caPath string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -e
cp %s /etc/pki/ca-trust/source/anchors/
update-ca-trust
/usr/bin/tini -- /usr/local/bin/docker-entrypoint -e --path.config /usr/share/elastic-agent
`, caPath)
}

func createDataVolume(params Params) volume.VolumeLike {
	dataMountHostPath := fmt.Sprintf(DataMountHostPathTemplate, params.Agent.Namespace, params.Agent.Name)

	return volume.NewHostVolume(
		DataVolumeName,
		dataMountHostPath,
		DataMountPath,
		false,
		corev1.HostPathDirectoryOrCreate)
}

func certificatesDir(association commonv1.Association) string {
	ref := association.AssociationRef()
	return fmt.Sprintf(
		"/mnt/elastic-internal/%s-association/%s/%s/certs",
		association.AssociationType(),
		ref.Namespace,
		ref.Name,
	)
}
