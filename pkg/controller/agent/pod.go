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
	ConfigMountPath  = "/etc"
	ConfigFileName   = "agent.yml"

	FleetSetupVolumeName = "fleet-setup-config"
	FleetSetupMountPath  = "/usr/share/elastic-agent"
	FleetSetupFileName   = "fleet-setup.yml"

	FleetCertsVolumeName = "fleet-certs"
	FleetCertsMountPath  = "/usr/share/fleet-server/config/http-certs"

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

func buildPodTemplate(params Params, fleetCerts *certificates.CertificatesSecret, configHash hash.Hash) (corev1.PodTemplateSpec, error) {
	defer tracing.Span(&params.Context)()
	spec := &params.Agent.Spec
	builder := defaults.NewPodTemplateBuilder(params.GetPodTemplate(), ContainerName)

	// fleet mode requires some special treatment
	if spec.FleetModeEnabled() {
		var err error
		if builder, err = amendBuilderForFleetMode(params, fleetCerts, builder, configHash); err != nil {
			return corev1.PodTemplateSpec{}, err
		}
	} else if params.Agent.Spec.StandaloneModeEnabled() {
		builder = builder.
			WithResources(defaultResources).
			WithArgs("-e", "-c", path.Join(ConfigMountPath, ConfigFileName))
	}

	vols := []volume.VolumeLike{
		// volume with agent configuration file
		volume.NewSecretVolume(
			ConfigSecretName(params.Agent.Name),
			ConfigVolumeName,
			path.Join(ConfigMountPath, ConfigFileName),
			ConfigFileName,
			0440),
		// volume with agent data path
		createDataVolume(params),
	}

	// all volumes with CAs of direct associations
	vols = append(vols, getVolumesFromAssociations(params.Agent.GetAssociations())...)

	labels := maps.Merge(NewLabels(params.Agent), map[string]string{
		ConfigChecksumLabel: fmt.Sprintf("%x", configHash.Sum(nil)),
		VersionLabelName:    spec.Version})

	builder = builder.
		WithLabels(labels).
		WithDockerImage(spec.Image, container.ImageRepository(container.AgentImage, spec.Version)).
		WithAutomountServiceAccountToken().
		WithVolumeLikes(vols...).
		WithEnv(
			corev1.EnvVar{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			}},
		)

	return builder.PodTemplate, nil
}

func amendBuilderForFleetMode(params Params, fleetCerts *certificates.CertificatesSecret, builder *defaults.PodTemplateBuilder, configHash hash.Hash) (*defaults.PodTemplateBuilder, error) {
	// enabling fleet requires configuring fleet setup, agent enrollment, fleet server connection information, etc.
	// all this is defined in fleet-setup.yml file in the volume below
	vols := []volume.VolumeLike{volume.NewSecretVolume(
		ConfigSecretName(params.Agent.Name),
		FleetSetupVolumeName,
		path.Join(FleetSetupMountPath, FleetSetupFileName),
		FleetSetupFileName,
		0440,
	)}

	var esAssociation commonv1.Association
	//nolint:nestif
	if params.Agent.Spec.FleetServerEnabled {
		// ECK creates CA and a certificate for Fleet Server to use. This volume contains those.
		vols = append(vols, volume.NewSecretVolumeWithMountPath(
			fleetCerts.Name,
			FleetCertsVolumeName,
			FleetCertsMountPath,
		))

		builder = builder.WithPorts([]corev1.ContainerPort{{Name: params.Agent.Spec.HTTP.Protocol(), ContainerPort: FleetServerPort, Protocol: corev1.ProtocolTCP}})

		// Beats managed by the Elastic Agent don't trust the Elasticsearch CA that Elastic Agent itself is configured
		// to trust. There is currently no way to configure those Beats to trust a particular CA. The intended way to handle
		// it is to allow Fleet to provide Beat output settings, but due to https://github.com/elastic/kibana/issues/102794
		// this is not supported outside of UI. To workaround this limitation the Agent is going to update Pod-wide CA store
		// before starting Elastic Agent.
		esAssociation = association.GetAssociationOfType(params.Agent.GetAssociations(), commonv1.ElasticsearchAssociationType)
	} else {
		// See the long comment above. As the reference chain is: Elastic Agent ---> Fleet Server ---> Elasticsearch,
		// we need first to identify the Fleet Server and then identify its reference to Elasticsearch.
		fs, err := getAssociatedFleetServer(params)
		if err != nil {
			return nil, err
		}

		if fs != nil {
			esAssociation = association.GetAssociationOfType(fs.GetAssociations(), commonv1.ElasticsearchAssociationType)
			if esAssociation != nil {
				if params.Agent.Namespace != esAssociation.Associated().GetNamespace() {
					return nil, fmt.Errorf(
						"agent namespace %s is different than referenced Elasticsearch namespace %s, this is not supported yet",
						params.Agent.Namespace,
						esAssociation.Associated().GetNamespace(),
					)
				}

				caSecretName := esAssociation.AssociationConf().GetCASecretName()
				vols = append(vols, volume.NewSecretVolumeWithMountPath(
					caSecretName,
					fmt.Sprintf("%s-certs", esAssociation.AssociationType()),
					certificatesDir(esAssociation),
				))

				// Because of the reference chain (Elastic Agent ---> Fleet Server ---> Elasticsearch), we are going to get
				// notified when CA of Elasticsearch changes as Fleet Server resource will get updated as well. But what we
				// also need to do is to roll Elastic Agent Pods to pick up the update CA. To do be able to do that, we are
				// adding Fleet Server associations (which includes Elasticsearch) to config hash attached to Elastic Agent
				// Pods.
				if err := commonassociation.WriteAssocsToConfigHash(params.Client, fs.GetAssociations(), configHash); err != nil {
					return nil, err
				}
			}
		}
	}

	if esAssociation != nil {
		builder = builder.
			WithCommand([]string{"/usr/bin/env", "bash", "-c", trustCAScript(path.Join(certificatesDir(esAssociation), CAFileName))})
	}

	builder = builder.
		WithVolumeLikes(vols...).
		WithResources(defaultFleetResources).
		// needed to pick up fleet-setup.yml correctly
		WithEnv(corev1.EnvVar{Name: "CONFIG_PATH", Value: "/usr/share/elastic-agent"})

	return builder, nil
}

func getVolumesFromAssociations(associations []commonv1.Association) []volume.VolumeLike {
	vols := []volume.VolumeLike{}
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

func getAssociatedFleetServer(params Params) (commonv1.Associated, error) {
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
/usr/bin/tini -- /usr/local/bin/docker-entrypoint -e
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
