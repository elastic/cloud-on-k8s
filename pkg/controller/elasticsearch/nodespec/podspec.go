// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/stackmon"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

const (
	defaultFsGroup                    = 1000
	log4j2FormatMsgNoLookupsParamName = "-Dlog4j2.formatMsgNoLookups"
)

// Starting 8.0.0, the Elasticsearch container does not run with the root user anymore. As a result,
// we cannot chown the mounted volumes to the right user (id 1000) in an init container.
// Instead, we can rely on Kubernetes `securityContext.fsGroup` feature: by setting it to 1000
// mounted volumes can correctly be accessed by the default container user.
// On some restricted environments (custom PSPs or Openshift), setting the Pod security context
// is forbidden: the user can either set `--set-default-security-context=false`, or override the
// podTemplate securityContext to an empty value.
var minDefaultSecurityContextVersion = version.MustParse("8.0.0")

// BuildPodTemplateSpec builds a new PodTemplateSpec for an Elasticsearch node.
func BuildPodTemplateSpec(
	client k8s.Client,
	es esv1.Elasticsearch,
	nodeSet esv1.NodeSet,
	cfg settings.CanonicalConfig,
	keystoreResources *keystore.Resources,
	setDefaultSecurityContext bool,
) (corev1.PodTemplateSpec, error) {
	downwardAPIVolume := volume.DownwardAPI{}.WithAnnotations(es.HasDownwardNodeLabels())
	volumes, volumeMounts := buildVolumes(es.Name, nodeSet, keystoreResources, downwardAPIVolume)

	labels, err := buildLabels(es, cfg, nodeSet, keystoreResources)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	defaultContainerPorts := getDefaultContainerPorts(es)

	// now build the initContainers using the effective main container resources as an input
	initContainers, err := initcontainer.NewInitContainers(
		transportCertificatesVolume(esv1.StatefulSet(es.Name, nodeSet.Name)),
		keystoreResources,
		es.DownwardNodeLabels(),
	)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	builder := defaults.NewPodTemplateBuilder(nodeSet.PodTemplate, esv1.ElasticsearchContainerName)

	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	if ver.GTE(minDefaultSecurityContextVersion) && setDefaultSecurityContext {
		builder = builder.WithPodSecurityContext(corev1.PodSecurityContext{
			FSGroup: pointer.Int64(defaultFsGroup),
		})
	}

	headlessServiceName := HeadlessServiceName(esv1.StatefulSet(es.Name, nodeSet.Name))

	// build the podTemplate until we have the effective resources configured
	builder = builder.
		WithLabels(labels).
		WithAnnotations(DefaultAnnotations).
		WithDockerImage(es.Spec.Image, container.ImageRepository(container.ElasticsearchImage, es.Spec.Version)).
		WithResources(DefaultResources).
		WithTerminationGracePeriod(DefaultTerminationGracePeriodSeconds).
		WithPorts(defaultContainerPorts).
		WithReadinessProbe(*NewReadinessProbe()).
		WithAffinity(DefaultAffinity(es.Name)).
		WithEnv(DefaultEnvVars(es.Spec.HTTP, headlessServiceName)...).
		WithVolumes(volumes...).
		WithVolumeMounts(volumeMounts...).
		WithInitContainers(initContainers...).
		WithInitContainerDefaults(corev1.EnvVar{Name: settings.HeadlessServiceName, Value: headlessServiceName}).
		WithPreStopHook(*NewPreStopHook())

	builder, err = stackmon.WithMonitoring(client, builder, es)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	if ver.LT(version.From(7, 2, 0)) {
		// mitigate CVE-2021-44228
		enableLog4JFormatMsgNoLookups(builder)
	}

	return builder.PodTemplate, nil
}

func getDefaultContainerPorts(es esv1.Elasticsearch) []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{Name: es.Spec.HTTP.Protocol(), ContainerPort: network.HTTPPort, Protocol: corev1.ProtocolTCP},
		{Name: "transport", ContainerPort: network.TransportPort, Protocol: corev1.ProtocolTCP},
	}
}

func transportCertificatesVolume(ssetName string) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		esv1.StatefulSetTransportCertificatesSecret(ssetName),
		esvolume.TransportCertificatesSecretVolumeName,
		esvolume.TransportCertificatesSecretVolumeMountPath,
	)
}

func buildLabels(
	es esv1.Elasticsearch,
	cfg settings.CanonicalConfig,
	nodeSet esv1.NodeSet,
	keystoreResources *keystore.Resources,
) (map[string]string, error) {
	// label with version
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}

	// label with a hash of the config to rotate the pod on config changes
	unpackedCfg, err := cfg.Unpack(ver)
	if err != nil {
		return nil, err
	}
	cfgHash := hash.HashObject(cfg)
	if es.HasDownwardNodeLabels() {
		// update the config checksum with the list of node labels expected on the pod to rotate the pod when the list is updated
		configChecksum := fnv.New32()
		_, _ = configChecksum.Write([]byte(cfgHash))
		_, _ = configChecksum.Write([]byte(es.Annotations[esv1.DownwardNodeLabelsAnnotation]))
		cfgHash = fmt.Sprint(configChecksum.Sum32())
	}

	node := unpackedCfg.Node
	podLabels := label.NewPodLabels(
		k8s.ExtractNamespacedName(&es),
		esv1.StatefulSet(es.Name, nodeSet.Name),
		ver, node, cfgHash, es.Spec.HTTP.Protocol(),
	)

	if keystoreResources != nil {
		// label with a checksum of the secure settings to rotate the pod on secure settings change
		// TODO: use hash.HashObject instead && fix the config checksum label name?
		configChecksum := sha256.New224()
		_, _ = configChecksum.Write([]byte(keystoreResources.Version))
		podLabels[label.SecureSettingsHashLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))
	}

	return podLabels, nil
}

// enableLog4JFormatMsgNoLookups prepends the JVM parameter `-Dlog4j2.formatMsgNoLookups=true` to the environment variable `ES_JAVA_OPTS`
// in order to mitigate the Log4Shell vulnerability CVE-2021-44228, if it is not yet defined by the user, for
// versions of Elasticsearch before 7.2.0.
func enableLog4JFormatMsgNoLookups(builder *defaults.PodTemplateBuilder) {
	log4j2Param := fmt.Sprintf("%s=true", log4j2FormatMsgNoLookupsParamName)
	for c, esContainer := range builder.PodTemplate.Spec.Containers {
		if esContainer.Name != esv1.ElasticsearchContainerName {
			continue
		}
		currentJvmOpts := ""
		for e, envVar := range esContainer.Env {
			if envVar.Name != settings.EnvEsJavaOpts {
				continue
			}
			currentJvmOpts = envVar.Value
			if !strings.Contains(currentJvmOpts, log4j2FormatMsgNoLookupsParamName) {
				builder.PodTemplate.Spec.Containers[c].Env[e].Value = log4j2Param + " " + currentJvmOpts
			}
		}
		if currentJvmOpts == "" {
			builder.PodTemplate.Spec.Containers[c].Env = append(
				builder.PodTemplate.Spec.Containers[c].Env,
				corev1.EnvVar{Name: settings.EnvEsJavaOpts, Value: log4j2Param},
			)
		}
	}
}
