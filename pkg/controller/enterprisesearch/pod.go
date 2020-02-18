package enterprisesearch

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	HTTPPort = 3002
	DefaultJavaOpts = "-Xms3500m -Xmx3500m"
	ConfigHashLabelName = "enterprisesearch.k8s.elastic.co/config-hash"
)

var (
	DefaultMemoryLimits = resource.MustParse("4Gi")
	DefaultResources    = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: DefaultMemoryLimits,
		},
	}

	DefaultEnv = []corev1.EnvVar{
		{Name: "JAVA_OPTS", Value: DefaultJavaOpts},
		{Name: "ENT_SEARCH_CONFIG_PATH", Value: filepath.Join(ConfigMountPath, ConfigFilename)},
	}
)

func newPodSpec(ents entsv1beta1.EnterpriseSearch, configHash string) corev1.PodTemplateSpec {
	cfgVolume := ConfigSecretVolume(ents)

	builder := defaults.NewPodTemplateBuilder(
		ents.Spec.PodTemplate, entsv1beta1.EnterpriseSearchContainerName).
		WithResources(DefaultResources).
		WithDockerImage(ents.Spec.Image, container.ImageRepository(container.EnterpriseSearchImage, ents.Spec.Version)).
		//WithReadinessProbe(readinessProbe(as.Spec.HTTP.TLS.Enabled())).
		WithPorts([]corev1.ContainerPort{
			{Name: ents.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
		}).
		WithVolumes(cfgVolume.Volume()).
		WithVolumeMounts(cfgVolume.VolumeMount()).
		WithEnv(DefaultEnv...).
		// ensure the Pod gets rotated on config change
		WithLabels(map[string]string{ConfigHashLabelName: configHash})

	builder = withESCertsVolume(builder, ents)

	return builder.PodTemplate
}

// TODO: handle differently?
func withESCertsVolume(builder *defaults.PodTemplateBuilder, ents entsv1beta1.EnterpriseSearch) *defaults.PodTemplateBuilder {
	if !ents.AssociationConf().CAIsConfigured() {
		return builder
	}
	vol := volume.NewSecretVolumeWithMountPath(
		ents.AssociationConf().GetCASecretName(),
		"es-certs",
		ESCertsPath,
	)
	return builder.
		WithVolumes(vol.Volume()).
		WithVolumeMounts(vol.VolumeMount())
}

