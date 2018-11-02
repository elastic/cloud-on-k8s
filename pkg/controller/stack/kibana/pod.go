package kibana

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultImageRepositoryAndName string = "docker.elastic.co/kibana/kibana"
)

type PodSpecParams struct {
	Version          string
	ElasticsearchUrl string
	CustomImageName  string
}

func imageWithVersion(image string, version string) string {
	return fmt.Sprintf("%s:%s", image, version)
}

func NewPodSpec(p PodSpecParams) corev1.PodSpec {
	imageName := p.CustomImageName
	if p.CustomImageName == "" {
		imageName = imageWithVersion(defaultImageRepositoryAndName, p.Version)
	}

	port := 5601

	probe := &corev1.Probe{
		InitialDelaySeconds: 10,
		PeriodSeconds:       30,
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.FromInt(port),
				Path:   "/",
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}

	return corev1.PodSpec{
		Containers: []corev1.Container{{
			Env: []corev1.EnvVar{
				{Name: "ELASTICSEARCH_URL", Value: p.ElasticsearchUrl},
			},
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Name:            "kibana",
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: int32(port), Protocol: corev1.ProtocolTCP},
			},
			LivenessProbe:  probe,
			ReadinessProbe: probe,
		}},
	}

}
