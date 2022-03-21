// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

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
	// ConfigHashAnnotationName is an annotation used to store a hash of the Elasticsearch configuration.
	configHashAnnotationName = "elasticsearch.k8s.elastic.co/config-hash"
)

// Starting 8.0.0, the Elasticsearch container does not run with the root user anymore. As a result,
// we cannot chown the mounted volumes to the right user (id 1000) in an init container.
// Instead, we can rely on Kubernetes `securityContext.fsGroup` feature: by setting it to 1000
// mounted volumes can correctly be accessed by the default container user.
// On some restricted environments (custom PSPs or Openshift), setting the Pod security context
// is forbidden: the user can either set `--set-default-security-context=false`, or override the
// podTemplate securityContext to an empty value.
var minDefaultSecurityContextVersion = version.MinFor(8, 0, 0)

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

	labels, err := buildLabels(es, cfg, nodeSet)
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

	// We retrieve the ConfigMap that holds the scripts to trigger a Pod restart if it is updated.
	esScripts := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.ScriptsConfigMap(es.Name)}, esScripts); err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	annotations := buildAnnotations(es, cfg, keystoreResources, esScripts.ResourceVersion)

	// build the podTemplate until we have the effective resources configured
	builder = builder.
		WithLabels(labels).
		WithAnnotations(annotations).
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
) (map[string]string, error) {
	// label with version
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}

	unpackedCfg, err := cfg.Unpack(ver)
	if err != nil {
		return nil, err
	}

	node := unpackedCfg.Node
	podLabels := label.NewPodLabels(
		k8s.ExtractNamespacedName(&es),
		esv1.StatefulSet(es.Name, nodeSet.Name),
		ver, node, es.Spec.HTTP.Protocol(),
	)

	return podLabels, nil
}

func buildAnnotations(
	es esv1.Elasticsearch,
	cfg settings.CanonicalConfig,
	keystoreResources *keystore.Resources,
	scriptsVersion string,
) map[string]string {
	// start from our defaults
	annotations := DefaultAnnotations

	configHash := fnv.New32a()
	// hash of the ES config to rotate the pod on config changes
	hash.WriteHashObject(configHash, cfg)
	// hash of the scripts' version to rotate the pod if the scripts have changed
	_, _ = configHash.Write([]byte(scriptsVersion))

	if es.HasDownwardNodeLabels() {
		// list of node labels expected on the pod to rotate the pod when the list is updated
		_, _ = configHash.Write([]byte(es.Annotations[esv1.DownwardNodeLabelsAnnotation]))
	}

	if keystoreResources != nil {
		// resource version of the secure settings secret to rotate the pod on secure settings change
		_, _ = configHash.Write([]byte(keystoreResources.Version))
	}

	// set the annotation in place
	annotations[configHashAnnotationName] = fmt.Sprint(configHash.Sum32())

	return annotations
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
