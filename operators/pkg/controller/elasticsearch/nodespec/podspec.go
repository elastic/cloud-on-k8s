// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"crypto/sha256"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

// BuildPodTemplateSpec builds a new PodTemplateSpec for an Elasticsearch node.
func BuildPodTemplateSpec(
	es v1alpha1.Elasticsearch,
	nodeSpec v1alpha1.NodeSpec,
	cfg settings.CanonicalConfig,
	keystoreResources *keystore.Resources,
) (corev1.PodTemplateSpec, error) {
	volumes, volumeMounts := buildVolumes(es.Name, nodeSpec, keystoreResources)
	labels, err := buildLabels(es, cfg, nodeSpec, keystoreResources)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	builder := defaults.NewPodTemplateBuilder(nodeSpec.PodTemplate, v1alpha1.ElasticsearchContainerName).
		WithDockerImage(es.Spec.Image, stringsutil.Concat(DefaultImageRepository, ":", es.Spec.Version))

	initContainers, err := initcontainer.NewInitContainers(
		builder.Container.Image,
		es.Spec.SetVMMaxMapCount,
		transportCertificatesVolume(es.Name),
		es.Name,
		keystoreResources,
	)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	builder = builder.
		WithResources(DefaultResources).
		WithTerminationGracePeriod(DefaultTerminationGracePeriodSeconds).
		WithPorts(DefaultContainerPorts).
		WithReadinessProbe(*NewReadinessProbe()).
		WithAffinity(DefaultAffinity(es.Name)).
		WithEnv(EnvVars...).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithLabels(labels).
		WithInitContainers(initContainers...).
		WithInitContainerDefaults()

	return builder.PodTemplate, nil
}

func transportCertificatesVolume(esName string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		name.TransportCertificatesSecret(esName),
		esvolume.TransportCertificatesSecretVolumeName,
		esvolume.TransportCertificatesSecretVolumeMountPath,
	)
}

func buildLabels(
	es v1alpha1.Elasticsearch,
	cfg settings.CanonicalConfig,
	nodeSpec v1alpha1.NodeSpec,
	keystoreResources *keystore.Resources,
) (map[string]string, error) {
	// label with a hash of the config to rotate the pod on config changes
	unpackedCfg, err := cfg.Unpack()
	if err != nil {
		return nil, err
	}
	nodeRoles := unpackedCfg.Node
	cfgHash := hash.HashObject(cfg)

	// label with version
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}

	podLabels, err := label.NewPodLabels(k8s.ExtractNamespacedName(&es), name.StatefulSet(es.Name, nodeSpec.Name), *ver, nodeRoles, cfgHash)
	if err != nil {
		return nil, err
	}

	if keystoreResources != nil {
		// label with a checksum of the secure settings to rotate the pod on secure settings change
		// TODO: use hash.HashObject instead && fix the config checksum label name?
		configChecksum := sha256.New224()
		_, _ = configChecksum.Write([]byte(keystoreResources.Version))
		podLabels[label.ConfigChecksumLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))
	}

	return podLabels, nil
}
