package kibana

import (
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// HTTPPort is the (default) port used by Kibana
	HTTPPort = 5601

	defaultImageRepositoryAndName string = "docker.elastic.co/kibana/kibana"
)

type PodSpecParams struct {
	Version          string
	ElasticsearchUrl string
	CustomImageName  string
	User             client.User
}

func imageWithVersion(image string, version string) string {
	return common.Concat(image, ":", version)
}

func NewPodSpec(p PodSpecParams) corev1.PodSpec {
	imageName := p.CustomImageName
	if p.CustomImageName == "" {
		imageName = imageWithVersion(defaultImageRepositoryAndName, p.Version)
	}

	probe := &corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(HTTPPort),
				Path:   "/",
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}

	return corev1.PodSpec{
		Containers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			},
			Env: []corev1.EnvVar{
				{Name: "ELASTICSEARCH_URL", Value: p.ElasticsearchUrl},
				{Name: "ELASTICSEARCH_USERNAME", Value: p.User.Name},
				{Name: "ELASTICSEARCH_PASSWORD", Value: p.User.Password},
			},
			Image: imageName,
			Name:  "kibana",
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
			},
			ReadinessProbe: probe,
		}},
	}

}
