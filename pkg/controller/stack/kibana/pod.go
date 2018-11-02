package kibana

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	//HTTPPort is the (default) port used by Kibana
	HTTPPort = 5601

	defaultImageRepositoryAndName string = "docker.elastic.co/kibana/kibana"
)

type PodSpecParams struct {
	Version          string
	ElasticsearchUrl string
}

func NewPodSpec(p PodSpecParams) corev1.PodSpec {
	imageName := fmt.Sprintf("%s:%s", defaultImageRepositoryAndName, p.Version)

	probe := &corev1.Probe{
		InitialDelaySeconds: 10,
		PeriodSeconds:       30,
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
			Env: []corev1.EnvVar{
				{Name: "ELASTICSEARCH_URL", Value: p.ElasticsearchUrl},
			},
			Image:           imageName,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Name:            "kibana",
			Ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: int32(HTTPPort), Protocol: corev1.ProtocolTCP},
			},
			LivenessProbe:  probe,
			ReadinessProbe: probe,
		}},
	}

}
