package enterprisesearch

import (
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	HTTPPort = 3002
	DefaultJavaOpts = "-Xms3500m -Xmx3500m"
	ESCertsPath = "/mnt/es-certs"
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
)

func newPodSpec(ents entsv1beta1.EnterpriseSearch) corev1.PodTemplateSpec {
	builder := defaults.NewPodTemplateBuilder(
		ents.Spec.PodTemplate, entsv1beta1.EnterpriseSearchContainerName).
		WithResources(DefaultResources).
		WithDockerImage(ents.Spec.Image, container.ImageRepository(container.EnterpriseSearchImage, ents.Spec.Version)).
		// TODO
		//WithReadinessProbe(readinessProbe(as.Spec.HTTP.TLS.Enabled())).
		WithPorts([]corev1.ContainerPort{
			{Name: ents.Spec.HTTP.Protocol(), ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
		}).
		WithEnv(defaultEnv()...).
		WithEnv(esAssociationEnv(ents)...)
	builder = withAssociationVolume(builder, ents)

	return builder.PodTemplate
}

func defaultEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "JAVA_OPTS",
			Value: DefaultJavaOpts,
		},
		// TODO secret
		{
			Name: "secret_session_key",
			Value: "TODOCHANGEMEsecret_session_key",
		},
		// TODO secret
		{
			Name: "secret_management.encryption_keys",
			Value: "[TODOCHANGEMEsecret_management.encryption_keys]",
		},
		{
			Name: "ent_search.external_url",
			Value: fmt.Sprintf("http://localhost:%d", HTTPPort),
		},
		// TODO: "ent_search"?
		{
			Name: "app_search.listen_host",
			Value: "0.0.0.0",
		},
		{
			Name: "allow_es_settings_modification",
			Value: "true",
		},
	}
}

// TODO create a dedicated user
func esAssociationEnv(ents entsv1beta1.EnterpriseSearch) []corev1.EnvVar {
	assoc := ents.AssociationConf()
	if assoc == nil {
		return nil
	}

	return []corev1.EnvVar{
		{
			Name: "ent_search.auth.source",
			Value: "elasticsearch-native",
		},
		{
			Name: "elasticsearch.host",
			Value: assoc.URL,
		},
		{
			Name: "elasticsearch.password",
			ValueFrom: &corev1.EnvVarSource {
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: esv1.ElasticUserSecret(ents.Spec.ElasticsearchRef.Name),
					},
					Key:                  "elastic",
					Optional:             nil,
				},
			},
		},
		{
			Name: "elasticsearch.ssl.enabled",
			Value: "true",
		},
		{
			Name: "elasticsearch.ssl.certificate_authority",
			Value: filepath.Join(ESCertsPath, "tls.crt"),
		},
	}
}

// TODO: refactor
func withAssociationVolume(builder *defaults.PodTemplateBuilder, ents entsv1beta1.EnterpriseSearch) *defaults.PodTemplateBuilder {
	if assoc := ents.AssociationConf(); assoc != nil {
		vol := volume.NewSecretVolumeWithMountPath(
			assoc.CASecretName,
			"es-certs",
			ESCertsPath,
		)
		builder = builder.WithVolumes(vol.Volume()).WithVolumeMounts(vol.VolumeMount())
	}
	return builder
}
